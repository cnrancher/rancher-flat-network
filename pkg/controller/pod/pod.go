package pod

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cnrancher/rancher-flat-network/pkg/controller/wrangler"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	appscontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/apps/v1"
	batchcontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/batch/v1"
	corecontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/core/v1"
	flcontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/flatnetwork.pandaria.io/v1"
)

const (
	handlerName = "rancher-flat-network-pod"
)

type handler struct {
	podClient    corecontroller.PodClient
	podCache     corecontroller.PodCache
	ipClient     flcontroller.FlatNetworkIPClient
	ipCache      flcontroller.FlatNetworkIPCache
	subnetCache  flcontroller.FlatNetworkSubnetCache
	subnetClient flcontroller.FlatNetworkSubnetController

	namespaceCache   corecontroller.NamespaceCache
	deploymentCache  appscontroller.DeploymentCache
	daemonSetCache   appscontroller.DaemonSetCache
	replicaSetCache  appscontroller.ReplicaSetCache
	statefulSetCache appscontroller.StatefulSetCache
	cronJobCache     batchcontroller.CronJobCache
	jobCache         batchcontroller.JobCache

	podEnqueueAfter func(string, string, time.Duration)
	podEnqueue      func(string, string)
}

func Register(
	ctx context.Context,
	wctx *wrangler.Context,
) {
	h := &handler{
		podClient:    wctx.Core.Pod(),
		podCache:     wctx.Core.Pod().Cache(),
		ipClient:     wctx.FlatNetwork.FlatNetworkIP(),
		ipCache:      wctx.FlatNetwork.FlatNetworkIP().Cache(),
		subnetCache:  wctx.FlatNetwork.FlatNetworkSubnet().Cache(),
		subnetClient: wctx.FlatNetwork.FlatNetworkSubnet(),

		namespaceCache:   wctx.Core.Namespace().Cache(),
		deploymentCache:  wctx.Apps.Deployment().Cache(),
		daemonSetCache:   wctx.Apps.DaemonSet().Cache(),
		replicaSetCache:  wctx.Apps.ReplicaSet().Cache(),
		statefulSetCache: wctx.Apps.StatefulSet().Cache(),
		cronJobCache:     wctx.Batch.CronJob().Cache(),
		jobCache:         wctx.Batch.Job().Cache(),

		podEnqueueAfter: wctx.Core.Pod().EnqueueAfter,
		podEnqueue:      wctx.Core.Pod().Enqueue,
	}

	wctx.Core.Pod().OnChange(ctx, handlerName, h.handleError(h.sync))
}

func (h *handler) handleError(
	sync func(string, *corev1.Pod) (*corev1.Pod, error),
) func(string, *corev1.Pod) (*corev1.Pod, error) {
	return func(s string, pod *corev1.Pod) (*corev1.Pod, error) {
		podSynced, err := sync(s, pod)
		if err != nil {
			logrus.WithFields(fieldsPod(pod)).Error(err)
			return pod, err
		}
		return podSynced, nil
	}
}

// sync ensures flat-network IP resource exists.
func (h *handler) sync(_ string, pod *corev1.Pod) (*corev1.Pod, error) {
	// Skip non-flat-network pods
	if !utils.IsPodEnabledFlatNetwork(pod) {
		return pod, nil
	}
	if pod.DeletionTimestamp != nil {
		// The pod is deleting.
		err := h.ipClient.Delete(pod.Namespace, pod.Name, &metav1.DeleteOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return pod, nil
			}
			return pod, err
		}
		return pod, nil
	}

	// Ensure FlatNetwork IP resource created.
	flatnetworkIP, err := h.ensureFlatNetworkIP(pod)
	if err != nil {
		return pod, fmt.Errorf("ensureFlatNetworkIP: %w", err)
	}
	if flatnetworkIP == nil || flatnetworkIP.Status.Phase != "Active" {
		logrus.WithFields(fieldsPod(pod)).
			Debugf("waiting for flat-network IP status to active")

		// Requeue in few seconds to wait for IP status active.
		// This will not block the pod creation process and just waiting for
		// a few seconds to update the pod flat-network labels.
		h.podEnqueueAfter(pod.Namespace, pod.Name, time.Second*5)
		return pod, nil
	}

	// Ensure Pod label updated with the FlatNetworkIP.
	if err = h.updatePodLabel(pod, flatnetworkIP); err != nil {
		return pod, err
	}

	return pod, nil
}

// ensureFlatNetworkIP ensure the FlatNetworkIP resource exists.
func (h *handler) ensureFlatNetworkIP(pod *corev1.Pod) (*flv1.FlatNetworkIP, error) {
	existFlatNetworkIP, err := h.ipCache.Get(pod.Namespace, pod.Name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			logrus.WithFields(fieldsPod(pod)).
				Errorf("failed to get flat-network IP: %v", err)
			return nil, err
		}
	}
	expectedIP, err := h.newFlatNetworkIP(pod)
	if err != nil {
		return expectedIP, err
	}
	h.setIfStatefulSetOwnerRef(expectedIP, pod)
	h.setWorkloadAndProjectLabel(expectedIP, pod)
	if flatNetworkIPUpdated(existFlatNetworkIP, expectedIP) {
		// FlatNetworkIP created and no need to update, return
		return existFlatNetworkIP, nil
	}

	if existFlatNetworkIP != nil {
		// FlatNetworkIP already exists, update specs
		err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			existFlatNetworkIP, err = h.ipCache.Get(pod.Namespace, pod.Name)
			if err != nil {
				return err
			}
			existFlatNetworkIP = existFlatNetworkIP.DeepCopy()
			existFlatNetworkIP.OwnerReferences = expectedIP.OwnerReferences
			existFlatNetworkIP.Labels = expectedIP.Labels
			existFlatNetworkIP.Annotations = expectedIP.Annotations
			existFlatNetworkIP.Spec = expectedIP.Spec
			result, err := h.ipClient.Update(existFlatNetworkIP)
			if err != nil {
				return err
			}
			existFlatNetworkIP = result
			return nil
		})
		if err != nil {
			return existFlatNetworkIP, err
		}
		logrus.WithFields(fieldsPod(pod)).
			Infof("request to update flat-network IP [%v/%v]",
				pod.Namespace, pod.Name)
	}

	// FlatNetworkIP not exists, create
	createdFlatNetworkIP, err := h.ipClient.Create(expectedIP)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			// The IP may just created and the informer cache may not
			// synced yet, ignore error, return the expectedIP directly
			// and requeue to wait for IP status to active.
			return expectedIP, nil
		}
		return nil, err
	}
	logrus.WithFields(fieldsPod(pod)).
		Infof("request to create flat-network IP [%v/%v]",
			pod.Namespace, pod.Name)

	return createdFlatNetworkIP, nil
}

func (h *handler) updatePodLabel(pod *corev1.Pod, ip *flv1.FlatNetworkIP) error {
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	annotationIP := pod.Annotations[flv1.AnnotationIP]
	annotationSubnet := pod.Annotations[flv1.AnnotationSubnet]
	annotationMac := pod.Annotations[flv1.AnnotationMac]

	labels := map[string]string{}
	labels[flv1.LabelSubnet] = annotationSubnet
	labels[flv1.LabelSelectedIP] = ""
	labels[flv1.LabelSelectedMac] = ""
	labels[flv1.LabelFlatNetworkIPType] = flv1.AllocateModeSpecific

	if ip.Status.Addr != nil {
		// IPv6 address contains invalid char ':'
		s := ip.Status.Addr.String()
		labels[flv1.LabelSelectedIP] = strings.ReplaceAll(s, ":", ".")
	}
	if ip.Status.MAC != "" && annotationMac != "" {
		labels[flv1.LabelSelectedMac] = strings.ReplaceAll(ip.Status.MAC, ":", "")
	}
	if annotationIP == flv1.AllocateModeAuto {
		labels[flv1.LabelFlatNetworkIPType] = flv1.AllocateModeAuto
	}
	skip := true
	for k, v := range labels {
		if pod.Labels[k] != v {
			skip = false
			break
		}
	}
	if skip {
		// Pod label already updated, skip.
		return nil
	}

	// Pod may just created and updated by other kube-controllers,
	// use retry to avoid conflict.
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		result, err := h.podCache.Get(pod.Namespace, pod.Name)
		if err != nil {
			logrus.WithFields(fieldsPod(pod)).
				Errorf("failed to get latest version of pod: %v", err)
			return err
		}
		pod := result.DeepCopy()
		if pod.Labels == nil {
			pod.Labels = make(map[string]string)
		}
		for k, v := range labels {
			pod.Labels[k] = v
		}
		_, err = h.podClient.Update(pod)
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		logrus.WithFields(fieldsPod(pod)).
			Errorf("failed to update pod flatnetwork label: %v", err)
		return err
	}
	logrus.WithFields(fieldsPod(pod)).
		Debugf("finished syncing pod flat network label")

	return nil
}

func fieldsPod(pod *corev1.Pod) logrus.Fields {
	if pod == nil {
		return logrus.Fields{}
	}
	return logrus.Fields{
		"GID": utils.GID(),
		"POD": fmt.Sprintf("%v/%v", pod.Namespace, pod.Name),
	}
}

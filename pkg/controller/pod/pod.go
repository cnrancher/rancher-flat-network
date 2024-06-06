package pod

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cnrancher/flat-network-operator/pkg/controller/wrangler"
	"github.com/cnrancher/flat-network-operator/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	flv1 "github.com/cnrancher/flat-network-operator/pkg/apis/flatnetwork.cattle.io/v1"
	appscontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/apps/v1"
	batchcontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/batch/v1"
	corecontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/core/v1"
	flcontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/flatnetwork.cattle.io/v1"
)

const (
	handlerName = "flatnetwork-pod"
)

type handler struct {
	podClient    corecontroller.PodClient
	podCache     corecontroller.PodCache
	ipClient     flcontroller.IPClient
	ipCache      flcontroller.IPCache
	subnetCache  flcontroller.SubnetCache
	subnetClient flcontroller.SubnetController

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
		ipClient:     wctx.FlatNetwork.IP(),
		ipCache:      wctx.FlatNetwork.IP().Cache(),
		subnetCache:  wctx.FlatNetwork.Subnet().Cache(),
		subnetClient: wctx.FlatNetwork.Subnet(),

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
			logrus.WithFields(fieldsPod(pod)).
				Errorf("failed to sync pod: %v", err)
			return podSynced, err
		}
		return podSynced, nil
	}
}

// sync ensures flat-network IP resource exists.
func (h *handler) sync(name string, pod *corev1.Pod) (*corev1.Pod, error) {
	// Skip non-flat-network pods
	if !utils.IsPodEnabledFlatNetwork(pod) {
		return pod, nil
	}
	if pod.DeletionTimestamp != nil {
		// The pod is deleting.
		return pod, nil
	}
	pod, err := h.podCache.Get(pod.Namespace, pod.Name)
	if err != nil {
		return pod, fmt.Errorf("failed to get pod from cache: %v", err)
	}

	// Ensure FlatNetwork IP resource created.
	flatnetworkIP, err := h.ensureFlatNetworkIP(pod)
	if err != nil {
		return pod, fmt.Errorf("ensureFlatNetworkIP: %w", err)
	}
	if flatnetworkIP == nil || flatnetworkIP.Status.Phase != "Active" {
		logrus.WithFields(fieldsPod(pod)).
			Infof("waiting for flat-network IP status to active")

		// Requeue in few seconds to wait for IP status active.
		// This will not block the pod creation process and just waiting for
		// a few seconds to update the pod flatnetwork labels.
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
func (h *handler) ensureFlatNetworkIP(pod *corev1.Pod) (*flv1.IP, error) {
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
		return existFlatNetworkIP, nil
	}

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
		Infof("request to create flat-network IP [%v/%v] IP [%v]",
			pod.Namespace, pod.Name, createdFlatNetworkIP.Spec.CIDR)

	return createdFlatNetworkIP, nil
}

func (h *handler) updatePodLabel(pod *corev1.Pod, ip *flv1.IP) error {
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	annotationIP := pod.Annotations[flv1.AnnotationIP]
	annotationSubnet := pod.Annotations[flv1.AnnotationSubnet]
	annotationMac := pod.Annotations[flv1.AnnotationMac]

	labels := map[string]string{}
	labels[flv1.LabelMultipleIPHash] = calcHash(annotationIP, annotationMac)
	labels[flv1.LabelSubnet] = annotationSubnet
	labels[flv1.LabelSelectedIP] = ""
	labels[flv1.LabelSelectedMac] = ""
	labels[flv1.LabelFlatNetworkIPType] = "specific"

	if ip.Status.Address != nil {
		if ip.Status.Address.To4() != nil {
			// TODO: IPv6 address contains invalid char ':'
			// Set IPv4 labels only.
			labels[flv1.LabelSelectedIP] = ip.Status.Address.String()
		}
	}
	if ip.Status.MAC != nil {
		labels[flv1.LabelSelectedMac] = strings.ReplaceAll(ip.Status.MAC.String(), ":", "_")
	}
	if annotationIP == "auto" {
		labels[flv1.LabelFlatNetworkIPType] = "auto"
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
		result, err := h.podClient.Get(pod.Namespace, pod.Name, metav1.GetOptions{})
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
		Infof("finished syncing pod flat network label")

	return nil
}

func fieldsPod(pod *corev1.Pod) logrus.Fields {
	if pod == nil {
		return logrus.Fields{}
	}
	return logrus.Fields{
		"GID": utils.GetGID(),
		"POD": fmt.Sprintf("%v/%v", pod.Namespace, pod.Name),
	}
}

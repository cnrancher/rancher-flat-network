package pod

import (
	"context"
	"fmt"
	"strings"
	"time"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/controller/wrangler"
	"github.com/cnrancher/flat-network-operator/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

	appscontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/apps/v1"
	batchcontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/batch/v1"
	corecontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/core/v1"
	macvlancontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/macvlan.cluster.cattle.io/v1"
)

const (
	handlerName = "flat-network-pod"
)

type handler struct {
	podClient           corecontroller.PodClient
	podCache            corecontroller.PodCache
	macvlanIPClient     macvlancontroller.MacvlanIPClient
	macvlanIPCache      macvlancontroller.MacvlanIPCache
	macvlanSubnetCache  macvlancontroller.MacvlanSubnetCache
	macvlanSubnetClient macvlancontroller.MacvlanSubnetController

	namespaceCache   corecontroller.NamespaceCache
	deploymentCache  appscontroller.DeploymentCache
	daemonSetCache   appscontroller.DaemonSetCache
	replicaSetCache  appscontroller.ReplicaSetCache
	statefulSetCache appscontroller.StatefulSetCache
	cronJobCache     batchcontroller.CronJobCache
	jobCache         batchcontroller.JobCache

	recorder record.EventRecorder

	podEnqueueAfter func(string, string, time.Duration)
	podEnqueue      func(string, string)
}

func Register(
	ctx context.Context,
	wctx *wrangler.Context,
) {
	h := &handler{
		podClient:           wctx.Core.Pod(),
		podCache:            wctx.Core.Pod().Cache(),
		macvlanIPClient:     wctx.Macvlan.MacvlanIP(),
		macvlanIPCache:      wctx.Macvlan.MacvlanIP().Cache(),
		macvlanSubnetCache:  wctx.Macvlan.MacvlanSubnet().Cache(),
		macvlanSubnetClient: wctx.Macvlan.MacvlanSubnet(),

		namespaceCache:   wctx.Core.Namespace().Cache(),
		deploymentCache:  wctx.Apps.Deployment().Cache(),
		daemonSetCache:   wctx.Apps.DaemonSet().Cache(),
		replicaSetCache:  wctx.Apps.ReplicaSet().Cache(),
		statefulSetCache: wctx.Apps.StatefulSet().Cache(),
		cronJobCache:     wctx.Batch.CronJob().Cache(),
		jobCache:         wctx.Batch.Job().Cache(),

		recorder: wctx.Recorder,

		podEnqueueAfter: wctx.Core.Pod().EnqueueAfter,
		podEnqueue:      wctx.Core.Pod().Enqueue,
	}

	wctx.Core.Pod().OnChange(ctx, handlerName, h.sync)
}

// sync ensures macvlanIP resource exists.
func (h *handler) sync(name string, pod *corev1.Pod) (*corev1.Pod, error) {
	// Skip non-macvlan pods
	if !utils.IsMacvlanPod(pod) {
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

	// Ensure FlatNetwork IP (macvlanIP) resource created.
	macvlanIP, err := h.ensureFlatNetworkIP(pod)
	if err != nil {
		// h.eventMacvlanIPError(pod, err)
		return pod, fmt.Errorf("ensureFlatNetworkIP: %w", err)
	}
	// Ensure Pod label updated with the FlatNetworkIP.
	if err = h.updatePodLabel(pod, macvlanIP); err != nil {
		// h.eventMacvlanIPError(pod, err)
		return pod, err
	}

	return pod, nil
}

// ensureFlatNetworkIP ensure the FlatNetworkIP (macvlanIP) resource exists.
func (h *handler) ensureFlatNetworkIP(pod *corev1.Pod) (*macvlanv1.MacvlanIP, error) {
	existMacvlanIP, err := h.macvlanIPCache.Get(pod.Namespace, pod.Name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			logrus.WithFields(fieldsPod(pod)).
				Errorf("failed to get macvlanIP: %v", err)
			return nil, err
		}
	}
	expectedIP, err := h.newMacvlanIP(pod)
	if err != nil {
		return expectedIP, err
	}
	h.setIfStatefulSetOwnerRef(expectedIP, pod)
	h.setWorkloadAndProjectLabel(expectedIP, pod)
	if macvlanIPUpdated(existMacvlanIP, expectedIP) {
		return existMacvlanIP, nil
	}

	createdMacvlanIP, err := h.macvlanIPClient.Create(expectedIP)
	if err != nil {
		return nil, err
	}
	logrus.WithFields(fieldsPod(pod)).
		Infof("request to created macvlanIP [%v/%v] IP [%v]",
			pod.Namespace, pod.Name, createdMacvlanIP.Spec.IP)
	return createdMacvlanIP, nil
}

func (h *handler) updatePodLabel(pod *corev1.Pod, macvlanIP *macvlanv1.MacvlanIP) error {
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	annotationIP := pod.Annotations[macvlanv1.AnnotationIP]
	annotationSubnet := pod.Annotations[macvlanv1.AnnotationSubnet]
	annotationMac := pod.Annotations[macvlanv1.AnnotationMac]

	if macvlanIP.Status.Phase != "Active" {
		logrus.WithFields(fieldsPod(pod)).
			Infof("waiting for macvlanIP status to Active")
		h.podEnqueueAfter(pod.Namespace, pod.Name, time.Second*5)
		return nil
	}

	labels := map[string]string{}
	labels[macvlanv1.LabelMultipleIPHash] = calcHash(annotationIP, annotationMac)
	labels[macvlanv1.LabelSubnet] = annotationSubnet
	if macvlanIP.Status.IP != nil {
		labels[macvlanv1.LabelSelectedIP] = macvlanIP.Status.IP.String()
	} else {
		labels[macvlanv1.LabelSelectedIP] = ""
	}
	if macvlanIP.Spec.MAC != nil {
		labels[macvlanv1.LabelSelectedMac] = strings.ReplaceAll(macvlanIP.Spec.MAC.String(), ":", "_")
	} else {
		labels[macvlanv1.LabelSelectedMac] = ""
	}
	if annotationIP == "auto" {
		labels[macvlanv1.LabelMacvlanIPType] = "auto"
	} else {
		labels[macvlanv1.LabelMacvlanIPType] = "specific"
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
			logrus.WithFields(fieldsPod(pod)).
				Warnf("onPodUpdate: failed to update pod label [%v]: %v",
					utils.PrintObject(pod.Labels), err)
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	logrus.WithFields(fieldsPod(pod)).
		Infof("finished syncing pod flat network labels")

	return nil
}

func fieldsPod(pod *corev1.Pod) logrus.Fields {
	if pod == nil {
		return logrus.Fields{}
	}
	return logrus.Fields{
		"POD": fmt.Sprintf("%v/%v", pod.Namespace, pod.Name),
	}
}

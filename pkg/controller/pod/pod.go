package pod

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/cnrancher/flat-network-operator/pkg/controller/wrangler"
	"github.com/cnrancher/flat-network-operator/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	appscontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/apps/v1"
	batchcontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/batch/v1"
	corecontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/core/v1"
	macvlancontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/macvlan.cluster.cattle.io/v1"
)

const (
	controllerName       = "pod"
	controllerRemoveName = "pod-remove"
)

var (
	errSelectedIPMismatch = fmt.Errorf("pod selected ip mismatch")
)

type handler struct {
	podClient          corecontroller.PodClient
	podCache           corecontroller.PodCache
	macvlanIPClient    macvlancontroller.MacvlanIPClient
	macvlanIPCache     macvlancontroller.MacvlanIPCache
	macvlanSubnetCache macvlancontroller.MacvlanSubnetCache
	namespaceCache     corecontroller.NamespaceCache

	deploymentCache  appscontroller.DeploymentCache
	daemonSetCache   appscontroller.DaemonSetCache
	replicaSetCache  appscontroller.ReplicaSetCache
	statefulSetCache appscontroller.StatefulSetCache
	cronJobCache     batchcontroller.CronJobCache
	jobCache         batchcontroller.JobCache

	recorder record.EventRecorder

	podEnqueueAfter func(string, string, time.Duration)
	podEnqueue      func(string, string)

	// mutex for allocating IP address
	mutex sync.Mutex
}

func Register(
	ctx context.Context,
	wctx *wrangler.Context,
) {
	h := &handler{
		podClient:          wctx.Core.Pod(),
		podCache:           wctx.Core.Pod().Cache(),
		namespaceCache:     wctx.Core.Namespace().Cache(),
		macvlanIPClient:    wctx.Macvlan.MacvlanIP(),
		macvlanIPCache:     wctx.Macvlan.MacvlanIP().Cache(),
		macvlanSubnetCache: wctx.Macvlan.MacvlanSubnet().Cache(),

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

	wctx.Core.Pod().OnChange(ctx, controllerName, h.onPodUpdate)
	wctx.Core.Pod().OnRemove(ctx, controllerName, h.onPodRemove)
}

// onPodUpdate creates macvlanIP resource and update pod label
// when the macvlan pod created.
func (h *handler) onPodUpdate(name string, pod *corev1.Pod) (*corev1.Pod, error) {
	// Skip non-macvlan pods
	if !utils.IsMacvlanPod(pod) {
		return pod, nil
	}

	logrus.WithFields(fieldsPod(pod)).Infof("Sync POD %v", pod.Name)
	ok, err := h.checkMacvlanIPInitialized(pod)
	if err != nil {
		if errors.Is(err, errSelectedIPMismatch) {
			// FIXME: Not recommended to delete pod directly.

			// Pod label selected ip does not match the exists macvlanip, will
			// delete the pod directly.
			logrus.Warnf("Request to delete pod [%v/%v]", pod.Namespace, pod.Name)
			err = h.podClient.Delete(pod.Namespace, pod.Name, &metav1.DeleteOptions{})
			if err != nil {
				logrus.Warnf("failed to delete pod [%v/%v]: %v", pod.Namespace, pod.Name, err)
				return pod, err
			}
			return pod, nil
		}
		return pod, fmt.Errorf("failed to check macvlan pod [%v/%v] initialized %w",
			pod.Namespace, pod.Name, err)
	}
	if ok {
		// Skip if macvlanip already created.
		return pod, nil
	}

	annotationIP := pod.Annotations[macvlanv1.AnnotationIP]
	annotationSubnet := pod.Annotations[macvlanv1.AnnotationSubnet]
	annotationMac := pod.Annotations[macvlanv1.AnnotationMac]

	subnet, err := h.macvlanSubnetCache.Get(macvlanv1.MacvlanSubnetNamespace, annotationSubnet)
	if err != nil {
		h.eventMacvlanSubnetError(pod, err)
		logrus.WithFields(fieldsPod(pod)).
			Errorf("failed to get subnet [%v]: %v",
				annotationSubnet, err)
		return pod, err
	}

	if err := h.validateSubnetProject(subnet, pod); err != nil {
		h.eventMacvlanSubnetError(pod, err)
		logrus.WithFields(fieldsPod(pod)).
			Errorf("failed to validate subnet project: %v", err)
		return pod, err
	}

	// allocate ip in subnet
	var allocatedIP net.IP
	var allocatedMac net.HardwareAddr
	var allocatedCIDR string
	var macvlanipType string = "specific"
	if annotationMac == "auto" {
		annotationMac = ""
	}

	// existMacvlanIP, _ := c.macvlanipsLister.MacvlanIPs(pod.Namespace).Get(pod.Name)
	existMacvlanIP, err := h.macvlanIPCache.Get(pod.Namespace, pod.Name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			logrus.WithFields(fieldsPod(pod)).
				Errorf("failed to get macvlanip: %v", err)
			return pod, err
		}
	}
	switch {
	case annotationIP == "auto":
		macvlanipType = "auto"
		if existMacvlanIP != nil {
			// FIXME: remove here.
			// for statefulset pod
			allocatedCIDR = existMacvlanIP.Spec.CIDR
			allocatedMac = existMacvlanIP.Spec.MAC
			allocatedIP, _, err = net.ParseCIDR(existMacvlanIP.Spec.CIDR)
			if err != nil {
				logrus.WithFields(fieldsPod(pod)).
					Errorf("failed to parse macvlanip [%v] CIDR [%v]: %v",
						existMacvlanIP.Name, existMacvlanIP.Spec.CIDR, err)
				return pod, err
			}
			break
		}

		allocatedIP, allocatedMac, err = h.allocateIPModeAuto(subnet, annotationMac)
		if err != nil {
			logrus.WithFields(fieldsPod(pod)).
				Errorf("failed to allocate macvlanip in auto mode: %v", err)
			return pod, err
		}
	case utils.IsSingleIP(annotationIP):
		allocatedIP, allocatedMac, err = h.allocateIPModeSingle(pod, subnet, annotationIP, annotationMac)
		if err != nil {
			logrus.WithFields(fieldsPod(pod)).
				Errorf("failed to allocate macvlanip in single mode: %v", err)
		}
	case utils.IsMultipleIP(annotationIP):
		allocatedIP, allocatedMac, err = h.allocateIPModeMultiple(pod, subnet, annotationIP, annotationMac)
		if err != nil {
			logrus.WithFields(fieldsPod(pod)).
				Errorf("failed to allocate macvlanip in single mode: %v", err)
		}
	default:
		err = fmt.Errorf("invalid anotation [%v] of pod [%v/%v]: %v",
			macvlanv1.AnnotationIP, pod.Namespace, pod.Name, annotationIP)
		logrus.WithFields(fieldsPod(pod)).Errorf("%v", err)
		return pod, err
	}
	logrus.WithFields(fieldsPod(pod)).
		Infof("Allocate macvlan IP [%v] MAC [%v] CIDR [%v]",
			allocatedIP.String(), allocatedMac, allocatedCIDR)

	// TODO: Update macvlansubnet status inUsedIP & inUsedMAC.

	// Create expectedMacvlanIP before updating the pod label to prevent the
	// static-macvlan-cni flush expectedMacvlanIP CRD not found errors.
	expectedMacvlanIP := makeMacvlanIP(pod, subnet, allocatedIP, allocatedMac, macvlanipType)
	// Add statefulset support.
	h.setIfStatefulSetOwnerRef(expectedMacvlanIP, pod)
	// Set workload/project label.
	h.setWorkloadAndProjectLabel(expectedMacvlanIP, pod)

	if err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if existMacvlanIP != nil {
			if macvlanIPUpdated(existMacvlanIP, expectedMacvlanIP) {
				// Skip if macvlanip already updated.
				logrus.WithFields(fieldsPod(pod)).
					Debugf("macvlanip already updated, skip")
				return nil
			}
			macvlanIP := existMacvlanIP.DeepCopy()
			macvlanIP.Annotations = expectedMacvlanIP.Annotations
			macvlanIP.Labels = expectedMacvlanIP.Labels
			macvlanIP.OwnerReferences = expectedMacvlanIP.OwnerReferences
			macvlanIP.Spec = expectedMacvlanIP.Spec
			_, err = h.macvlanIPClient.Update(macvlanIP)
			if err != nil {
				logrus.WithFields(fieldsPod(pod)).
					Warnf("failed to update macvlanip [%v/%v]: %v",
						pod.Namespace, expectedMacvlanIP.Name, err)
				return err
			}
			return nil
		}

		_, err = h.macvlanIPClient.Create(expectedMacvlanIP)
		if err != nil {
			logrus.WithFields(fieldsPod(pod)).
				Warnf("failed to create macvlanip [%v/%v]: %v",
					pod.Namespace, expectedMacvlanIP.Name, err)
			return err
		}
		return nil
	}); err != nil {
		h.eventMacvlanIPError(pod, err)
		logrus.WithFields(fieldsPod(pod)).
			Errorf("macvlanip sync failed: %v", err)
		return pod, err
	}
	logrus.WithFields(fieldsPod(pod)).
		Infof("Successfully sync macvlanIP [%v/%v]", pod.Namespace, pod.Name)

	// Update macvlanip label (ip, selectedip) after the macvlanip was created.
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err := h.podClient.Get(pod.Namespace, pod.Name, metav1.GetOptions{})
		if err != nil {
			logrus.WithFields(fieldsPod(pod)).
				Warnf("onPodUpdate: failed to get latest version of Pod [%v/%v]: %v",
					pod.Namespace, pod.Name, err)
			return err
		}
		pod = result.DeepCopy()
		if pod.Labels == nil {
			pod.Labels = map[string]string{}
		}

		pod.Labels[macvlanv1.LabelMultipleIPHash] = calcHash(annotationIP, annotationMac)
		pod.Labels[macvlanv1.LabelSelectedIP] = allocatedIP.String()
		pod.Labels[macvlanv1.LabelSelectedMac] = strings.Replace(allocatedMac.String(), ":", "_", -1)
		pod.Labels[macvlanv1.LabelMacvlanIPType] = macvlanipType
		pod.Labels[macvlanv1.LabelSubnet] = annotationSubnet
		if equality.Semantic.DeepEqual(result.Labels, pod.Labels) {
			// Skip update pod labels if the selected ip already match.
			logrus.WithFields(fieldsPod(pod)).
				Debugf("pod label already updated, skip")
			return nil
		}

		// Pod will be frequently updated by multus-cni and rancher when it is
		// just created, so the Update may failed with conflict and need to retry few times.
		logrus.Debugf("Kube apiserver update pod [%v/%v] request", pod.Namespace, pod.Name)
		// _, err = c.kubeClientset.CoreV1().Pods(pod.Namespace).Update(context.TODO(), pod, metav1.UpdateOptions{})
		_, err = h.podClient.Update(pod)
		if err != nil {
			logrus.WithFields(fieldsPod(pod)).
				Warnf("onPodUpdate: failed to update pod label: %v\n%v",
					utils.PrintObject(pod.Labels), err)
			return err
		}
		return nil
	}); err != nil {
		logrus.WithFields(fieldsPod(pod)).
			Errorf("onPodUpdate: failed to update pod label: %v", err)
		return pod, err
	}
	logrus.WithFields(fieldsPod(pod)).
		Infof("Successfully sync pod [%v/%v]", pod.Namespace, pod.Name)
	return pod, nil
}

// checkMacvlanIPInitialized checks if the macvlanip of this pod already exists
// and match the selectedIP label.
// If the macvlanip does not match the selected ip,
// the errSelectedIPMismatch error will be returned.
func (h *handler) checkMacvlanIPInitialized(pod *corev1.Pod) (bool, error) {
	if pod.Labels == nil || pod.Labels[macvlanv1.LabelSelectedIP] == "" {
		return false, nil
	}

	selectedIP := pod.Labels[macvlanv1.LabelSelectedIP]
	macvlanip, err := h.macvlanIPCache.Get(pod.Namespace, pod.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Warnf("MacvlanIP [%v/%v] not found: %v",
				pod.Namespace, pod.Name, err)
			logrus.Infof("Pod [%v/%v] already selected ip [%v], waiting for MacvlanIP resource create",
				pod.Namespace, pod.Name, selectedIP)
			return false, err
		}
		return false, fmt.Errorf("failed to get macvlanip [%v/%v]: %w",
			pod.Namespace, pod.Name, err)
	}

	if strings.SplitN(macvlanip.Spec.CIDR, "/", 2)[0] == selectedIP {
		logrus.Infof("MacvlanIP [%v/%v] exist, CIDR [%s] selectedIP [%s]",
			macvlanip.Namespace, macvlanip.Name, macvlanip.Spec.CIDR, selectedIP)
		return true, nil
	}
	logrus.Warnf("MacvlanIP [%v/%v] mismatch, expected [%v], actual [%v]",
		macvlanip.Namespace, macvlanip.Name, selectedIP, macvlanip.Spec.CIDR)
	return false, errSelectedIPMismatch
}

func (h *handler) validateSubnetProject(subnet *macvlanv1.MacvlanSubnet, pod *corev1.Pod) error {
	ns, err := h.namespaceCache.Get(pod.Namespace)
	if err != nil {
		return err
	}

	if ns.Annotations == nil {
		// not in rancher project
		return nil
	}

	podProject, exist := ns.Annotations["field.cattle.io/projectId"]
	if !exist {
		// not in rancher project
		return nil
	}

	subnetProjectLabel, exist := subnet.Labels["project"]
	if !exist {
		return fmt.Errorf("subnet %s is not own by rancher project", subnet.Name)
	}

	if subnetProjectLabel == "" {
		// All Projects
		return nil
	}

	podProject = strings.Replace(podProject, ":", "-", -1)
	if subnetProjectLabel != podProject {
		return fmt.Errorf("%s(%s) is not own by %s", pod.Name, podProject, subnetProjectLabel)
	}
	return nil
}

func calcHash(ip, mac string) string {
	return fmt.Sprintf("hash-%x", sha1.Sum([]byte(ip+mac)))
}

func (h *handler) findOwnerWorkload(pod *corev1.Pod) (string, string, types.UID, error) {
	for _, owner := range pod.OwnerReferences {
		switch owner.Kind {
		case "ReplicaSet":
			rs, err := h.replicaSetCache.Get(pod.Namespace, owner.Name)
			if err != nil {
				return "", "", "", err
			}
			if rs.OwnerReferences == nil || len(rs.OwnerReferences) < 1 {
				return "", "", "", fmt.Errorf("pod owner is empty")
			}
			if rs.OwnerReferences[0].Kind != "Deployment" {
				return "", "", "", fmt.Errorf("pod owner is invalid kind: %s", rs.OwnerReferences[0].Kind)
			}
			o, err := h.getAppsV1Object("Deployment", pod.Namespace, rs.OwnerReferences[0].Name)
			if err != nil {
				return "", "", "", err
			}
			return o.GetName(), rs.OwnerReferences[0].Name, o.GetUID(), nil
		default:
			o, err := h.getAppsV1Object(owner.Kind, pod.Namespace, owner.Name)
			if err != nil {
				return "", "", "", err
			}
			return o.GetName(), owner.Kind, o.GetUID(), nil
		}
	}
	return "", "", "", fmt.Errorf("%s owner workload not found", pod.Name)
}

func (h *handler) getAppsV1Object(kind, namespace, name string) (metav1.Object, error) {
	switch strings.ToLower(kind) {
	case "daemonset":
		o, err := h.daemonSetCache.Get(namespace, name)
		if err != nil {
			return nil, err
		}
		return o, nil
	case "deployment":
		o, err := h.deploymentCache.Get(namespace, name)
		if err != nil {
			return nil, err
		}
		return o, nil
	case "replicaset":
		o, err := h.replicaSetCache.Get(namespace, name)
		if err != nil {
			return nil, err
		}
		return o, nil
	case "statefulset":
		o, err := h.statefulSetCache.Get(namespace, name)
		if err != nil {
			return nil, err
		}
		return o, nil
	}
	return nil, fmt.Errorf("getAppName: unrecognized kind: %v", kind)
}

func (h *handler) setIfStatefulSetOwnerRef(macvlanip *macvlanv1.MacvlanIP, pod *corev1.Pod) {
	ownerName, ownerKind, ownerUID, err := h.findOwnerWorkload(pod)
	if err != nil {
		return
	}

	if ownerKind == "StatefulSet" {
		logrus.Infof("%s is own by workload %s", pod.Name, ownerName)
		controller := true
		macvlanip.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: "v1",
				Kind:       "StatefulSet",
				UID:        ownerUID,
				Name:       ownerName,
				Controller: &controller,
			},
		}
	}
}

func (h *handler) setWorkloadAndProjectLabel(macvlanip *macvlanv1.MacvlanIP, pod *corev1.Pod) {
	// get name from pod's owner
	ns, err := h.namespaceCache.Get(pod.Namespace)
	if err != nil {
		return
	}

	if macvlanip.Labels == nil {
		macvlanip.Labels = map[string]string{}
	}
	macvlanip.Labels[macvlanv1.LabelProjectID] = ns.Labels[macvlanv1.LabelProjectID]
	macvlanip.Labels[macvlanv1.LabelWorkloadSelector] = pod.Labels[macvlanv1.LabelWorkloadSelector]

	if macvlanip.Labels[macvlanv1.LabelWorkloadSelector] == "" {
		if pod.OwnerReferences != nil {
			for _, podOwner := range pod.OwnerReferences {
				switch podOwner.Kind {
				case "Job":
					j, err := h.jobCache.Get(pod.Namespace, podOwner.Name)
					if err != nil {
						return
					}
					if j.OwnerReferences == nil || len(j.OwnerReferences) == 0 {
						macvlanip.Labels[macvlanv1.LabelWorkloadSelector] = fmt.Sprintf("%s-%s-%s", "job", pod.Namespace, j.Name)
						return
					}
					for _, jobOwner := range j.OwnerReferences {
						switch jobOwner.Kind {
						case "CronJob":
							macvlanip.Labels[macvlanv1.LabelWorkloadSelector] = fmt.Sprintf("%s-%s-%s", "cronjob", pod.Namespace, jobOwner.Name)
							return
						}
					}
				}
			}
		}
	}
}

// onPodRemove deletes macvlanIP resource when the macvlan pod deleted.
func (h *handler) onPodRemove(name string, pod *corev1.Pod) (*corev1.Pod, error) {
	if !utils.IsMacvlanPod(pod) {
		return pod, nil
	}
	logrus.WithFields(fieldsPod(pod)).
		Infof("Pod [%v/%v] deleted", pod.Namespace, pod.Name)

	return pod, nil
}

func fieldsPod(pod *corev1.Pod) logrus.Fields {
	if pod == nil {
		return logrus.Fields{}
	}
	return logrus.Fields{
		"POD": fmt.Sprintf("%v/%v", pod.Namespace, pod.Name),
	}
}

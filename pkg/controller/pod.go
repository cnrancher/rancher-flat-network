package controller

import (
	"crypto/sha1"
	"fmt"
	"net"
	"strings"
	"time"

	macvlanv1 "github.com/cnrancher/macvlan-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

func isMacvlanPod(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	if _, ok := pod.GetAnnotations()[macvlanv1.AnnotationIP]; !ok {
		return false
	}
	if _, ok := pod.GetAnnotations()[macvlanv1.AnnotationSubnet]; !ok {
		return false
	}
	return true
}

func (h *Handler) handlePodError(
	onChange func(string, *corev1.Pod) (*corev1.Pod, error),
) func(string, *corev1.Pod) (*corev1.Pod, error) {
	return func(key string, pod *corev1.Pod) (*corev1.Pod, error) {
		var err error
		pod, err = onChange(key, pod)
		if pod == nil {
			return pod, err
		}
		if err != nil {
			// Avoid trigger the rate limit.
			logrus.Warnf("%v", err)
			time.Sleep(time.Second * 1)

			// TODO: handle error event here.
		}

		return pod, err
	}
}

func (h *Handler) onPodChanged(s string, pod *corev1.Pod) (*corev1.Pod, error) {
	if pod == nil || pod.Name == "" || pod.DeletionTimestamp != nil {
		return pod, nil
	}
	if !isMacvlanPod(pod) {
		return pod, nil
	}

	switch pod.Status.Phase {
	case corev1.PodFailed:
		logrus.Infof("Ignore to sync the failed pod [%v/%v]",
			pod.Namespace, pod.Name)
		return pod, nil
	default:
		return h.syncMacvlanPodIP(s, pod)
	}
}

func (h *Handler) onPodRemoved(_ string, pod *corev1.Pod) (*corev1.Pod, error) {
	return pod, nil
}

func (h *Handler) syncMacvlanPodIP(s string, pod *corev1.Pod) (*corev1.Pod, error) {
	if err := h.checkMacvlanServiceDisabled(pod); err != nil {
		return pod, err
	}

	if pod.Labels != nil && pod.Labels[macvlanv1.LabelSelectedIP] != "" {
		selectedIP := pod.Labels[macvlanv1.LabelSelectedIP]
		// macvlanip, err := c.macvlanipsLister.MacvlanIPs(pod.Namespace).Get(pod.Name)
		macvlanip, err := h.macvlanIPs.Get(pod.Namespace, pod.Name, metav1.GetOptions{})
		if err != nil {
			// itemKey, err := cache.MetaNamespaceKeyFunc(pod)
			// if err != nil {
			// 	return err
			// }
			// if c.workqueue.NumRequeues(itemKey) > maxRetrySelectedIPPod {
			// 	logrus.Warnf("syncMacvlanPodIP: data of the pod and macvlanip is not synchronized, delete the pod %s", itemKey)
			// 	c.kubeClientset.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
			// }
			// return fmt.Errorf("macvlanip %s cannot be found, will try on next queue item", pod.Name)

			logrus.Infof("syncMacvlanPodIP: Already selected ip [%s] for pod [%s/%s], waiting for macvlanip resource created",
				selectedIP, pod.Namespace, pod.Name)
			h.podEnqueueAfter(pod.Namespace, pod.Name, time.Second)
			return pod, nil
		}
		match := strings.SplitN(macvlanip.Spec.CIDR, "/", 2)[0] == selectedIP
		if match {
			logrus.Infof("syncMacvlanPodIP: macvlanip [%s] exist, cidr [%s], selected ip [%s]",
				macvlanip.Name, macvlanip.Spec.CIDR, selectedIP)
			return pod, nil
		}
		logrus.Warnf("syncMacvlanPodIP: macvlanip [%s] mismatch: cidr [%s], selected ip [%s]",
			macvlanip.Name, macvlanip.Spec.CIDR, selectedIP)

		// TODO: Delete the pod directly is not recommended.
		if err := h.pods.Delete(pod.Namespace, pod.Name, &metav1.DeleteOptions{}); err != nil {
			logrus.Warnf("syncMacvlanPodIP: failed to delete pod [%v]: %v", pod.Name, err)
			return pod, err
		}
		logrus.Infof("Request to delete the IP mismatch pod %q", pod.Name)
		if err := h.macvlanIPs.Delete(pod.Namespace, pod.Name, &metav1.DeleteOptions{}); err != nil {
			logrus.Warnf("syncMacvlanPodIP: failed to delete macvlan ip [%v]: %v", pod.Name, err)
			return pod, err
		}
		logrus.Infof("Request to delete the mismatch macvlanip resource %q", pod.Name)
		return pod, fmt.Errorf("macvlanip [%s] mismatch, will try on next queue item", pod.Name)
	}

	annotationIP := pod.Annotations[macvlanv1.AnnotationIP]
	annotationSubnet := pod.Annotations[macvlanv1.AnnotationSubnet]
	annotationMac := pod.Annotations[macvlanv1.AnnotationMac]
	subnet, err := h.macvlanSubnets.Get(macvlanv1.MacvlanSubnetNamespace, annotationSubnet, metav1.GetOptions{})
	if err != nil {
		return pod, fmt.Errorf("syncMacvlanPodIP: failed to get subnet %q of pod [%v]: %w",
			annotationSubnet, s)
	}
	if err := h.validateSubnetProject(subnet, pod); err != nil {
		return pod, fmt.Errorf("syncMacvlanPodIP: failed to validate the subnet %q of pod [%v]: %w",
			annotationSubnet, s)
	}

	// allocate ip in subnet
	var allocatedIP net.IP
	var macvlanipCIDR string
	var macvlanipMac string
	macvlanipType := "specific"
	if annotationMac == "auto" {
		annotationMac = ""
	}

	existMacvlanIP, _ := h.macvlanIPs.Get(pod.Namespace, pod.Name, metav1.GetOptions{})
	if annotationIP == "auto" {
		logrus.Infof("Alloate ip mode auto for pod [%s/%s]", pod.Namespace, pod.Name)
		macvlanipType = "auto"
		if existMacvlanIP != nil {
			// For statefulset pod.
			macvlanipCIDR = existMacvlanIP.Spec.CIDR
			macvlanipMac = existMacvlanIP.Spec.MAC
			allocatedIP, _, _ = net.ParseCIDR(existMacvlanIP.Spec.CIDR)
		} else {
			allocatedIP, macvlanipCIDR, macvlanipMac, err = h.allocateAutoIP(pod, subnet, annotationMac)
		}
	} else if isSingleIP(annotationIP) {
		logrus.Infof("Alloate single macvlan IP for pod [%v]", s)
		allocatedIP, macvlanipCIDR, macvlanipMac, err = h.allocateSingleIP(pod, subnet, annotationIP, annotationMac)
	} else if isMultipleIP(annotationIP) {
		logrus.Infof("Alloate multiple macvlan IP for pod [%v]", s)
		allocatedIP, macvlanipCIDR, macvlanipMac, err = h.allocateMultipleIP(pod, subnet, annotationIP, annotationMac)
	} else {
		return pod, fmt.Errorf("syncMacvlanPodIP: the ip value %q of annotation %q on pod %q is invalid",
			annotationIP, macvlanv1.AnnotationIP, s)
	}
	if err != nil {
		return pod, fmt.Errorf("syncMacvlanPodIP: failed to allocate IP for pod %q: %w", s, err)
	}
	if allocatedIP == nil {
		return pod, fmt.Errorf("syncMacvlanPodIP: failed to allocate IP for pod %q: allocated IP is nil", s)
	}

	key := fmt.Sprintf("%s:%s", allocatedIP.String(), subnet.Name)
	owner := fmt.Sprintf("%s:%s", pod.Namespace, pod.Name)
	logrus.Infof("syncMacvlanPodIP: finished to allocate IP: CIDR[%s] MAC[%s] OWNER[%s]", macvlanipCIDR, macvlanipMac, owner)
	h.inUsedIPs.Store(key, owner)
	logrus.Debugf("syncMacvlanPodIP: set syncmap cache, key: %s, value: %s", key, owner)
	if macvlanipMac != "" && macvlanipType == "auto" && annotationMac != "" {
		h.inUsedMacForAuto.Store(macvlanipMac, owner)
		logrus.Debugf("syncMacvlanPodIP: set inUsedMacForAuto cache, key: %s, value: %s", macvlanipMac, owner)
	}

	// Update the label of macvlanip (ip, selectedip).
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err := h.pods.Get(pod.Namespace, pod.Name, metav1.GetOptions{})
		if err != nil {
			logrus.Warnf("syncMacvlanPodIP: failed to get latest version of Pod: %v", err)
			return err
		}

		result = result.DeepCopy()
		if result.Labels == nil {
			result.Labels = map[string]string{}
		}

		hash := calcHash(annotationIP, annotationMac)
		result.Labels[macvlanv1.LabelMultipleIPHash] = hash
		result.Labels[macvlanv1.LabelSelectedIP] = allocatedIP.String()
		result.Labels[macvlanv1.LabelSelectedMac] = strings.Replace(macvlanipMac, ":", "_", -1)
		result.Labels[macvlanv1.LabelMacvlanIPType] = macvlanipType
		result.Labels[macvlanv1.LabelSubnet] = annotationSubnet

		_, err = h.pods.Update(result)
		if err != nil {
			logrus.Warnf("syncMacvlanPodIP: failed to update pod IP: %v %v", result.Labels, err)
			return err
		}
		return nil
	})
	if err != nil {
		if h.deleteKeyFromInUsedIPCache(key, owner) {
			logrus.Infof("syncMacvlanPodIP: delete key %s from syncmap", key)
		}
		if h.deleteKeyFromInUsedMacCache(macvlanipMac, owner) {
			logrus.Infof("syncMacvlanPodIP: delete key %q from inUsedMacForAuto", macvlanipMac)
		}
		logrus.Errorf("syncMacvlanPodIP: pod update labels retry error: %v", err)
		return pod, fmt.Errorf("syncMacvlanPodIP: failed to update pod label")
	}

	// Create macvlanip resource.
	macvlanip := makeMacvlanIP(pod, subnet, macvlanipCIDR, macvlanipMac, macvlanipType)
	// Add statefulset support.
	h.setIfStatefulSetOwnerRef(macvlanip, pod)
	// Set workload/project label.
	h.setWorkloadAndProjectLabel(macvlanip, pod)
	// Add finalizer.
	if macvlanipType == "auto" && subnet.Spec.IPDelayReuse != 0 {
		macvlanip = addMacvlanIPDelayReuseFinalizer(macvlanip)
	}

	if existMacvlanIP != nil {
		// For statefulset pod
		// TODO: need enhancement here instead of edit the resource version directly.
		macvlanip.ResourceVersion = existMacvlanIP.ResourceVersion
		_, err = h.macvlanIPs.Update(macvlanip)
	} else {
		_, err = h.macvlanIPs.Create(macvlanip)
	}
	if err != nil {
		if h.deleteKeyFromInUsedIPCache(key, owner) {
			logrus.Infof("syncMacvlanPodIP: done to delete key %s from syncmap", key)
		}
		if h.deleteKeyFromInUsedMacCache(macvlanipMac, owner) {
			logrus.Infof("syncMacvlanPodIP: done to delete key %s from inUsedMacForAuto", macvlanipMac)
		}
		logrus.Errorf("syncMacvlanPodIP: failed to sync macvlanip CRD: %v", err)
		return pod, err
	}
	logrus.Infof("syncMacvlanPodIP: sync macvlanIP %s %s", macvlanipCIDR, owner)

	// Auto sync service
	if err = h.syncService(pod); err != nil {
		logrus.Errorf("syncMacvlanPodIP: sync service error: %v", err)
	}
	return pod, nil
}

func (h *Handler) checkMacvlanServiceDisabled(pod *corev1.Pod) error {
	if pod.Annotations == nil {
		return nil
	}

	if pod.Annotations[macvlanv1.AnnotationMacvlanService] == "disable" &&
		pod.Annotations[macvlanv1.AnnotationIP] == "" {
		ownerName, _, _, err := h.findOwnerWorkload(pod)
		if err != nil {
			return nil
		}

		macvlanSvcName := fmt.Sprintf("%s-macvlan", ownerName)
		_, err = h.services.Get(pod.Namespace, macvlanSvcName, metav1.GetOptions{})
		if err != nil {
			// TODO: CHECK ERROR TYPE
			return nil
		}
		logrus.Infof("checkMacvlanServiceDisabled: deleting service %s - %s", pod.Name, macvlanSvcName)
		// err = c.kubeClientset.CoreV1().Services(pod.Namespace).Delete(context.TODO(), macvlanSvcName, metav1.DeleteOptions{})
		err = h.services.Delete(pod.Namespace, macvlanSvcName, &metav1.DeleteOptions{})
		if err != nil {
			logrus.Infof("checkMacvlanServiceDisabled: deleting service err %s %v", macvlanSvcName, err)
		}
	}
	return nil
}

func (h *Handler) setWorkloadAndProjectLabel(macvlanip *macvlanv1.MacvlanIP, pod *corev1.Pod) {
	// get name from pod's owner
	// ns, err := h.kubeClientset.CoreV1().Namespaces().Get(context.TODO(), pod.Namespace, metav1.GetOptions{})
	ns, err := h.namespaces.Get(pod.Namespace, metav1.GetOptions{})
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
					// j, err := h.kubeClientset.BatchV1().Jobs(pod.Namespace).Get(context.TODO(), podOwner.Name, metav1.GetOptions{})
					j, err := h.jobs.Get(pod.Namespace, podOwner.Name, metav1.GetOptions{})
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

func (h *Handler) validateSubnetProject(subnet *macvlanv1.MacvlanSubnet, pod *corev1.Pod) error {
	ns, err := h.namespaces.Get(pod.Namespace, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if ns.Annotations == nil {
		// Not in rancher project.
		return nil
	}
	podProject, exist := ns.Annotations["field.cattle.io/projectId"]
	if !exist {
		// Not in rancher project.
		return nil
	}
	if subnet.Labels == nil {
		return fmt.Errorf("subnet %q is not owned by rancher project", subnet.Name)
	}
	subnetProjectLabel, ok := subnet.Labels["project"]
	if !ok {
		return fmt.Errorf("subnet %q is not owned by rancher project", subnet.Name)
	}
	if subnetProjectLabel == "" {
		// All Projects
		return nil
	}
	podProject = strings.ReplaceAll(podProject, ":", "-")
	if subnetProjectLabel != podProject {
		return fmt.Errorf("%s(%s) is not own by %s", pod.Name, podProject, subnetProjectLabel)
	}
	return nil
}

func makeMacvlanIP(pod *corev1.Pod, subnet *macvlanv1.MacvlanSubnet, cidr, mac, macvlanipType string) *macvlanv1.MacvlanIP {
	controller := true
	macvlanip := &macvlanv1.MacvlanIP{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Labels: map[string]string{
				"subnet":                     subnet.Name,
				macvlanv1.LabelMacvlanIPType: macvlanipType,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "Pod",
					UID:        pod.UID,
					Name:       pod.Name,
					Controller: &controller,
				},
			},
		},
		Spec: macvlanv1.MacvlanIPSpec{
			CIDR:   cidr,
			MAC:    mac,
			PodID:  string(pod.GetUID()),
			Subnet: subnet.Name,
		},
	}

	if subnet.Annotations[macvlanv1.AnnotationsIPv6to4] != "" {
		macvlanip.Annotations = map[string]string{}
		macvlanip.Annotations[macvlanv1.AnnotationsIPv6to4] = "true"
	}

	return macvlanip
}

func (h *Handler) setIfStatefulSetOwnerRef(macvlanip *macvlanv1.MacvlanIP, pod *corev1.Pod) {
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

func (h *Handler) deleteKeyFromInUsedIPCache(key, owner string) bool {
	if v, ok := h.inUsedIPs.Load(key); ok {
		if v == owner {
			h.inUsedIPs.Delete(key)
			logrus.Infof("deleteKeyFromInUsedIPCache: delete key %s from syncmap, %s", key, owner)
			return true
		}
		if temp := strings.SplitN(v.(string), ":", 2); len(temp) == 2 {
			// use api client to get the latest resource version
			// pod, err := h.kubeClientset.CoreV1().Pods(temp[0]).Get(context.TODO(), temp[1], metav1.GetOptions{})
			pod, err := h.pods.Get(temp[0], temp[1], metav1.GetOptions{})
			if (err != nil && k8serrors.IsNotFound(err)) || (pod != nil && pod.DeletionTimestamp != nil) {
				h.inUsedIPs.Delete(key)
				logrus.Infof("deleteKeyFromInUsedIPCache: delete key %s from syncmap, %s, as pod is not found", key, owner)
				return true
			}
		}
	}
	return false
}

func (h *Handler) deleteKeyFromInUsedMacCache(key, owner string) bool {
	if v, ok := h.inUsedMacForAuto.Load(key); ok {
		if v == owner {
			h.inUsedMacForAuto.Delete(key)
			logrus.Infof("deleteKeyFromInUsedMacCache: delete key %s from inUsedMacForAuto, %s", key, owner)
			return true
		}
		temp := strings.SplitN(v.(string), ":", 2)
		// use api client to get the latest resource version
		// pod, err := c.kubeClientset.CoreV1().Pods(temp[0]).Get(context.TODO(), temp[1], metav1.GetOptions{})
		pod, err := h.pods.Get(temp[0], temp[1], metav1.GetOptions{})
		if (err != nil && k8serrors.IsNotFound(err)) || (pod != nil && pod.DeletionTimestamp != nil) {
			h.inUsedMacForAuto.Delete(key)
			logrus.Infof("deleteKeyFromInUsedMacCache: delete key %s from inUsedMacForAuto, %s, as pod is not found", key, owner)
			return true
		}
	}
	return false
}

func addMacvlanIPDelayReuseFinalizer(ip *macvlanv1.MacvlanIP) *macvlanv1.MacvlanIP {
	ip = ip.DeepCopy()
	if ip.ObjectMeta.Finalizers == nil {
		ip.ObjectMeta.Finalizers = []string{}
	}
	for _, v := range ip.ObjectMeta.Finalizers {
		if v == macvlanv1.FinalizerIPDelayReuse {
			return ip
		}
	}
	ip.ObjectMeta.Finalizers = append(ip.ObjectMeta.Finalizers, macvlanv1.FinalizerIPDelayReuse)
	return ip
}

func calcHash(ip, mac string) string {
	return fmt.Sprintf("hash-%x", sha1.Sum([]byte(ip+mac)))
}

package pod

import (
	"fmt"
	"strings"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

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

func (h *handler) setIfStatefulSetOwnerRef(macvlanIP *macvlanv1.MacvlanIP, pod *corev1.Pod) {
	ownerName, ownerKind, ownerUID, err := h.findOwnerWorkload(pod)
	if err != nil {
		return
	}

	if ownerKind == "StatefulSet" {
		logrus.Infof("%s is own by workload %s", pod.Name, ownerName)
		controller := true
		macvlanIP.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
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

func (h *handler) setWorkloadAndProjectLabel(macvlanIP *macvlanv1.MacvlanIP, pod *corev1.Pod) {
	// get name from pod's owner
	ns, err := h.namespaceCache.Get(pod.Namespace)
	if err != nil {
		return
	}

	if macvlanIP.Labels == nil {
		macvlanIP.Labels = map[string]string{}
	}
	macvlanIP.Labels[macvlanv1.LabelProjectID] = ns.Labels[macvlanv1.LabelProjectID]
	macvlanIP.Labels[macvlanv1.LabelWorkloadSelector] = pod.Labels[macvlanv1.LabelWorkloadSelector]

	if macvlanIP.Labels[macvlanv1.LabelWorkloadSelector] == "" {
		if pod.OwnerReferences != nil {
			for _, podOwner := range pod.OwnerReferences {
				switch podOwner.Kind {
				case "Job":
					j, err := h.jobCache.Get(pod.Namespace, podOwner.Name)
					if err != nil {
						return
					}
					if j.OwnerReferences == nil || len(j.OwnerReferences) == 0 {
						macvlanIP.Labels[macvlanv1.LabelWorkloadSelector] = fmt.Sprintf("%s-%s-%s", "job", pod.Namespace, j.Name)
						return
					}
					for _, jobOwner := range j.OwnerReferences {
						switch jobOwner.Kind {
						case "CronJob":
							macvlanIP.Labels[macvlanv1.LabelWorkloadSelector] = fmt.Sprintf("%s-%s-%s", "cronjob", pod.Namespace, jobOwner.Name)
							return
						}
					}
				}
			}
		}
	}
}

package pod

import (
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
)

func (h *handler) eventMacvlanIPError(pod *corev1.Pod, err error) {
	h.recorder.Event(pod, corev1.EventTypeWarning, "MacvlanIPError", err.Error())
}

func (h *handler) eventMacvlanSubnetError(pod *corev1.Pod, err error) {
	h.recorder.Event(pod, corev1.EventTypeWarning, "MacvlanSubnetError", err.Error())
}

func makeMacvlanIP(
	pod *corev1.Pod, subnet *macvlanv1.MacvlanSubnet, cidr, mac, macvlanipType string,
) *macvlanv1.MacvlanIP {
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

func macvlanIPUpdated(a, b *macvlanv1.MacvlanIP) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Name != b.Name || a.Namespace != b.Namespace {
		logrus.Debugf("ip name/namespace of [%v/%v] mismatch", a.Namespace, a.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(a.OwnerReferences, b.OwnerReferences) {
		logrus.Debugf("ip OwnerReferences of [%v/%v] mismatch", a.Namespace, a.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(a.Labels, b.Labels) {
		logrus.Debugf("ip Labels of [%v/%v] mismatch", a.Namespace, a.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(a.Annotations, b.Annotations) {
		logrus.Debugf("ip Annotations of [%v/%v] mismatch", a.Namespace, a.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(a.Spec, b.Spec) {
		logrus.Debugf("ip Spec of [%v/%v] mismatch", a.Namespace, a.Name)
		return false
	}
	return true
}

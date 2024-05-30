package pod

import (
	"crypto/sha1"
	"fmt"
	"net"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/utils"
)

func (h *handler) eventMacvlanIPError(pod *corev1.Pod, err error) {
	h.recorder.Event(pod, corev1.EventTypeWarning, "MacvlanIPError", err.Error())
}

func (h *handler) eventMacvlanSubnetError(pod *corev1.Pod, err error) {
	h.recorder.Event(pod, corev1.EventTypeWarning, "MacvlanSubnetError", err.Error())
}

// newMacvlanIP returns a new macvlanIP struct object by Pod.
func (h *handler) newMacvlanIP(pod *corev1.Pod) (*macvlanv1.MacvlanIP, error) {
	// Valid pod annotation
	annotationIP := pod.Annotations[macvlanv1.AnnotationIP]
	annotationSubnet := pod.Annotations[macvlanv1.AnnotationSubnet]
	annotationMac := pod.Annotations[macvlanv1.AnnotationMac]
	macvlanIPType := "specific"
	switch annotationIP {
	case "auto":
		macvlanIPType = "auto"
	default:
		if !utils.IsSingleIP(annotationIP) && !utils.IsMultipleIP(annotationIP) {
			return nil, fmt.Errorf("newMacvlanIP: invalid annotation [%v: %v]",
				macvlanv1.AnnotationIP, annotationIP)
		}
	}
	subnet, err := h.macvlanSubnetCache.Get(macvlanv1.MacvlanSubnetNamespace, annotationSubnet)
	if err != nil {
		return nil, fmt.Errorf("newMacvlanIP: failed to get subnet [%v]: %w",
			annotationSubnet, err)
	}
	var mac net.HardwareAddr
	if annotationMac != "" {
		mac, err = net.ParseMAC(annotationMac)
		if err != nil {
			return nil, fmt.Errorf("newMacvlanIP: failed to parse mac [%v]: %w",
				annotationMac, err)
		}
	}

	controller := true
	macvlanIP := &macvlanv1.MacvlanIP{
		ObjectMeta: metav1.ObjectMeta{
			Name:        pod.Name,
			Namespace:   pod.Namespace,
			Annotations: map[string]string{},
			Labels: map[string]string{
				"subnet":                     subnet.Name,
				macvlanv1.LabelMacvlanIPType: macvlanIPType,
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
			IP:     annotationIP,
			MAC:    mac,
			PodID:  string(pod.GetUID()),
			Subnet: subnet.Name,
		},
	}
	if subnet.Annotations[macvlanv1.AnnotationsIPv6to4] != "" {
		macvlanIP.Annotations[macvlanv1.AnnotationsIPv6to4] = "true"
	}
	return macvlanIP, nil
}

func calcHash(ip, mac string) string {
	return fmt.Sprintf("hash-%x", sha1.Sum([]byte(ip+mac)))
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

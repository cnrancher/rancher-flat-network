package pod

import (
	"crypto/sha1"
	"fmt"
	"net"
	"strings"

	flv1 "github.com/cnrancher/flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// newFlatNetworkIP returns a new flat-network IP struct object by Pod.
func (h *handler) newFlatNetworkIP(pod *corev1.Pod) (*flv1.FlatNetworkIP, error) {
	// Valid pod annotation
	annotationIP := pod.Annotations[flv1.AnnotationIP]
	annotationMAC := pod.Annotations[flv1.AnnotationMac]
	annotationSubnet := pod.Annotations[flv1.AnnotationSubnet]
	flatNetworkIPType := "specific"

	var (
		ipAddrs  []net.IP
		macAddrs []net.HardwareAddr
	)
	switch annotationIP {
	case "auto":
		flatNetworkIPType = "auto"
	default:
		spec := strings.Split(annotationIP, "-")
		for _, s := range spec {
			a := net.ParseIP(s)
			if len(a) == 0 {
				return nil, fmt.Errorf("newFlatNetworkIP: invalid annotation [%v: %v]",
					flv1.AnnotationIP, annotationIP)
			}
			ipAddrs = append(ipAddrs, a)
		}
	}
	if annotationMAC != "" {
		spec := strings.Split(annotationIP, "-")
		for _, s := range spec {
			a, err := net.ParseMAC(s)
			if len(a) == 0 || err != nil {
				return nil, fmt.Errorf("newFlatNetworkIP: invalid annotation [%v: %v]",
					flv1.AnnotationMac, annotationMAC)
			}
			macAddrs = append(macAddrs, a)
		}
	}
	subnet, err := h.subnetCache.Get(flv1.SubnetNamespace, annotationSubnet)
	if err != nil {
		return nil, fmt.Errorf("newFlatNetworkIP: failed to get subnet [%v]: %w",
			annotationSubnet, err)
	}

	flatNetworkIP := &flv1.FlatNetworkIP{
		ObjectMeta: metav1.ObjectMeta{
			Name:        pod.Name,
			Namespace:   pod.Namespace,
			Annotations: map[string]string{},
			Labels: map[string]string{
				"subnet":                    subnet.Name,
				flv1.LabelFlatNetworkIPType: flatNetworkIPType,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "Pod",
					UID:        pod.UID,
					Name:       pod.Name,
					Controller: utils.Pointer(true),
				},
				{
					APIVersion: "v1",
					Kind:       subnet.Kind,
					UID:        subnet.UID,
					Name:       subnet.Name,
				},
			},
		},
		Spec: flv1.IPSpec{
			Addrs:  ipAddrs,
			MACs:   macAddrs,
			PodID:  string(pod.GetUID()),
			Subnet: subnet.Name,
		},
	}
	if subnet.Annotations[flv1.AnnotationsIPv6to4] != "" {
		flatNetworkIP.Annotations[flv1.AnnotationsIPv6to4] = "true"
	}
	return flatNetworkIP, nil
}

func calcHash(ip, mac string) string {
	return fmt.Sprintf("hash-%x", sha1.Sum([]byte(ip+mac)))
}

func flatNetworkIPUpdated(a, b *flv1.FlatNetworkIP) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Name != b.Name || a.Namespace != b.Namespace {
		logrus.Debugf("ip namespace/name [%v/%v] != [%v/%v]",
			a.Namespace, a.Name, b.Namespace, b.Name)
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

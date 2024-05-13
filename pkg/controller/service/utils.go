package service

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/cnrancher/flat-network-operator/pkg/utils"
	"github.com/sirupsen/logrus"
)

const (
	k8sCNINetworksKey = "k8s.v1.cni.cncf.io/networks"
	netAttatchDefName = "static-macvlan-cni-attach"
)

// makeMacvlanService returns a macvlan headless sercive struct based on
// the provided existing service.
func makeMacvlanService(svc *corev1.Service) *corev1.Service {
	ports := []corev1.ServicePort{}

	for _, v := range svc.Spec.Ports {
		port := v.DeepCopy()
		if svc.Spec.ClusterIP == corev1.ClusterIPNone {
			port.Port = port.Port + 1
			port.TargetPort = intstr.FromInt(port.TargetPort.IntValue() + 1)
		}
		ports = append(ports, *port)
	}

	specCopy := svc.Spec.DeepCopy()
	specCopy.ClusterIP = corev1.ClusterIPNone

	// Make this service headless.
	// Setting this to "None" makes a "headless service" (no virtual IP),
	// which is useful when direct endpoint connections are preferred and
	// proxying is not required.
	specCopy.ClusterIPs = []string{"None"}
	specCopy.Ports = ports

	s := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            fmt.Sprintf("%s%s", svc.Name, macvlanServiceNameSuffix),
			Namespace:       svc.Namespace,
			OwnerReferences: svc.OwnerReferences,
			Annotations: map[string]string{
				k8sCNINetworksKey: netAttatchDefName,
			},
		},
		Spec: *specCopy,
	}

	return s
}

// macvlanServiceUpdated returns true if the macvlan service already updated
func macvlanServiceUpdated(a, b *corev1.Service) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Name != b.Name || a.Namespace != b.Namespace {
		logrus.Debugf("service [%v/%v] name/namespace mismatch [%v/%v]",
			a.Namespace, a.Name, b.Namespace, b.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(a.OwnerReferences, b.OwnerReferences) {
		logrus.Debugf("service [%v/%v] OwnerReferences mismatch, a: %v\nb: %v",
			a.Namespace, a.Name,
			utils.PrintObject(a.OwnerReferences), utils.PrintObject(b.OwnerReferences))
		return false
	}
	if !equality.Semantic.DeepEqual(a.Annotations, b.Annotations) {
		logrus.Debugf("service [%v/%v] Annotations mismatch: a: %v\n b: %v",
			a.Namespace, a.Name,
			utils.PrintObject(a.Annotations), utils.PrintObject(b.Annotations))
		return false
	}
	if a.Spec.ClusterIPs == nil {
		a.Spec.ClusterIPs = []string{"None"}
	}
	if b.Spec.ClusterIPs == nil {
		b.Spec.ClusterIPs = []string{"None"}
	}
	if !equality.Semantic.DeepEqual(a.Spec, b.Spec) {
		logrus.Debugf("service [%v/%v] Spec mismatch: a: %v\nb: %v",
			a.Namespace, a.Name,
			utils.PrintObject(a.Spec), utils.PrintObject(b.Spec))
		return false
	}
	return true
}

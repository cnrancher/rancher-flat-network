package service

import (
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	nettypes "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
)

// isIngressService detects if this svc is created by Rancher for ingress.
// Reserved: Only manager UI have this feature and will remove in upcoming release
func isIngressService(svc *corev1.Service) bool {
	if svc == nil || svc.Name == "" {
		return false
	}
	if !strings.HasPrefix(svc.Name, "ingress-") {
		return false
	}

	for _, owner := range svc.OwnerReferences {
		if strings.ToLower(owner.Kind) == "ingress" {
			return true
		}
	}

	return false
}

func (h *handler) isWorkloadDisabledFlatNetwork(svc *corev1.Service) (bool, error) {
	if len(svc.Spec.Selector) == 0 {
		return true, nil
	}

	pods, err := h.podCache.List(svc.Namespace, labels.SelectorFromSet(svc.Spec.Selector))
	if err != nil {
		return false, fmt.Errorf("failed to list pod by selector [%v] on service [%v/%v]: %w",
			svc.Spec.Selector, svc.Namespace, svc.Name, err)
	}
	if len(pods) == 0 {
		if time.Since(svc.CreationTimestamp.Time) < time.Second*10 {
			// The workload service may just created and pods of this workload
			// were not created yet. Wait 10 seconds to ensure no pods were
			// selected by this service.
			// TODO: need better logic to optimize if needed.
			h.serviceEnqueueAfter(svc.Namespace, svc.Name, defaultFlatNetworkServiceEnqueue)
		}
		return true, nil
	}

	// pod of this svc disabled flat-network service by annotation.
	for _, pod := range pods {
		if pod == nil {
			continue
		}
		annotations := pod.Annotations
		if annotations != nil && annotations[flv1.AnnotationFlatNetworkService] == "disabled" {
			return true, nil
		}
	}

	// Pod does not use flat-network.
	var podUseFlatNetwork bool
	for _, pod := range pods {
		if pod == nil {
			continue
		}
		if utils.IsPodEnabledFlatNetwork(pod) {
			podUseFlatNetwork = true
			break
		}
	}

	return !podUseFlatNetwork, nil
}

// newFlatNetworkService returns a flat-network headless sercive struct based on
// the provided existing service.
func newFlatNetworkService(svc *corev1.Service) *corev1.Service {
	svc = svc.DeepCopy()
	ports := []corev1.ServicePort{}
	for _, v := range svc.Spec.Ports {
		port := v.DeepCopy()
		port.NodePort = 0
		if svc.Spec.ClusterIP == corev1.ClusterIPNone {
			port.Port = port.Port + 1
			port.TargetPort = intstr.FromInt(port.TargetPort.IntValue() + 1)
		}
		ports = append(ports, *port)
	}

	// The flat-network service is owned by original service.
	ownerReference := svc.OwnerReferences
	ownerReference = append(ownerReference, metav1.OwnerReference{
		APIVersion: "v1",
		Kind:       "Service",
		Name:       svc.Name,
		UID:        svc.UID,
		// Controller: utils.Pointer(true),
	})

	s := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            fmt.Sprintf("%s%s", svc.Name, utils.FlatNetworkServiceNameSuffix),
			Namespace:       svc.Namespace,
			OwnerReferences: ownerReference,
			Annotations: map[string]string{
				nettypes.NetworkAttachmentAnnot: utils.NetAttatchDefName,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports:    ports,
			Selector: maps.Clone(svc.Spec.Selector),
			// Setting this to "None" makes a "headless service" (no virtual IP),
			// which is useful when direct endpoint connections are preferred and
			// proxying is not required.
			ClusterIP:  corev1.ClusterIPNone,
			ClusterIPs: []string{"None"},
			Type:       "ClusterIP",
		},
	}

	return s
}

// flatNetworkServiceUpdated returns true if the flat-network service already updated
func flatNetworkServiceUpdated(a, b *corev1.Service) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Name != b.Name || a.Namespace != b.Namespace {
		logrus.WithFields(fieldsService(a)).
			Debugf("service [%v/%v] name/namespace mismatch [%v/%v]",
				a.Namespace, a.Name, b.Namespace, b.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(a.OwnerReferences, b.OwnerReferences) {
		logrus.WithFields(fieldsService(a)).
			Debugf("service OwnerReferences mismatch, a: %v\nb: %v",
				utils.Print(a.OwnerReferences), utils.Print(b.OwnerReferences))
		return false
	}
	if !equality.Semantic.DeepEqual(a.Annotations, b.Annotations) {
		logrus.WithFields(fieldsService(a)).
			Debugf("service Annotations mismatch: a: %v\n b: %v",
				utils.Print(a.Annotations), utils.Print(b.Annotations))
		return false
	}
	if !equality.Semantic.DeepEqual(a.Spec.Ports, b.Spec.Ports) {
		logrus.WithFields(fieldsService(a)).
			Debugf("service spec.ports mismatch: a: %v\nb: %v",
				utils.Print(a.Spec.Ports), utils.Print(b.Spec.Ports))
		return false
	}
	if !equality.Semantic.DeepEqual(a.Spec.Selector, b.Spec.Selector) {
		logrus.WithFields(fieldsService(a)).
			Debugf("service spec.selector mismatch: a: %v\nb: %v",
				utils.Print(a.Spec.Selector), utils.Print(b.Spec.Selector))
		return false
	}
	if !equality.Semantic.DeepEqual(a.Spec.Selector, b.Spec.Selector) {
		logrus.WithFields(fieldsService(a)).
			Debugf("service spec.selector mismatch: a: %v\nb: %v",
				utils.Print(a.Spec.Selector), utils.Print(b.Spec.Selector))
		return false
	}
	if !equality.Semantic.DeepEqual(a.Spec.ClusterIP, b.Spec.ClusterIP) {
		logrus.WithFields(fieldsService(a)).
			Debugf("service spec.clusterIP mismatch: a: %v\nb: %v",
				utils.Print(a.Spec.ClusterIP), utils.Print(b.Spec.ClusterIP))
		return false
	}
	if !equality.Semantic.DeepEqual(a.Spec.ClusterIPs, b.Spec.ClusterIPs) {
		logrus.WithFields(fieldsService(a)).
			Debugf("service spec.clusterIPs mismatch: a: %v\nb: %v",
				utils.Print(a.Spec.ClusterIPs), utils.Print(b.Spec.ClusterIPs))
		return false
	}
	if !equality.Semantic.DeepEqual(a.Spec.Type, b.Spec.Type) {
		logrus.WithFields(fieldsService(a)).
			Debugf("service spec.type mismatch: a: %v\nb: %v",
				utils.Print(a.Spec.Type), utils.Print(b.Spec.Type))
		return false
	}
	return true
}

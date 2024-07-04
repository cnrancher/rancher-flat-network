package service

import (
	"fmt"
	"strings"

	flv1 "github.com/cnrancher/rancher-flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func (h *handler) handleFlatNetworkService(
	svc *corev1.Service,
) (*corev1.Service, error) {
	logrus.WithFields(fieldsService(svc)).
		Debugf("service is a flat-network service")

	pods, err := h.podCache.List(svc.Namespace, labels.SelectorFromSet(svc.Spec.Selector))
	if err != nil {
		return svc, fmt.Errorf("failed to list pod by selector [%v] on service [%v/%v]: %w",
			svc.Spec.Selector, svc.Namespace, svc.Name, err)
	}
	// Check whether this flat-network service should delete or not.
	ok, err := h.shouldDeleteFlatNetworkService(svc, pods)
	if err != nil {
		logrus.WithFields(fieldsService(svc)).
			Errorf("failed to sync flat-network service: %v", err)
		return nil, err
	}
	if ok {
		logrus.WithFields(fieldsService(svc)).
			Infof("request to delete flat-network service")
		err = h.serviceClient.Delete(svc.Namespace, svc.Name, &metav1.DeleteOptions{})
		if err != nil {
			logrus.WithFields(fieldsService(svc)).
				Errorf("failed to delete flat-network service: %v", err)
			return svc, err
		}
		return svc, nil
	}

	resource, err := h.getEndpointResources(svc, pods)
	if err != nil {
		logrus.WithFields(fieldsService(svc)).
			Errorf("failed to get endpoint resources of service: %v", err)
		return svc, fmt.Errorf("failed to get endpoint resources: %w", err)
	}

	// Update corev1.Endpoints of this service.
	if err = h.syncCoreV1Endpoints(svc, resource); err != nil {
		return svc, err
	}
	// Update discoveryv1.EndpointSlice of this service.
	if err = h.syncDiscoveryV1EndpointSlice(svc, resource); err != nil {
		return svc, err
	}

	// Resync flat-network service in every 5min.
	h.serviceEnqueueAfter(svc.Namespace, svc.Name, defaultRequeueTime)
	return svc, nil
}

func (h *handler) shouldDeleteFlatNetworkService(
	svc *corev1.Service, pods []*corev1.Pod,
) (bool, error) {
	if len(svc.Spec.Selector) == 0 {
		return true, nil
	}

	originalServiceName := strings.TrimSuffix(svc.Name, utils.FlatNetworkServiceNameSuffix)
	originalService, err := h.serviceCache.Get(svc.Namespace, originalServiceName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Delete if no original service.
			logrus.WithFields(fieldsService(svc)).
				Infof("original service of flat-network service [%v/%v] not found",
					svc.Namespace, originalServiceName)
			return true, nil
		}
		return false, fmt.Errorf("failed to get service [%v/%v] from cache: %w",
			svc.Namespace, originalService.Name, err)
	}

	if len(pods) == 0 {
		logrus.WithFields(fieldsService(svc)).
			Infof("no pods on flat-network service [%v/%v]",
				svc.Namespace, svc.Name)
		return true, nil
	}

	// Workload of this svc disabled flat-network service by annotation.
	for _, pod := range pods {
		if pod == nil {
			continue
		}
		annotations := pod.Annotations
		if annotations != nil && annotations[flv1.AnnotationFlatNetworkService] == "disabled" {
			logrus.WithFields(fieldsService(svc)).
				Infof("annotation [%v: disabled] found, flat-network service disabled",
					flv1.AnnotationFlatNetworkService)
			return true, nil
		}
	}

	// Workload does not enabled flat-network.
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
	if !podUseFlatNetwork {
		logrus.WithFields(fieldsService(svc)).
			Infof("workload does not use flat-network")
	}

	return !podUseFlatNetwork, nil
}

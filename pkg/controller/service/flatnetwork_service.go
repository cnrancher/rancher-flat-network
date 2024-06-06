package service

import (
	"fmt"
	"strings"

	flv1 "github.com/cnrancher/flat-network-operator/pkg/apis/flatnetwork.cattle.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/utils"
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

	ok, err := h.shouldDeleteFlatNetworkService(svc)
	if err != nil {
		logrus.WithFields(fieldsService(svc)).
			Errorf("failed to sync flat-network service: %v", err)
		return nil, err
	}
	if !ok {
		return svc, nil
	}
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

func (h *handler) shouldDeleteFlatNetworkService(svc *corev1.Service) (bool, error) {
	if len(svc.Spec.Selector) == 0 {
		return true, nil
	}

	originalServiceName := strings.TrimSuffix(svc.Name, flatNetworkServiceNameSuffix)
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

	pods, err := h.podCache.List(svc.Namespace, labels.SelectorFromSet(svc.Spec.Selector))
	if err != nil {
		return false, fmt.Errorf("failed to list pod by selector [%v] on service [%v/%v]: %w",
			svc.Spec.Selector, svc.Namespace, svc.Name, err)
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

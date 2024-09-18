package service

import (
	"fmt"
	"strings"
	"time"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	// Requeue in a short intervals for flat-network services
	defaultFlatNetworkServiceEnqueue = time.Millisecond * 100
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
			// This flat-network service is created by user manually and does
			// not have original service, do-not-delete.
			return false, nil
		}
		return false, fmt.Errorf("failed to get service [%v/%v] from cache: %w",
			svc.Namespace, originalService.Name, err)
	}

	if len(pods) == 0 {
		logrus.WithFields(fieldsService(svc)).
			Debugf("no pods on flat-network service [%v/%v]",
				svc.Namespace, svc.Name)
		return false, nil
	}

	// Workload of this service disabled flat-network service by annotation.
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
	return false, nil
}

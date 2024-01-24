package controller

import (
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

func (h *Handler) handleServiceError(
	onChange func(string, *corev1.Service) (*corev1.Service, error),
) func(string, *corev1.Service) (*corev1.Service, error) {
	return func(key string, service *corev1.Service) (*corev1.Service, error) {
		var err error
		service, err = onChange(key, service)
		if service == nil {
			return service, err
		}

		if err != nil {
			// Avoid trigger the rate limit.
			logrus.Warnf("%v", err)
			time.Sleep(time.Second * 1)

			// TODO: handle error event here.
		}
		return service, err
	}
}

func (h *Handler) onServiceChanged(s string, service *corev1.Service) (*corev1.Service, error) {
	if service == nil || service.Name == "" || service.DeletionTimestamp != nil {
		return service, nil
	}

	return service, nil
}

func (h *Handler) onServiceRemoved(s string, service *corev1.Service) (*corev1.Service, error) {
	if service == nil || service.Name == "" {
		return service, nil
	}

	return service, nil
}

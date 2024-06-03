package service

import (
	"context"
	"fmt"
	"time"

	"github.com/cnrancher/flat-network-operator/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"

	corecontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/core/v1"
)

const (
	handlerName              = "flatnetwork-service"
	handlerRemoveName        = "flatnetwork-service-remove"
	macvlanServiceNameSuffix = "-macvlan"
)

type handler struct {
	serviceClient corecontroller.ServiceClient
	serviceCache  corecontroller.ServiceCache
	podCache      corecontroller.PodCache

	serviceEnqueueAfter func(string, string, time.Duration)
	serviceEnqueue      func(string, string)
}

func Register(
	ctx context.Context,
	services corecontroller.ServiceController,
	pods corecontroller.PodController,
) {
	h := &handler{
		serviceClient: services,
		serviceCache:  services.Cache(),
		podCache:      pods.Cache(),

		serviceEnqueueAfter: services.EnqueueAfter,
		serviceEnqueue:      services.Enqueue,
	}

	services.OnChange(ctx, handlerName, h.syncService)
}

func (h *handler) syncService(name string, svc *corev1.Service) (*corev1.Service, error) {
	if svc == nil || svc.Name == "" || svc.DeletionTimestamp != nil {
		return svc, nil
	}

	switch {
	case isIngressService(svc):
		// ignore rancher managed ingress service (manager UI only).
		return svc, nil
	case isFlatNetworkService(svc):
		// sync flat-network service created by this operator.
		return h.handleFlatNetworkService(svc)
	default:
		// sync other non-flat-network services.
		return h.handleDefaultService(svc)
	}
}

func (h *handler) handleDefaultService(svc *corev1.Service) (*corev1.Service, error) {
	flatNetworkServiceDisabled, err := h.isWorkloadDisabledFlatNetwork(svc)
	if err != nil {
		logrus.WithFields(fieldsService(svc)).
			Errorf("failed to check workload disabled flat-network service: %v", err)
		return svc, err
	}
	// The flat-network service creation was disabled, return directly.
	if flatNetworkServiceDisabled {
		return svc, nil
	}

	// Create if the flat-network service not exists.
	expectedService := newMacvlanService(svc)
	existService, err := h.serviceCache.Get(expectedService.Namespace, expectedService.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// if strings.HasSuffix(svc.Name, macvlanServiceNameSuffix) {
			// 	logrus.WithFields(fieldsService(svc)).
			// 		Infof("skip create [%v/%v] as the origional svc have %q suffix",
			// 			svc.Namespace, expectedService.Name, macvlanServiceNameSuffix)
			// 	return svc, nil
			// }
			logrus.WithFields(fieldsService(svc)).
				Infof("request to create flat-network service [%v/%v]",
					expectedService.Namespace, expectedService.Name)
			_, err := h.serviceClient.Create(expectedService)
			if err != nil {
				logrus.WithFields(fieldsService(svc)).
					Errorf("failed to create flat-network service [%v/%v]: %v",
						expectedService.Namespace, expectedService.Name, err)
				return svc, err
			}
			return svc, nil
		}

		logrus.WithFields(fieldsService(svc)).
			Errorf("failed to get [%v/%v] from cache: %v",
				expectedService.Namespace, expectedService.Name, err)
		return svc, err
	}

	// Skip if the macvlan service is already updated.
	if flatNetworkServiceUpdated(existService, expectedService) {
		logrus.WithFields(fieldsService(svc)).
			Debugf("flat-network service [%v/%v] already updated, skip",
				expectedService.Namespace, expectedService.Name)
		return svc, nil
	}

	// Update the flat-network service with retry to avoid conflict.
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err := h.serviceCache.Get(expectedService.Namespace, expectedService.Name)
		if err != nil {
			logrus.WithFields(fieldsService(svc)).
				Warnf("failed to get svc [%v/%v] from cache: %v",
					expectedService.Namespace, expectedService.Name, err)
			return err
		}

		result = result.DeepCopy()
		result.Spec = expectedService.Spec
		result.Annotations = expectedService.Annotations
		result.OwnerReferences = expectedService.OwnerReferences
		result, err = h.serviceClient.Update(result)
		if err != nil {
			return err
		}
		svc = result
		return nil
	}); err != nil {
		logrus.WithFields(fieldsService(svc)).
			Errorf("failed to update flat-network service [%v/%v]: %v",
				expectedService.Namespace, expectedService.Name, err)
		return svc, err
	}
	logrus.WithFields(fieldsService(svc)).
		Infof("updated flat-network service [%v/%v]",
			expectedService.Namespace, expectedService.Name)

	return nil, nil
}

func fieldsService(svc *corev1.Service) logrus.Fields {
	if svc == nil {
		return logrus.Fields{}
	}
	return logrus.Fields{
		"GID": utils.GetGID(),
		"SVC": fmt.Sprintf("%v/%v", svc.Namespace, svc.Name),
	}
}

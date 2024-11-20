package service

import (
	"context"
	"fmt"
	"time"

	"github.com/cnrancher/rancher-flat-network/pkg/controller/wrangler"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"

	corecontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/core/v1"
	discoverycontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/discovery.k8s.io/v1"
)

const (
	handlerName        = "rancher-flat-network-service"
	defaultRequeueTime = time.Minute * 5
)

type handler struct {
	serviceClient       corecontroller.ServiceClient
	serviceCache        corecontroller.ServiceCache
	podCache            corecontroller.PodCache
	endpointsCache      corecontroller.EndpointsCache
	endpointsClient     corecontroller.EndpointsClient
	endpointSliceCache  discoverycontroller.EndpointSliceCache
	endpointSliceClient discoverycontroller.EndpointSliceClient

	supportDiscoveryV1 bool

	serviceEnqueueAfter func(string, string, time.Duration)
	serviceEnqueue      func(string, string)
}

func Register(
	ctx context.Context,
	wctx *wrangler.Context,
) {
	h := &handler{
		serviceClient:       wctx.Core.Service(),
		serviceCache:        wctx.Core.Service().Cache(),
		podCache:            wctx.Core.Pod().Cache(),
		endpointsCache:      wctx.Core.Endpoints().Cache(),
		endpointsClient:     wctx.Core.Endpoints(),
		endpointSliceCache:  wctx.Discovery.EndpointSlice().Cache(),
		endpointSliceClient: wctx.Discovery.EndpointSlice(),
		supportDiscoveryV1:  wctx.SupportDiscoveryV1(),

		serviceEnqueueAfter: wctx.Core.Service().EnqueueAfter,
		serviceEnqueue:      wctx.Core.Service().Enqueue,
	}

	wctx.Core.Service().OnChange(ctx, handlerName, h.syncService)
}

func (h *handler) syncService(_ string, svc *corev1.Service) (*corev1.Service, error) {
	if svc == nil || svc.Name == "" || svc.DeletionTimestamp != nil {
		return svc, nil
	}

	switch {
	case isIngressService(svc):
		// ignore rancher managed ingress service (manager UI only).
		return svc, nil
	case utils.IsFlatNetworkService(svc):
		// sync flat-network service created by operator or user manually.
		return h.handleFlatNetworkService(svc)
	case utils.IsMacvlanV1Service(svc):
		// Skip Macvlan V1 services
		return svc, nil
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
	expectedService := newFlatNetworkService(svc)
	existService, err := h.serviceCache.Get(expectedService.Namespace, expectedService.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.WithFields(fieldsService(svc)).
				Infof("request to create flat-network service [%v/%v]",
					expectedService.Namespace, expectedService.Name)
			_, err := h.serviceClient.Create(expectedService)
			if err != nil {
				if apierrors.IsAlreadyExists(err) {
					return svc, nil
				}
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

	// Skip if the flat-network service is already updated.
	if flatNetworkServiceUpdated(existService, expectedService) {
		logrus.WithFields(fieldsService(svc)).
			Debugf("flat-network service [%v/%v] already updated, skip",
				expectedService.Namespace, expectedService.Name)
		h.serviceEnqueueAfter(svc.Namespace, svc.Name, defaultRequeueTime)
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
	h.serviceEnqueueAfter(svc.Namespace, svc.Name, defaultRequeueTime)

	return nil, nil
}

func fieldsService(svc *corev1.Service) logrus.Fields {
	if svc == nil {
		return logrus.Fields{}
	}
	return logrus.Fields{
		"GID": utils.GID(),
		"SVC": fmt.Sprintf("%v/%v", svc.Namespace, svc.Name),
	}
}

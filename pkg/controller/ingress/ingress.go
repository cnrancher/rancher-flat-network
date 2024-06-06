package ingress

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	flv1 "github.com/cnrancher/flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
	corecontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/core/v1"
	networkingcontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/networking.k8s.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/utils"
)

const (
	handlerName = "flatnetwork-ingress"

	k8sCNINetworksKey = "k8s.v1.cni.cncf.io/networks"
	netAttatchDefName = "static-flat-network-cni-attach"
)

type handler struct {
	serviceClient corecontroller.ServiceClient
	serviceCache  corecontroller.ServiceCache

	ingressEnqueueAfter func(string, string, time.Duration)
	ingressEnqueue      func(string, string)
}

func Register(
	ctx context.Context,
	ingress networkingcontroller.IngressController,
	service corecontroller.ServiceController,
) {
	h := &handler{
		serviceClient: service,
		serviceCache:  service.Cache(),

		ingressEnqueueAfter: ingress.EnqueueAfter,
		ingressEnqueue:      ingress.Enqueue,
	}
	ingress.OnChange(ctx, handlerName, h.syncIngress)
}

func (h *handler) syncIngress(
	_ string, ingress *networkingv1.Ingress,
) (*networkingv1.Ingress, error) {
	if ingress == nil || ingress.Name == "" || ingress.DeletionTimestamp != nil {
		return ingress, nil
	}
	if len(ingress.Spec.Rules) == 0 || ingress.Annotations == nil {
		return ingress, nil
	}
	// TODO: Only need to sync manager-UI created ingress service.

	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil || rule.HTTP.Paths == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service == nil || path.Backend.Service.Name == "" {
				continue
			}
			svcName := path.Backend.Service.Name
			// Only sync service with name has prefix 'ingress-'.
			flatNetworkEnabled := false
			if ingress.Annotations[flv1.AnnotationIngress] == "true" {
				flatNetworkEnabled = true
			}
			// Get service from the informer cache.
			// The service may not found if the ingress is just created.
			svc, err := h.serviceCache.Get(ingress.Namespace, svcName)
			if err != nil {
				if errors.IsNotFound(err) {
					logrus.WithFields(fieldsIngress(ingress)).
						Infof("service [%s/%s] not found from cache, will try later",
							ingress.Namespace, svcName)
					h.ingressEnqueueAfter(ingress.Namespace, ingress.Name, time.Second*5)
					return ingress, nil
				}

				return ingress, fmt.Errorf("failed to get service [%s/%s] of ingress [%v]: %w",
					ingress.Namespace, svcName, ingress.Name, err)
			}

			if err = h.handleIngressService(ingress, svc, flatNetworkEnabled); err != nil {
				logrus.WithFields(fieldsIngress(ingress)).
					Errorf("failed to update ingress service: %v", err)
				return ingress, nil
			}
		}
	}

	return ingress, nil
}

// handleIngressService add/remove flat-network multus annotation for ingress service.
func (h *handler) handleIngressService(
	ingress *networkingv1.Ingress, service *corev1.Service, flatNetworkEnabled bool,
) error {
	if service.Annotations == nil {
		service.Annotations = map[string]string{}
	}
	// Skip if the service annotation already updated.
	if flatNetworkEnabled {
		if service.Annotations[k8sCNINetworksKey] == netAttatchDefName {
			return nil
		}
	} else if service.Annotations[k8sCNINetworksKey] == "" {
		return nil
	}

	service = service.DeepCopy()
	if flatNetworkEnabled {
		service.Annotations[k8sCNINetworksKey] = netAttatchDefName
	} else {
		delete(service.Annotations, k8sCNINetworksKey)
	}
	_, err := h.serviceClient.Update(service)
	if err != nil {
		return fmt.Errorf("failed to update service [%v/%v]: %w",
			service.Namespace, service.Name, err)
	}

	logrus.WithFields(fieldsIngress(ingress)).
		Infof("ingress[%s] service[%s] update flat-network enabled [%v]",
			ingress.Name, service.Name, flatNetworkEnabled)
	return nil
}

func fieldsIngress(ingress *networkingv1.Ingress) logrus.Fields {
	if ingress == nil {
		return logrus.Fields{}
	}
	return logrus.Fields{
		"GID": utils.GetGID(),
		"IP":  fmt.Sprintf("%v/%v", ingress.Namespace, ingress.Name),
	}
}

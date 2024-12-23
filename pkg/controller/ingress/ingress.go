package ingress

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/controller/wrangler"
	corecontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/core/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	nettypes "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
)

const (
	handlerName = "rancher-flat-network-ingress"

	rancherFlatNetworkCNI = "rancher-flat-network-cni"
)

type handler struct {
	serviceClient corecontroller.ServiceClient
	serviceCache  corecontroller.ServiceCache

	ingressEnqueueAfter func(string, string, time.Duration)
	ingressEnqueue      func(string, string)
}

func Register(
	ctx context.Context,
	wctx *wrangler.Context,
) {
	h := &handler{
		serviceClient: wctx.Core.Service(),
		serviceCache:  wctx.Core.Service().Cache(),

		ingressEnqueueAfter: wctx.Networking.Ingress().EnqueueAfter,
		ingressEnqueue:      wctx.Networking.Ingress().Enqueue,
	}
	wctx.Networking.Ingress().OnChange(ctx, handlerName, h.syncIngress)
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

				return ingress, fmt.Errorf("failed to get service [%s/%s] of ingress [%v] from cache: %w",
					ingress.Namespace, svcName, ingress.Name, err)
			}
			if utils.IsFlatNetworkService(svc) || utils.IsMacvlanV1Service(svc) {
				// IMPORTANT: Skip sync operator created flat-network service,
				// sync rancher-created/user created service only.
				continue
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
		if service.Annotations[nettypes.NetworkAttachmentAnnot] == rancherFlatNetworkCNI {
			return nil
		}
	} else if service.Annotations[nettypes.NetworkAttachmentAnnot] == "" {
		return nil
	}

	service = service.DeepCopy()
	if flatNetworkEnabled {
		service.Annotations[nettypes.NetworkAttachmentAnnot] = rancherFlatNetworkCNI
	} else {
		delete(service.Annotations, nettypes.NetworkAttachmentAnnot)
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
		"GID":     utils.GID(),
		"Ingress": fmt.Sprintf("%v/%v", ingress.Namespace, ingress.Name),
	}
}

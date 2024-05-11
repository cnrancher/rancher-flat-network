package ingress

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	corecontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/core/v1"
	networkingcontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/networking.k8s.io/v1"
)

const (
	controllerName       = "ingress"
	controllerRemoveName = "ingress-remove"
)

const (
	k8sCNINetworksKey = "k8s.v1.cni.cncf.io/networks"
	netAttatchDefName = "static-macvlan-cni-attach"
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
	ingress.OnChange(ctx, controllerName, h.handleIngressError(h.syncIngress))
}

func (h *handler) handleIngressError(
	onChange func(string, *networkingv1.Ingress) (*networkingv1.Ingress, error),
) func(string, *networkingv1.Ingress) (*networkingv1.Ingress, error) {
	// TODO: handle service retry
	return onChange
}

func (h *handler) syncIngress(
	name string, ingress *networkingv1.Ingress,
) (*networkingv1.Ingress, error) {
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
			macvlanEnabled := false
			if ingress.Annotations[macvlanv1.AnnotationIngress] == "true" {
				macvlanEnabled = true
			}
			// Get service from the informer cache.
			// The service may not found if the ingress is just created.
			svc, err := h.serviceCache.Get(ingress.Namespace, svcName)
			if err != nil {
				if errors.IsNotFound(err) {
					logrus.Debugf("onIngressAdd: Service [%s/%s] not found yet: [%v], will try later.",
						ingress.Namespace, svcName, err)
				} else {
					logrus.Errorf("onIngressAdd: Failed to get service [%s/%s] of ingress [%v]: %v",
						ingress.Namespace, svcName, ingress.Name, err)
				}
				continue
			}
			// Skip update if the service annotation already set.
			if macvlanEnabled {
				if svc.Annotations != nil && svc.Annotations[k8sCNINetworksKey] == netAttatchDefName {
					continue
				}
			} else {
				if svc.Annotations == nil || svc.Annotations[k8sCNINetworksKey] == "" {
					continue
				}
			}
			err = h.handleIngressService(ingress.Namespace, svcName, ingress.Name, macvlanEnabled)
			if err != nil {
				logrus.Errorf("onIngressAdd: failed to update ingress service: %v", err)
			}
		}
	}

	return ingress, nil
}

// handleIngressService add/remove macvlan annotation for ingress service.
func (h *handler) handleIngressService(
	namespace, svcName, ingressName string, macvlanEnabled bool,
) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Retrieve the latest version of Service before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		result, err := h.serviceClient.Get(namespace, svcName, metav1.GetOptions{})
		if err != nil {
			logrus.Errorf("Failed to get latest version of Service: %v", err)
			return err
		}

		svc := result.DeepCopy()
		if svc.Annotations == nil {
			svc.Annotations = make(map[string]string)
		}
		if macvlanEnabled {
			svc.Annotations[k8sCNINetworksKey] = netAttatchDefName
		} else {
			delete(svc.Annotations, k8sCNINetworksKey)
		}
		if equality.Semantic.DeepEqual(result.Annotations, svc.Annotations) {
			// Skip if the service annotation already updated.
			logrus.Debugf("Skip update ingress svc [%v] annotation as already updated", svc.Name)
			return nil
		}
		logrus.Debugf("Kube apiserver update service [%v/%v] request", svc.Namespace, svc.Name)
		_, err = h.serviceClient.Update(svc)
		if err != nil {
			return err
		}
		logrus.Infof("handleIngressService: namespace[%s] ingress[%s] service[%s] update macvlan enabled [%v]",
			namespace, ingressName, svcName, macvlanEnabled)
		return nil
	})
	if err != nil {
		return fmt.Errorf("ingress service update annotation error: %w", err)
	}
	return nil
}

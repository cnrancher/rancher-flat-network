package service

import (
	"fmt"

	"github.com/cnrancher/rancher-flat-network-operator/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
)

// syncCoreV1Endpoints updates service corev1.Endpoints & discoveryv1.Endpoint
// IP to pod flat-network IP.
func (h *handler) syncCoreV1Endpoints(
	svc *corev1.Service, resource *endpointReource,
) error {
	if resource == nil {
		logrus.WithFields(fieldsService(svc)).
			Debugf("skip update endpoint [%v]: no subset resource found", svc.Name)
		return nil
	}

	// Update corev1.Endpoints
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err := h.endpointsCache.Get(svc.Namespace, svc.Name)
		if err != nil {
			return fmt.Errorf("failed to get corev1.Endpoints from cache: %w", err)
		}
		if apiequality.Semantic.DeepDerivative(resource.subsets, result.Subsets) {
			logrus.WithFields(fieldsService(svc)).
				Debugf("corev1.Endpoints [%v] already updated, skip", result.Name)
			return nil
		}
		result = result.DeepCopy()
		result.SetOwnerReferences(
			[]metav1.OwnerReference{
				*metav1.NewControllerRef(svc, schema.GroupVersionKind{
					Group:   corev1.SchemeGroupVersion.Group,
					Version: corev1.SchemeGroupVersion.Version,
					Kind:    "Service",
				}),
			},
		)
		if result.Labels == nil {
			result.Labels = map[string]string{}
		}
		result.Labels[discoveryv1.LabelSkipMirror] = "true"
		result.Subsets = resource.subsets
		endpoints, err := h.endpointsClient.Update(result)
		if err != nil {
			return err
		}

		addrs := []string{}
		for _, s := range endpoints.Subsets {
			for _, a := range s.Addresses {
				addrs = append(addrs, a.IP)
			}
		}
		logrus.WithFields(fieldsService(svc)).
			Infof("update corev1.Endpoints [%v] address: %v",
				endpoints.Name, utils.Print(addrs))
		return nil
	}); err != nil {
		return fmt.Errorf("failed to update corev1.Endpoints: %w", err)
	}

	return nil
}

package endpoints

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cnrancher/rancher-flat-network/pkg/common"
	"github.com/cnrancher/rancher-flat-network/pkg/controller/wrangler"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"

	corecontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/core/v1"
	discoverycontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/discovery.k8s.io/v1"
)

const (
	handlerName = "rancher-flat-network-endpoints"

	labelServiceName = "kubernetes.io/service-name"
)

type handler struct {
	endpointsCache     corecontroller.EndpointsCache
	endpointsClient    corecontroller.EndpointsClient
	endpointSliceCache discoverycontroller.EndpointSliceCache
	serviceCache       corecontroller.ServiceCache
	podCache           corecontroller.PodCache
	supportDiscoveryV1 bool

	endpointsEnqueueAfter func(string, string, time.Duration)
	endpointsEnqueue      func(string, string)
	endpointSliceEnqueue  func(string, string)
}

func Register(
	ctx context.Context,
	wctx *wrangler.Context,
) {
	h := &handler{
		endpointsCache:     wctx.Core.Endpoints().Cache(),
		endpointsClient:    wctx.Core.Endpoints(),
		endpointSliceCache: wctx.Discovery.EndpointSlice().Cache(),
		serviceCache:       wctx.Core.Service().Cache(),
		podCache:           wctx.Core.Pod().Cache(),
		supportDiscoveryV1: wctx.SupportDiscoveryV1(),

		endpointsEnqueueAfter: wctx.Core.Endpoints().EnqueueAfter,
		endpointsEnqueue:      wctx.Core.Endpoints().Enqueue,
		endpointSliceEnqueue:  wctx.Discovery.EndpointSlice().Enqueue,
	}

	wctx.Core.Endpoints().OnChange(ctx, handlerName, h.sync)
}

func (h *handler) sync(
	_ string, endpoints *corev1.Endpoints,
) (*corev1.Endpoints, error) {
	if endpoints == nil || endpoints.Name == "" || endpoints.DeletionTimestamp != nil {
		return endpoints, nil
	}

	svc, err := h.serviceCache.Get(endpoints.Namespace, endpoints.Name)
	if err != nil {
		return endpoints, fmt.Errorf("failed to get svc of corev1.Endpoints %q: %w",
			endpoints.Name, err)
	}
	pods, err := h.podCache.List(svc.Namespace, labels.SelectorFromSet(svc.Spec.Selector))
	if err != nil {
		return endpoints, fmt.Errorf("failed to list pod by selector [%v] on endpointSlice [%v/%v]: %w",
			svc.Spec.Selector, endpoints.Namespace, endpoints.Name, err)
	}
	for _, pod := range pods {
		if !utils.IsPodEnabledFlatNetwork(pod) {
			return endpoints, nil
		}
	}
	resource, err := common.GetEndpointResources(svc, pods)
	if err != nil {
		if errors.Is(err, common.ErrPodNetworkStatusNotUpdated) {
			logrus.WithFields(fieldsEPS(endpoints)).
				Debugf("wait for pod network status updated by multus CNI")
			h.endpointsEnqueueAfter(endpoints.Namespace, endpoints.Name, time.Millisecond*100)
			return endpoints, nil
		}
		logrus.WithFields(fieldsEPS(endpoints)).
			Errorf("failed to get endpoint resources of endpointSlice: %v", err)
		return endpoints, fmt.Errorf("failed to get endpoint resources: %w", err)
	}

	if err := h.syncCoreV1Endpoints(endpoints, svc, resource); err != nil {
		return endpoints, err
	}

	return endpoints, nil
}

// syncCoreV1Endpoints updates service corev1.Endpoints to flat-network IP
func (h *handler) syncCoreV1Endpoints(
	endpoints *corev1.Endpoints,
	svc *corev1.Service,
	resource *common.EndpointReource,
) error {
	if resource == nil {
		logrus.WithFields(fieldsEPS(endpoints)).
			Debugf("skip update endpoint [%v]: no subset resource found", svc.Name)
		return nil
	}

	// Update corev1.Endpoints
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err := h.endpointsCache.Get(svc.Namespace, svc.Name)
		if err != nil {
			return fmt.Errorf("failed to get corev1.Endpoints from cache: %w", err)
		}
		if apiequality.Semantic.DeepDerivative(resource.Subsets, result.Subsets) {
			logrus.WithFields(fieldsEPS(endpoints)).
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
		if h.supportDiscoveryV1 {
			result.Labels[discoveryv1.LabelSkipMirror] = "true"
		}
		result.Subsets = resource.Subsets
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
		logrus.WithFields(fieldsEPS(endpoints)).
			Infof("update corev1.Endpoints [%v] address: %v",
				endpoints.Name, utils.Print(addrs))
		return nil
	}); err != nil {
		return fmt.Errorf("failed to update corev1.Endpoints: %w", err)
	}

	set := map[string]string{
		labelServiceName: svc.Name,
	}
	epSlices, err := h.endpointSliceCache.List(
		endpoints.Namespace, labels.SelectorFromSet(set))
	if err != nil {
		logrus.WithFields(fieldsEPS(endpoints)).
			Errorf("failed to list discoveryv1.EndpointSlice by selector %v: %v",
				set, err)
		return fmt.Errorf("failed to list EndpointSlice: %w", err)
	}
	if len(epSlices) == 0 {
		return nil
	}
	for _, epslice := range epSlices {
		// EndpointSlice will not updated automatically when Endpoints changed
		// Need to requeue EndpointSlice manually to update endpoints.
		h.endpointSliceEnqueue(epslice.Namespace, epslice.Name)
	}

	return nil
}

func fieldsEPS(eps *corev1.Endpoints) logrus.Fields {
	if eps == nil {
		return logrus.Fields{}
	}
	return logrus.Fields{
		"GID":       utils.GID(),
		"Endpoints": fmt.Sprintf("%v/%v", eps.Namespace, eps.Name),
	}
}

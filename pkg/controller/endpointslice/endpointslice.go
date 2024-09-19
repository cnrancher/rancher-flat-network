package endpointslice

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cnrancher/rancher-flat-network/pkg/common"
	"github.com/cnrancher/rancher-flat-network/pkg/controller/wrangler"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/util/retry"

	corecontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/core/v1"
	discoverycontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/discovery.k8s.io/v1"
)

const (
	handlerName = "rancher-flat-network-endpointslice"

	labelServiceName = "kubernetes.io/service-name"
)

type handler struct {
	endpointSliceCache  discoverycontroller.EndpointSliceCache
	endpointSliceClient discoverycontroller.EndpointSliceClient
	serviceCache        corecontroller.ServiceCache
	podCache            corecontroller.PodCache
	supportDiscoveryV1  bool

	endpointSliceEnqueueAfter func(string, string, time.Duration)
	endpointSliceEnqueue      func(string, string)
}

func Register(
	ctx context.Context,
	wctx *wrangler.Context,
) {
	h := &handler{
		endpointSliceCache:  wctx.Discovery.EndpointSlice().Cache(),
		endpointSliceClient: wctx.Discovery.EndpointSlice(),
		serviceCache:        wctx.Core.Service().Cache(),
		podCache:            wctx.Core.Pod().Cache(),
		supportDiscoveryV1:  wctx.SupportDiscoveryV1(),

		endpointSliceEnqueueAfter: wctx.Discovery.EndpointSlice().EnqueueAfter,
		endpointSliceEnqueue:      wctx.Discovery.EndpointSlice().Enqueue,
	}
	if !h.supportDiscoveryV1 {
		logrus.Infof("skip register EndpointSlice handler as discovery.k8s.io/v1 is not supported")
		return
	}

	wctx.Discovery.EndpointSlice().OnChange(ctx, handlerName, h.sync)
}

func (h *handler) sync(
	_ string, epslice *discoveryv1.EndpointSlice,
) (*discoveryv1.EndpointSlice, error) {
	if !h.supportDiscoveryV1 {
		return epslice, nil
	}
	if epslice == nil || epslice.Name == "" || epslice.DeletionTimestamp != nil {
		return epslice, nil
	}
	if len(epslice.Labels) == 0 {
		return epslice, nil
	}

	serviceName := epslice.Labels[labelServiceName]
	if serviceName == "" {
		return epslice, nil
	}

	svc, err := h.serviceCache.Get(epslice.Namespace, serviceName)
	if err != nil {
		return epslice, fmt.Errorf("failed to get service of discoveryv1.EndpointSlice %q: %w",
			epslice.Name, err)
	}
	pods, err := h.podCache.List(svc.Namespace, labels.SelectorFromSet(svc.Spec.Selector))
	if err != nil {
		return epslice, fmt.Errorf("failed to list pod by selector [%v] on endpointSlice [%v/%v]: %w",
			svc.Spec.Selector, epslice.Namespace, epslice.Name, err)
	}
	resource, err := common.GetEndpointResources(svc, pods)
	if err != nil {
		if errors.Is(err, common.ErrPodNetworkStatusNotUpdated) {
			logrus.WithFields(fieldsEPS(epslice)).
				Debugf("wait for pod network status updated by multus CNI")
			h.endpointSliceEnqueueAfter(epslice.Namespace, epslice.Name, time.Millisecond*100)
			return epslice, nil
		}
		logrus.WithFields(fieldsEPS(epslice)).
			Errorf("failed to get endpoint resources of endpointSlice: %v", err)
		return epslice, fmt.Errorf("failed to get endpoint resources: %w", err)
	}

	if err := h.syncDiscoveryV1EndpointSlice(epslice, svc, resource); err != nil {
		return epslice, err
	}
	return epslice, nil
}

func (h *handler) syncDiscoveryV1EndpointSlice(
	epSlice *discoveryv1.EndpointSlice,
	svc *corev1.Service,
	resource *common.EndpointReource,
) error {
	if resource == nil {
		return nil
	}

	// Update discoveryv1.EndpointSlice
	endpointSlices, err := h.endpointSliceCache.List(svc.Namespace, labels.SelectorFromSet(map[string]string{
		discoveryv1.LabelServiceName: svc.Name}))
	if err != nil {
		return fmt.Errorf("failed to list discoveryv1.EndpointSlices from cache: %w", err)
	}
	endpointSlices = getAvailableEndpointSlices(endpointSlices)
	if len(endpointSlices) == 0 {
		logrus.WithFields(fieldsEPS(epSlice)).
			Warnf("no matching EndpointSlice found of service")
		return nil
	}

	for _, s := range endpointSlices {
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			endpointSlice, err := h.endpointSliceCache.Get(
				s.Namespace, s.Name)
			if err != nil {
				return err
			}

			endpointSlice = endpointSlice.DeepCopy()
			endpoints := endpointSlice.Endpoints
			ports := endpointSlice.Ports
			sort.Slice(endpoints, func(i, j int) bool {
				return strings.Compare(endpoints[i].TargetRef.Name, endpoints[j].TargetRef.Name) > 0
			})
			sort.Slice(ports, func(i, j int) bool {
				return strings.Compare(*ports[i].Name, *ports[j].Name) > 0
			})
			sort.Slice(resource.Endpoints, func(i, j int) bool {
				return strings.Compare(
					resource.Endpoints[i].TargetRef.Name, resource.Endpoints[j].TargetRef.Name) > 0
			})
			sort.Slice(resource.EndpointPorts, func(i, j int) bool {
				return strings.Compare(
					*resource.EndpointPorts[i].Name, *resource.EndpointPorts[j].Name) > 0
			})
			if len(endpoints) == len(resource.Endpoints) &&
				apiequality.Semantic.DeepDerivative(resource.Endpoints, endpoints) &&
				apiequality.Semantic.DeepDerivative(resource.EndpointPorts, ports) {
				logrus.WithFields(fieldsEPS(epSlice)).
					Debugf("discoveryv1.EndpointSlice [%v] already updated, skip",
						endpointSlice.Name)
				return nil
			}
			endpointSlice.Labels[discoveryv1.LabelManagedBy] = "rancher-flat-network-controller"
			endpointSlice.Endpoints = resource.Endpoints
			endpointSlice.Ports = resource.EndpointPorts
			addressType, err := resource.GetEndpointSliceAddressType()
			if err != nil {
				logrus.WithFields(fieldsEPS(epSlice)).
					Errorf("failed to update endpointSlice: %v", err)
				return nil
			}
			if addressType != endpointSlice.AddressType {
				logrus.WithFields(fieldsEPS(epSlice)).
					Infof("skip to update endpointSlice [%v]: address type changed to IPv6, skip",
						endpointSlice.Name)
				return nil
			}
			endpointSlice, err = h.endpointSliceClient.Update(endpointSlice)
			if err != nil {
				return fmt.Errorf("failed to update endpointSlice: %w", err)
			}

			epSliceIPs := []string{}
			for _, e := range endpointSlice.Endpoints {
				epSliceIPs = append(epSliceIPs, e.Addresses...)
			}
			logrus.WithFields(fieldsEPS(epSlice)).
				Infof("update EndpointSlice [%v] addresses: %v",
					endpointSlice.Name, utils.Print(epSliceIPs))
			return nil
		}); err != nil {
			return fmt.Errorf("failed to update discoveryv1.EndpointSlice: %w", err)
		}
	}
	return nil
}

// Get IPv4 & IPv6 endpoint slices
func getAvailableEndpointSlices(
	s []*discoveryv1.EndpointSlice,
) []*discoveryv1.EndpointSlice {
	if len(s) == 0 {
		return s
	}
	result := []*discoveryv1.EndpointSlice{}
	for _, e := range s {
		switch {
		case e.DeletionTimestamp != nil,
			e.AddressType != discoveryv1.AddressTypeIPv4 &&
				e.AddressType != discoveryv1.AddressTypeIPv6:
			continue
		}
		result = append(result, e)
	}
	return result
}

func fieldsEPS(eps *discoveryv1.EndpointSlice) logrus.Fields {
	if eps == nil {
		return logrus.Fields{}
	}
	return logrus.Fields{
		"GID":           utils.GID(),
		"EndpointSlice": fmt.Sprintf("%v/%v", eps.Namespace, eps.Name),
	}
}

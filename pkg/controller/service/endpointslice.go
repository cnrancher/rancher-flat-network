package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cnrancher/rancher-flat-network-operator/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/util/retry"
)

func (h *handler) syncDiscoveryV1EndpointSlice(
	svc *corev1.Service, resource *endpointReource,
) error {
	if resource == nil {
		return nil
	}
	if !h.supportDiscoveryV1 {
		logrus.WithFields(fieldsService(svc)).
			Debugf("skip update EndpointSlice as discovery.k8s.io/v1 not supported by this cluster")
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
		logrus.WithFields(fieldsService(svc)).
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
			sort.Slice(resource.endpoints, func(i, j int) bool {
				return strings.Compare(
					resource.endpoints[i].TargetRef.Name, resource.endpoints[j].TargetRef.Name) > 0
			})
			sort.Slice(resource.endpointPorts, func(i, j int) bool {
				return strings.Compare(
					*resource.endpointPorts[i].Name, *resource.endpointPorts[j].Name) > 0
			})
			if len(endpoints) == len(resource.endpoints) &&
				apiequality.Semantic.DeepDerivative(resource.endpoints, endpoints) &&
				apiequality.Semantic.DeepDerivative(resource.endpointPorts, ports) {
				logrus.WithFields(fieldsService(svc)).
					Debugf("discoveryv1.EndpointSlice [%v] already updated, skip",
						endpointSlice.Name)
				return nil
			}
			endpointSlice.Labels[discoveryv1.LabelManagedBy] = "rancher-flat-network-controller"
			endpointSlice.Endpoints = resource.endpoints
			endpointSlice.Ports = resource.endpointPorts
			addressType, err := resource.getEndpointSliceAddressType()
			if err != nil {
				logrus.WithFields(fieldsService(svc)).
					Errorf("failed to update endpointSlice: %v", err)
				return nil
			}
			if addressType != endpointSlice.AddressType {
				logrus.WithFields(fieldsService(svc)).
					Warnf("skip to update endpointSlice [%v]: address type changed to IPv6, skip",
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
			logrus.WithFields(fieldsService(svc)).
				Infof("update EndpointSlice [%v] addresses: %v",
					endpointSlice.Name, utils.Print(epSliceIPs))
			return nil
		}); err != nil {
			return fmt.Errorf("failed to update discoveryv1.EndpointSlice: %w", err)
		}
	}
	return nil
}

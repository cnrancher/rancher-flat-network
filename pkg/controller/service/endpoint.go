package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cnrancher/flat-network-operator/pkg/utils"
	"github.com/sirupsen/logrus"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	"k8s.io/kubernetes/pkg/api/v1/endpoints"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"

	nettypes "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
)

type endpointReource struct {
	subsets       []corev1.EndpointSubset
	endpoints     []discoveryv1.Endpoint
	endpointPorts []discoveryv1.EndpointPort
}

// syncServiceEndpoints updates service corev1.Endpoints & discoveryv1.Endpoint
// IP to pod flat-network IP.
func (h *handler) syncServiceEndpoints(
	svc *corev1.Service, pods []*corev1.Pod,
) error {
	resource, err := h.getEndpointResources(svc, pods)
	if err != nil {
		return fmt.Errorf("failed to get endpoint resources: %w", err)
	}
	if resource == nil {
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
		logrus.WithFields(fieldsService(svc)).
			Infof("Updated corev1.Endpoints [%v/%v] Subsets: %v",
				endpoints.Namespace, endpoints.Name, endpoints.Subsets)
		return nil
	}); err != nil {
		return fmt.Errorf("failed to update corev1.Endpoints: %w", err)
	}
	if !h.supportDiscoveryV1 {
		logrus.WithFields(fieldsService(svc)).
			Debugf("skip to update service EndpointSlice as discovery.k8s.io/v1 not supported")
		return nil
	}

	// Update discoveryv1.EndpointSlice
	logrus.WithFields(fieldsService(svc)).
		Debugf("updating service discoveryv1.EndpointSlice")
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

	if len(endpointSlices) > 1 {
		for _, s := range endpointSlices[1:] {
			err = h.endpointSliceClient.Delete(s.Namespace, s.Name, &metav1.DeleteOptions{})
			if err != nil {
				return fmt.Errorf("failed to delete endpointSlice [%v/%v]: %w",
					s.Namespace, s.Name, err)
			}
			logrus.WithFields(fieldsService(svc)).
				Infof("request to delete unused EndpointSlice [%v/%v]",
					s.Namespace, s.Name)
		}
	}
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		endpointSlice, err := h.endpointSliceCache.Get(
			endpointSlices[0].Namespace, endpointSlices[0].Name)
		if err != nil {
			return fmt.Errorf("failed to get EndpointSlice from cache: %w", err)
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
		endpointSlice, err = h.endpointSliceClient.Update(endpointSlice)
		if err != nil {
			return fmt.Errorf("failed to update endpointSlice: %w", err)
		}
		logrus.WithFields(fieldsService(svc)).
			Infof("update EndpointSlice [%v/%v] endpoints: %v, ports %v",
				endpointSlice.Namespace, endpointSlice.Name,
				utils.Print(resource.endpoints),
				utils.Print(resource.endpointPorts))
		return nil
	}); err != nil {
		return fmt.Errorf("failed to update discoveryv1.EndpointSlice: %w", err)
	}
	return nil
}

func (h *handler) getEndpointResources(
	svc *corev1.Service, pods []*corev1.Pod,
) (*endpointReource, error) {
	svcNetworkValue := svc.Annotations[k8sCNINetworksKey]
	if svcNetworkValue == "" {
		return nil, nil
	}
	svcNetworkSelections, err := parseServiceNetworkSelections(svcNetworkValue, svc.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to parse service network annotation [%v: %v]: %w",
			k8sCNINetworksKey, svcNetworkValue, err)
	}
	resource := &endpointReource{
		subsets:       make([]corev1.EndpointSubset, 0),
		endpoints:     make([]discoveryv1.Endpoint, 0),
		endpointPorts: make([]discoveryv1.EndpointPort, 0),
	}
	for _, pod := range pods {
		// Skip deleted pods
		if pod.DeletionTimestamp != nil {
			continue
		}
		subset := getPodEndpointSubset(svc, svcNetworkSelections, pod)
		if subset == nil || subset.Addresses == nil {
			continue
		}
		resource.subsets = append(resource.subsets, *subset)
	}
	resource.subsets = endpoints.RepackSubsets(resource.subsets)
	for _, subset := range resource.subsets {
		for _, address := range subset.Addresses {
			endpoint := addressToEndpoint(address)
			resource.endpoints = append(resource.endpoints, endpoint)
		}
		ports := epPortsToEpsPorts(subset.Ports)
		resource.endpointPorts = append(resource.endpointPorts, ports...)
	}
	return resource, nil
}

func getPodEndpointSubset(
	svc *corev1.Service, svcNetworks []*types.NetworkSelectionElement, pod *corev1.Pod,
) *corev1.EndpointSubset {
	addresses := make([]corev1.EndpointAddress, 0)
	ports := make([]corev1.EndpointPort, 0)
	networksStatus := make([]nettypes.NetworkStatus, 0)
	status := pod.Annotations[k8sCNINetworksStatusKey]
	if status == "" {
		logrus.WithFields(fieldsService(svc)).
			Debugf("skip update pod [%v/%v] corev1.Endpoints: pod network status not updated by multus",
				pod.Namespace, pod.Name)
		return nil
	}
	err := json.Unmarshal([]byte(status), &networksStatus)
	if err != nil {
		logrus.WithFields(fieldsService(svc)).
			Warningf("skip to update pod [%v/%v] endpoints: unmarshal json [%v]: %v",
				pod.Namespace, pod.Name, pod.Annotations[k8sCNINetworksStatusKey], err)
		return nil
	}

	// Find networks used by pod and match network annotation of this service
	for _, status := range networksStatus {
		if !isInNetworkSelectionElementsArray(status.Name, pod.Namespace, svcNetworks) {
			continue
		}
		// All IPs of matching network are added as endpoints
		for _, ip := range status.IPs {
			epAddress := corev1.EndpointAddress{
				IP:       ip,
				NodeName: &pod.Spec.NodeName,
				TargetRef: &corev1.ObjectReference{
					Kind:            "Pod",
					Name:            pod.GetName(),
					Namespace:       pod.GetNamespace(),
					ResourceVersion: pod.GetResourceVersion(),
					UID:             pod.GetUID(),
				},
			}
			addresses = append(addresses, epAddress)
		}
	}
	for i := range svc.Spec.Ports {
		// Check whether pod has the ports needed by service and add them to endpoints
		portNumber, err := podutil.FindPort(pod, &svc.Spec.Ports[i])
		if err != nil {
			logrus.WithFields(fieldsService(svc)).
				Warnf("update endpoint: failed to find pod port: %v, skip...", err)
			continue
		}

		port := corev1.EndpointPort{
			Port:     int32(portNumber),
			Protocol: svc.Spec.Ports[i].Protocol,
			Name:     svc.Spec.Ports[i].Name,
		}
		ports = append(ports, port)
	}
	subset := &corev1.EndpointSubset{
		Addresses: addresses,
		Ports:     ports,
	}
	return subset
}

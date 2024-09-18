package common

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	nettypes "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/sirupsen/logrus"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/kubernetes/pkg/api/v1/endpoints"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
)

type EndpointReource struct {
	Subsets       []corev1.EndpointSubset
	Endpoints     []discoveryv1.Endpoint
	EndpointPorts []discoveryv1.EndpointPort
}

func (r *EndpointReource) GetEndpointSliceAddressType() (discoveryv1.AddressType, error) {
	var t discoveryv1.AddressType
	for _, e := range r.Endpoints {
		if len(e.Addresses) == 0 {
			continue
		}
		for _, a := range e.Addresses {
			addr := net.ParseIP(a)
			if len(addr) == 0 {
				continue
			}
			if addr.To16() == nil {
				continue
			}
			if addr.To4() != nil {
				// Address type is IPv4
				if t == discoveryv1.AddressTypeIPv6 {
					return "", fmt.Errorf(
						"both IPv4 and IPv6 address types found in endpoint")
				}
				t = discoveryv1.AddressTypeIPv4
				continue
			}

			// Address type is IPv6
			if t == discoveryv1.AddressTypeIPv4 {
				return "", fmt.Errorf(
					"both IPv4 and IPv6 address types found in endpoint")
			}
			t = discoveryv1.AddressTypeIPv6
		}
	}

	if t == "" {
		return discoveryv1.AddressTypeIPv4, nil
	}
	return t, nil
}

var (
	ErrPodNetworkStatusNotUpdated = fmt.Errorf("pod network status not updated by multus")
)

// GetEndpointResources gets CoreV1 Endpoint Subsets and DiscoveryV1
// EndpointSlice resources by pods.
func GetEndpointResources(
	svc *corev1.Service, pods []*corev1.Pod,
) (*EndpointReource, error) {
	svcNetworkValue := svc.Annotations[nettypes.NetworkAttachmentAnnot]
	if svcNetworkValue == "" {
		return nil, nil
	}
	svcNetworkSelections, err := parseServiceNetworkSelections(svcNetworkValue, svc.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to parse service network annotation [%v: %v]: %w",
			nettypes.NetworkAttachmentAnnot, svcNetworkValue, err)
	}
	resource := &EndpointReource{
		Subsets:       make([]corev1.EndpointSubset, 0),
		Endpoints:     make([]discoveryv1.Endpoint, 0),
		EndpointPorts: make([]discoveryv1.EndpointPort, 0),
	}
	for _, pod := range pods {
		// Skip deleted pods
		if pod.DeletionTimestamp != nil {
			continue
		}
		subset, err := getPodEndpointSubset(svc, svcNetworkSelections, pod)
		if err != nil {
			return nil, err
		}
		if subset == nil || subset.Addresses == nil {
			continue
		}
		resource.Subsets = append(resource.Subsets, *subset)
	}
	resource.Subsets = endpoints.RepackSubsets(resource.Subsets)
	for _, subset := range resource.Subsets {
		for _, address := range subset.Addresses {
			endpoint := addressToEndpoint(address)
			resource.Endpoints = append(resource.Endpoints, endpoint)
		}
		ports := epPortsToEpsPorts(subset.Ports)
		resource.EndpointPorts = append(resource.EndpointPorts, ports...)
	}
	return resource, nil
}

var (
	validNameRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
)

func parsePodNetworkSelectionElement(
	selection, defaultNamespace string,
) (*types.NetworkSelectionElement, error) {
	var namespace, name, netInterface string
	var networkSelectionElement *types.NetworkSelectionElement

	units := strings.Split(selection, "/")
	switch len(units) {
	case 1:
		namespace = defaultNamespace
		name = units[0]
	case 2:
		namespace = units[0]
		name = units[1]
	default:
		return networkSelectionElement, fmt.Errorf(
			"invalid network selection element - more than one '/' rune in: '%s'", selection)
	}

	units = strings.Split(name, "@")
	switch len(units) {
	case 1:
		name = units[0]
		netInterface = ""
	case 2:
		name = units[0]
		netInterface = units[1]
	default:
		err := fmt.Errorf(
			"invalid network selection element - more than one '@' rune in: '%s'", selection)
		logrus.Error(err)
		return networkSelectionElement, err
	}

	for _, unit := range []string{namespace, name, netInterface} {
		ok := validNameRegex.MatchString(unit)
		if !ok && len(unit) > 0 {
			err := fmt.Errorf(
				"at least one of the network selection units is invalid: error found at '%s'", unit)
			logrus.Error(err)
			return networkSelectionElement, err
		}
	}

	networkSelectionElement = &types.NetworkSelectionElement{
		Namespace:        namespace,
		Name:             name,
		InterfaceRequest: netInterface,
	}

	return networkSelectionElement, nil
}

// Convert pod 'k8s.v1.cni.cncf.io/networks' annotation value to
// []*types.NetworkSelectionElement
func parseServiceNetworkSelections(
	podNetwork, defaultNamespace string,
) ([]*types.NetworkSelectionElement, error) {
	networkSelections := []*types.NetworkSelectionElement{}
	if len(podNetwork) == 0 {
		err := fmt.Errorf("empty string passed as network selection elements list")
		logrus.Error(err)
		return nil, err
	}

	err := json.Unmarshal([]byte(podNetwork), &networkSelections)
	if err != nil {
		for _, networkSelection := range strings.Split(podNetwork, ",") {
			networkSelection = strings.TrimSpace(networkSelection)
			networkSelectionElement, err := parsePodNetworkSelectionElement(networkSelection, defaultNamespace)
			if err != nil {
				return nil, fmt.Errorf("failed to parse network selection: %w", err)
			}
			networkSelections = append(networkSelections, networkSelectionElement)
		}
	}

	for _, networkSelection := range networkSelections {
		if networkSelection.Namespace == "" {
			networkSelection.Namespace = defaultNamespace
		}
	}
	return networkSelections, nil
}

func isInNetworkSelectionElementsArray(
	statusName, namespace string, networks []*types.NetworkSelectionElement,
) bool {
	// https://github.com/k8snetworkplumbingwg/multus-cni/blob/v4.0.2/pkg/types/conf.go#L117
	var netName, netNamespace string
	units := strings.SplitN(statusName, "/", 2)
	switch len(units) {
	case 1:
		netName = units[0]
		netNamespace = namespace
	case 2:
		netNamespace = units[0]
		netName = units[1]
	default:
		logrus.Debugf("skip invalid network status: %v", statusName)
		return false
	}
	for i := range networks {
		if netName == networks[i].Name && netNamespace == networks[i].Namespace {
			return true
		}
	}
	return false
}

func getPodEndpointSubset(
	svc *corev1.Service, svcNetworks []*types.NetworkSelectionElement, pod *corev1.Pod,
) (*corev1.EndpointSubset, error) {
	addresses := make([]corev1.EndpointAddress, 0)
	ports := make([]corev1.EndpointPort, 0)
	networksStatus := make([]nettypes.NetworkStatus, 0)
	status := pod.Annotations[nettypes.NetworkStatusAnnot]
	if status == "" {
		logrus.
			Debugf("skip update pod [%v/%v] corev1.Endpoints: pod network status not updated by multus",
				pod.Namespace, pod.Name)
		return nil, ErrPodNetworkStatusNotUpdated
	}
	err := json.Unmarshal([]byte(status), &networksStatus)
	if err != nil {
		logrus.
			Warningf("skip to update pod [%v/%v] endpoints: unmarshal json [%v]: %v",
				pod.Namespace, pod.Name, pod.Annotations[nettypes.NetworkStatusAnnot], err)
		return nil, fmt.Errorf("failed to parse pod network status json %q: %w", status, err)
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
			logrus.Warnf("update endpoint: failed to find pod port: %v, skip...", err)
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
	return subset, nil
}

// addressToEndpoint converts an address from an corev1.Endpoints resource to an
// discovertv1.EndpointSlice endpoint.
func addressToEndpoint(address corev1.EndpointAddress) discoveryv1.Endpoint {
	endpoint := discoveryv1.Endpoint{
		Addresses: []string{address.IP},
		Conditions: discoveryv1.EndpointConditions{
			Ready: utils.Ptr(true),
		},
		TargetRef: address.TargetRef,
	}

	if address.NodeName != nil {
		endpoint.NodeName = address.NodeName
	}
	if address.Hostname != "" {
		endpoint.Hostname = &address.Hostname
	}

	return endpoint
}

// epPortsToEpsPorts converts ports from an Endpoints resource to ports for an
// EndpointSlice resource.
func epPortsToEpsPorts(epPorts []corev1.EndpointPort) []discoveryv1.EndpointPort {
	epsPorts := []discoveryv1.EndpointPort{}
	for _, epPort := range epPorts {
		epp := epPort.DeepCopy() // TODO:
		epsPorts = append(epsPorts, discoveryv1.EndpointPort{
			Name:        &epp.Name,
			Port:        &epp.Port,
			Protocol:    &epp.Protocol,
			AppProtocol: epp.AppProtocol,
		})
	}
	return epsPorts
}

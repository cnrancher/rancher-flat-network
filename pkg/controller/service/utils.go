package service

import (
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"regexp"
	"strings"

	"github.com/cnrancher/rancher-flat-network-operator/pkg/utils"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/kubernetes/pkg/api/v1/endpoints"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"

	flv1 "github.com/cnrancher/rancher-flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
	nettypes "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
)

// isIngressService detects if this svc is created by Rancher for ingress.
// Reserved: Only manager UI have this feature and will remove in upcoming release
func isIngressService(svc *corev1.Service) bool {
	if svc == nil || svc.Name == "" {
		return false
	}
	if !strings.HasPrefix(svc.Name, "ingress-") {
		return false
	}

	for _, owner := range svc.OwnerReferences {
		if strings.ToLower(owner.Kind) == "ingress" {
			return true
		}
	}

	return false
}

func (h *handler) isWorkloadDisabledFlatNetwork(svc *corev1.Service) (bool, error) {
	if len(svc.Spec.Selector) == 0 {
		return true, nil
	}

	pods, err := h.podCache.List(svc.Namespace, labels.SelectorFromSet(svc.Spec.Selector))
	if err != nil {
		return false, fmt.Errorf("failed to list pod by selector [%v] on service [%v/%v]: %w",
			svc.Spec.Selector, svc.Namespace, svc.Name, err)
	}
	if len(pods) == 0 {
		return true, nil
	}

	// pod of this svc disabled flat-network service by annotation.
	for _, pod := range pods {
		if pod == nil {
			continue
		}
		annotations := pod.Annotations
		if annotations != nil && annotations[flv1.AnnotationFlatNetworkService] == "disabled" {
			return true, nil
		}
	}

	// Pod does not use flat-network.
	var podUseFlatNetwork bool
	for _, pod := range pods {
		if pod == nil {
			continue
		}
		if utils.IsPodEnabledFlatNetwork(pod) {
			podUseFlatNetwork = true
			break
		}
	}

	return !podUseFlatNetwork, nil
}

// newFlatNetworkService returns a flat-network headless sercive struct based on
// the provided existing service.
func newFlatNetworkService(svc *corev1.Service) *corev1.Service {
	svc = svc.DeepCopy()
	ports := []corev1.ServicePort{}
	for _, v := range svc.Spec.Ports {
		port := v.DeepCopy()
		port.NodePort = 0
		if svc.Spec.ClusterIP == corev1.ClusterIPNone {
			port.Port = port.Port + 1
			port.TargetPort = intstr.FromInt(port.TargetPort.IntValue() + 1)
		}
		ports = append(ports, *port)
	}

	// The flat-network service is owned by original service.
	ownerReference := svc.OwnerReferences
	ownerReference = append(ownerReference, metav1.OwnerReference{
		APIVersion: "v1",
		Kind:       "Service",
		Name:       svc.Name,
		UID:        svc.UID,
		// Controller: utils.Pointer(true),
	})

	s := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            fmt.Sprintf("%s%s", svc.Name, utils.FlatNetworkServiceNameSuffix),
			Namespace:       svc.Namespace,
			OwnerReferences: ownerReference,
			Annotations: map[string]string{
				utils.K8sCNINetworksKey: utils.NetAttatchDefName,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports:    ports,
			Selector: maps.Clone(svc.Spec.Selector),
			// Setting this to "None" makes a "headless service" (no virtual IP),
			// which is useful when direct endpoint connections are preferred and
			// proxying is not required.
			ClusterIP:  corev1.ClusterIPNone,
			ClusterIPs: []string{"None"},
			Type:       "ClusterIP",
		},
	}

	return s
}

// flatNetworkServiceUpdated returns true if the flat-network service already updated
func flatNetworkServiceUpdated(a, b *corev1.Service) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Name != b.Name || a.Namespace != b.Namespace {
		logrus.WithFields(fieldsService(a)).
			Debugf("service [%v/%v] name/namespace mismatch [%v/%v]",
				a.Namespace, a.Name, b.Namespace, b.Name)
		return false
	}
	if !equality.Semantic.DeepEqual(a.OwnerReferences, b.OwnerReferences) {
		logrus.WithFields(fieldsService(a)).
			Debugf("service OwnerReferences mismatch, a: %v\nb: %v",
				utils.Print(a.OwnerReferences), utils.Print(b.OwnerReferences))
		return false
	}
	if !equality.Semantic.DeepEqual(a.Annotations, b.Annotations) {
		logrus.WithFields(fieldsService(a)).
			Debugf("service Annotations mismatch: a: %v\n b: %v",
				utils.Print(a.Annotations), utils.Print(b.Annotations))
		return false
	}
	if !equality.Semantic.DeepEqual(a.Spec.Ports, b.Spec.Ports) {
		logrus.WithFields(fieldsService(a)).
			Debugf("service spec.ports mismatch: a: %v\nb: %v",
				utils.Print(a.Spec.Ports), utils.Print(b.Spec.Ports))
		return false
	}
	if !equality.Semantic.DeepEqual(a.Spec.Selector, b.Spec.Selector) {
		logrus.WithFields(fieldsService(a)).
			Debugf("service spec.selector mismatch: a: %v\nb: %v",
				utils.Print(a.Spec.Selector), utils.Print(b.Spec.Selector))
		return false
	}
	if !equality.Semantic.DeepEqual(a.Spec.Selector, b.Spec.Selector) {
		logrus.WithFields(fieldsService(a)).
			Debugf("service spec.selector mismatch: a: %v\nb: %v",
				utils.Print(a.Spec.Selector), utils.Print(b.Spec.Selector))
		return false
	}
	if !equality.Semantic.DeepEqual(a.Spec.ClusterIP, b.Spec.ClusterIP) {
		logrus.WithFields(fieldsService(a)).
			Debugf("service spec.clusterIP mismatch: a: %v\nb: %v",
				utils.Print(a.Spec.ClusterIP), utils.Print(b.Spec.ClusterIP))
		return false
	}
	if !equality.Semantic.DeepEqual(a.Spec.ClusterIPs, b.Spec.ClusterIPs) {
		logrus.WithFields(fieldsService(a)).
			Debugf("service spec.clusterIPs mismatch: a: %v\nb: %v",
				utils.Print(a.Spec.ClusterIPs), utils.Print(b.Spec.ClusterIPs))
		return false
	}
	if !equality.Semantic.DeepEqual(a.Spec.Type, b.Spec.Type) {
		logrus.WithFields(fieldsService(a)).
			Debugf("service spec.type mismatch: a: %v\nb: %v",
				utils.Print(a.Spec.Type), utils.Print(b.Spec.Type))
		return false
	}
	return true
}

// Convert pod 'k8s.v1.cni.cncf.io/networks' annotation value to
// []*types.NetworkSelectionElement
func parseServiceNetworkSelections(
	podNetwork, defaultNamespace string,
) ([]*types.NetworkSelectionElement, error) {
	networkSelections := []*types.NetworkSelectionElement{}
	if len(podNetwork) == 0 {
		err := errors.New("empty string passed as network selection elements list")
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

type endpointReource struct {
	subsets       []corev1.EndpointSubset
	endpoints     []discoveryv1.Endpoint
	endpointPorts []discoveryv1.EndpointPort
}

func (r *endpointReource) getEndpointSliceAddressType() (discoveryv1.AddressType, error) {
	var t discoveryv1.AddressType
	for _, e := range r.endpoints {
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

// getEndpointResources gets CoreV1 Endpoint Subsets and DiscoveryV1
// EndpointSlice resources by pods.
func (h *handler) getEndpointResources(
	svc *corev1.Service, pods []*corev1.Pod,
) (*endpointReource, error) {
	svcNetworkValue := svc.Annotations[utils.K8sCNINetworksKey]
	if svcNetworkValue == "" {
		return nil, nil
	}
	svcNetworkSelections, err := parseServiceNetworkSelections(svcNetworkValue, svc.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to parse service network annotation [%v: %v]: %w",
			utils.K8sCNINetworksKey, svcNetworkValue, err)
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
	status := pod.Annotations[utils.K8sCNINetworksStatusKey]
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
				pod.Namespace, pod.Name, pod.Annotations[utils.K8sCNINetworksStatusKey], err)
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

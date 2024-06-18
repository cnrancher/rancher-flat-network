package service

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"sort"
	"strings"

	"github.com/cnrancher/flat-network-operator/pkg/utils"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"

	flv1 "github.com/cnrancher/flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
)

const (
	k8sCNINetworksKey       = "k8s.v1.cni.cncf.io/networks"
	k8sCNINetworksStatusKey = "k8s.v1.cni.cncf.io/networks-status"
	netAttatchDefName       = "static-flat-network-cni-attach"
)

// isIngressService detects if this svc is owned by Rancher managed ingress.
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

// Check if this service is a flat-network service.
// A Flat-Network Service is a ClusterIP typed headless service, name ends with
// '-flatnetwork' suffix.
func isFlatNetworkService(svc *corev1.Service) bool {
	if !strings.HasSuffix(svc.Name, flatNetworkServiceNameSuffix) {
		return false
	}
	if svc.Spec.Type != "ClusterIP" {
		return false
	}
	if len(svc.Spec.ClusterIPs) != 0 {
		if svc.Spec.ClusterIPs[0] != "None" {
			return false
		}
	} else if svc.Spec.ClusterIP != "None" {
		return false
	}
	return true
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
			Name:            fmt.Sprintf("%s%s", svc.Name, flatNetworkServiceNameSuffix),
			Namespace:       svc.Namespace,
			OwnerReferences: ownerReference,
			Annotations: map[string]string{
				k8sCNINetworksKey: netAttatchDefName,
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

func getNetworkAnnotations(obj interface{}) string {
	metaObject := obj.(metav1.Object)
	annotations, ok := metaObject.GetAnnotations()[k8sCNINetworksKey]
	if !ok {
		return ""
	}
	return annotations
}

// NOTE: two below functions are copied from the net-attach-def admission controller, to be replaced with better implementation
func parsePodNetworkSelections(podNetworks, defaultNamespace string) ([]*types.NetworkSelectionElement, error) {
	var networkSelections []*types.NetworkSelectionElement

	if len(podNetworks) == 0 {
		err := errors.New("empty string passed as network selection elements list")
		logrus.Error(err)
		return nil, err
	}

	/* try to parse as JSON array */
	err := json.Unmarshal([]byte(podNetworks), &networkSelections)

	/* if failed, try to parse as comma separated */
	if err != nil {
		logrus.Infof("'%s' is not in JSON format: %s... trying to parse as comma separated network selections list", podNetworks, err)
		for _, networkSelection := range strings.Split(podNetworks, ",") {
			networkSelection = strings.TrimSpace(networkSelection)
			networkSelectionElement, err := parsePodNetworkSelectionElement(networkSelection, defaultNamespace)
			if err != nil {
				err := errors.Wrap(err, "error parsing network selection element")
				logrus.Error(err)
				return nil, err
			}
			networkSelections = append(networkSelections, networkSelectionElement)
		}
	}

	/* fill missing namespaces with default value */
	for _, networkSelection := range networkSelections {
		if networkSelection.Namespace == "" {
			networkSelection.Namespace = defaultNamespace
		}
	}

	return networkSelections, nil
}

func parsePodNetworkSelectionElement(selection, defaultNamespace string) (*types.NetworkSelectionElement, error) {
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
		err := errors.Errorf("invalid network selection element - more than one '/' rune in: '%s'", selection)
		logrus.Error(err)
		return networkSelectionElement, err
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
		err := errors.Errorf("invalid network selection element - more than one '@' rune in: '%s'", selection)
		logrus.Error(err)
		return networkSelectionElement, err
	}

	validNameRegex, _ := regexp.Compile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	for _, unit := range []string{namespace, name, netInterface} {
		ok := validNameRegex.MatchString(unit)
		if !ok && len(unit) > 0 {
			err := errors.Errorf("at least one of the network selection units is invalid: error found at '%s'", unit)
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

func isInNetworkSelectionElementsArray(name, namespace string, networks []*types.NetworkSelectionElement) bool {
	// https://github.com/k8snetworkplumbingwg/multus-cni/blob/v3.7.2/pkg/types/conf.go#L109
	var netName, netNamespace string
	units := strings.SplitN(name, "/", 2)
	switch len(units) {
	case 1:
		netName = units[0]
		netNamespace = namespace
	case 2:
		netNamespace = units[0]
		netName = units[1]
	default:
		// TODO: err := errors.Errorf("invalid network status - '%s'", name)
		// logrus.Error(err)
		return false
	}
	for i := range networks {
		if netName == networks[i].Name && netNamespace == networks[i].Namespace {
			return true
		}
	}
	return false
}

// addressToEndpoint converts an address from an Endpoints resource to an
// EndpointSlice endpoint.
func addressToEndpoint(address corev1.EndpointAddress) discovery.Endpoint {
	endpoint := discovery.Endpoint{
		Addresses: []string{address.IP},
		Conditions: discovery.EndpointConditions{
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
func epPortsToEpsPorts(epPorts []corev1.EndpointPort) []discovery.EndpointPort {
	epsPorts := []discovery.EndpointPort{}
	for _, epPort := range epPorts {
		epp := epPort.DeepCopy() // TODO:
		epsPorts = append(epsPorts, discovery.EndpointPort{
			Name:        &epp.Name,
			Port:        &epp.Port,
			Protocol:    &epp.Protocol,
			AppProtocol: epp.AppProtocol,
		})
	}
	return epsPorts
}

func sortEpsEndpoints(eps []discovery.Endpoint) {
	sort.Slice(eps, func(i, j int) bool {
		return strings.Compare(eps[i].TargetRef.Name, eps[j].TargetRef.Name) > 0
	})
}

func sortEpsPorts(ports []discovery.EndpointPort) {
	sort.Slice(ports, func(i, j int) bool {
		return strings.Compare(*ports[i].Name, *ports[j].Name) > 0
	})
}

func filterEpsList(eps []*discovery.EndpointSlice) []*discovery.EndpointSlice {
	result := []*discovery.EndpointSlice{}
	if len(eps) == 0 {
		return result
	}

	for _, e := range eps {
		if e.AddressType != discovery.AddressTypeIPv4 || e.DeletionTimestamp != nil {
			continue
		}
		result = append(result, e)
	}

	return result
}

package service

import (
	"encoding/json"
	"fmt"
	"strings"

	flv1 "github.com/cnrancher/flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	"k8s.io/kubernetes/pkg/api/v1/endpoints"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"

	nettypes "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
)

func (h *handler) handleFlatNetworkService(
	svc *corev1.Service,
) (*corev1.Service, error) {
	logrus.WithFields(fieldsService(svc)).
		Debugf("service is a flat-network service")

	pods, err := h.podCache.List(svc.Namespace, labels.SelectorFromSet(svc.Spec.Selector))
	if err != nil {
		return svc, fmt.Errorf("failed to list pod by selector [%v] on service [%v/%v]: %w",
			svc.Spec.Selector, svc.Namespace, svc.Name, err)
	}
	ok, err := h.shouldDeleteFlatNetworkService(svc, pods)
	if err != nil {
		logrus.WithFields(fieldsService(svc)).
			Errorf("failed to sync flat-network service: %v", err)
		return nil, err
	}
	if ok {
		logrus.WithFields(fieldsService(svc)).
			Infof("request to delete flat-network service")
		err = h.serviceClient.Delete(svc.Namespace, svc.Name, &metav1.DeleteOptions{})
		if err != nil {
			logrus.WithFields(fieldsService(svc)).
				Errorf("failed to delete flat-network service: %v", err)
			return svc, err
		}
		return svc, nil
	}

	// ----------------------------------------------------------------------

	// read network annotations from the service
	annotations := getNetworkAnnotations(svc)
	if len(annotations) == 0 {
		return svc, nil
	}
	logrus.Infof("service network annotation found: %v", annotations)
	networks, err := parsePodNetworkSelections(annotations, svc.Namespace)
	if err != nil {
		logrus.Errorf("service network annotation parse error: %v", err)
		return svc, nil
	}

	// get endpoints of the service
	ep, err := h.endpointsCache.Get(svc.Namespace, svc.Name)
	if err != nil {
		logrus.WithFields(fieldsService(svc)).
			Errorf("failed to get service endpoints: %s", err)
		return svc, err
	}

	subsets := make([]corev1.EndpointSubset, 0)
	epsForEndpointSlice := make([]discoveryv1.Endpoint, 0)
	epPortsForEndpointSlice := make([]discoveryv1.EndpointPort, 0)

	for _, pod := range pods {
		if pod.DeletionTimestamp != nil {
			continue
		}
		addresses := make([]corev1.EndpointAddress, 0)
		ports := make([]corev1.EndpointPort, 0)

		networksStatus := make([]nettypes.NetworkStatus, 0)
		err := json.Unmarshal([]byte(pod.Annotations[k8sCNINetworksStatusKey]), &networksStatus)
		if err != nil {
			logrus.Warningf("skip to update for pod %s as networks status are not expected: %v", pod.Name, err)
			continue
		}
		// find networks used by pod and match network annotation of the service
		for _, status := range networksStatus {
			if isInNetworkSelectionElementsArray(status.Name, pod.Namespace, networks) {
				logrus.Infof("processing pod %s/%s: found network %s interface %s with IP addresses %s",
					pod.Namespace, pod.Name, annotations, status.Interface, status.IPs)
				// all IPs of matching network are added as endpoints
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

					esAddress := addressToEndpoint(epAddress)
					epsForEndpointSlice = append(epsForEndpointSlice, esAddress)
				}
			}
		}
		for i := range svc.Spec.Ports {
			// check whether pod has the ports needed by service and add them to endpoints if so
			portNumber, err := podutil.FindPort(pod, &svc.Spec.Ports[i])
			if err != nil {
				logrus.Infof("Could not find pod port for service %s/%s: %s, skipping...", svc.Namespace, svc.Name, err)
				continue
			}

			port := corev1.EndpointPort{
				Port:     int32(portNumber),
				Protocol: svc.Spec.Ports[i].Protocol,
				Name:     svc.Spec.Ports[i].Name,
			}
			ports = append(ports, port)
		}
		subset := corev1.EndpointSubset{
			Addresses: addresses,
			Ports:     ports,
		}
		subsets = append(subsets, subset)
	}

	var updatedEndpoint *corev1.Endpoints
	// update endpoints resource
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err := h.endpointsCache.Get(ep.Namespace, ep.Name)
		if err != nil {
			logrus.Errorf("Failed to get latest version of endpoints: %v", err)
			return err
		}

		// repack subsets - NOTE: too naive? additional checks needed?
		toUpdateSubsets := endpoints.RepackSubsets(subsets)
		// to update ports for endpointslice
		for _, subset := range toUpdateSubsets {
			epPorts := epPortsToEpsPorts(subset.Ports)
			epPortsForEndpointSlice = append(epPortsForEndpointSlice, epPorts...)
		}

		// check if need to call an update
		if apiequality.Semantic.DeepDerivative(toUpdateSubsets, result.Subsets) {
			logrus.Infof("skip to update endpoints %s as semantic deep derivative", result.Name)
			return nil
		}

		resultCopy := result.DeepCopy()

		resultCopy.SetOwnerReferences(
			[]metav1.OwnerReference{
				*metav1.NewControllerRef(svc, schema.GroupVersionKind{
					Group:   corev1.SchemeGroupVersion.Group,
					Version: corev1.SchemeGroupVersion.Version,
					Kind:    "Service",
				}),
			},
		)

		if resultCopy.Labels == nil {
			resultCopy.Labels = map[string]string{}
		}
		resultCopy.Labels[discoveryv1.LabelSkipMirror] = "true"

		resultCopy.Subsets = toUpdateSubsets
		// updatedEndpoint, err = c.k8sClientSet.CoreV1().Endpoints(ep.Namespace).Update(context.TODO(), resultCopy, metav1.UpdateOptions{})
		updatedEndpoint, err = h.endpointsClient.Update(resultCopy)
		return err
	})
	if retryErr != nil {
		logrus.Errorf("endpoint update error: %v", retryErr)
		return svc, retryErr
	}

	// msg := fmt.Sprintf("Updated to use network %s", annotations)
	if updatedEndpoint != nil {
		logrus.Info("endpoint updated successfully")
		// c.recorder.Event(ep, corev1.EventTypeNormal, msg, "Endpoints update successful")
		// c.recorder.Event(svc, corev1.EventTypeNormal, msg, "Endpoints update successful")
	}

	if !h.supportDiscoveryV1 {
		logrus.Info("no need to update endpointslice as k8s is not support discovery.k8s.io/v1")
		return svc, nil
	}

	endpointSliceUpdated := false
	sortEpsEndpoints(epsForEndpointSlice)
	sortEpsPorts(epPortsForEndpointSlice)
	logrus.Infof("trying to update endpointslice with %#v", epsForEndpointSlice)
	retryErr = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		endpointSlices, err := h.endpointSliceCache.List(svc.Namespace, labels.SelectorFromSet(map[string]string{
			discoveryv1.LabelServiceName: svc.Name,
		}))
		if err != nil {
			logrus.Errorf("endpointslice list error: %v", err)
			return err
		}

		toActionList := filterEpsList(endpointSlices)

		for _, endpointSlice := range toActionList {
			esCopy := endpointSlice.DeepCopy()
			epsCopy := esCopy.Endpoints
			portsCopy := esCopy.Ports
			sortEpsEndpoints(epsCopy)
			sortEpsPorts(portsCopy)
			logrus.Infof("### Endpoint copy: %#v", epsCopy)
			logrus.Infof("### Endpoint compared: %t", apiequality.Semantic.DeepDerivative(epsForEndpointSlice, epsCopy))
			logrus.Infof("### EndpointPort length %d ---- %d", len(portsCopy), len(epPortsForEndpointSlice))
			logrus.Infof("### EndpointPort compared %t", apiequality.Semantic.DeepDerivative(epPortsForEndpointSlice, portsCopy))
			if len(esCopy.Endpoints) == len(epsForEndpointSlice) &&
				apiequality.Semantic.DeepDerivative(epsForEndpointSlice, epsCopy) &&
				apiequality.Semantic.DeepDerivative(epPortsForEndpointSlice, portsCopy) {
				logrus.Infof("skip to update endpointslice %s as semantic deep derivative", esCopy.Name)
				continue
			}
			// TODO:
			esCopy.Labels[discoveryv1.LabelManagedBy] = "rancher-flat-network-controller"
			esCopy.Endpoints = epsForEndpointSlice
			esCopy.Ports = epPortsForEndpointSlice
			_, err = h.endpointSliceClient.Update(esCopy)
			if err != nil {
				logrus.Errorf("endpointslice update error: %v", err)
				return err
			}
			// c.recorder.Event(esCopy, corev1.EventTypeNormal, msg, "EndpointSlices update successful")
			endpointSliceUpdated = true
		}

		if len(toActionList) > 1 {
			for _, endpointSlice := range toActionList[1:] {
				// err := c.k8sClientSet.DiscoveryV1().EndpointSlices(endpointSlice.Namespace).Delete(context.TODO(),
				// 	endpointSlice.Name, metav1.DeleteOptions{})
				err = h.endpointSliceClient.Delete(endpointSlice.Namespace, endpointSlice.Name, &metav1.DeleteOptions{})
				if err != nil {
					logrus.Errorf("endpointslice delete error: %v", err)
					continue
				}
				logrus.Infof("deleted endpointslice %s", endpointSlice.Name)
			}
		}
		return nil
	})
	if retryErr != nil {
		logrus.Errorf("endpointslice update error: %v", retryErr)
		return svc, retryErr
	}

	if endpointSliceUpdated {
		logrus.Info("endpointslice updated successfully")
		// c.recorder.Event(svc, corev1.EventTypeNormal, msg, "EndpointSlices update successful")
	}

	return svc, nil
}

func (h *handler) shouldDeleteFlatNetworkService(
	svc *corev1.Service, pods []*corev1.Pod,
) (bool, error) {
	if len(svc.Spec.Selector) == 0 {
		return true, nil
	}

	originalServiceName := strings.TrimSuffix(svc.Name, flatNetworkServiceNameSuffix)
	originalService, err := h.serviceCache.Get(svc.Namespace, originalServiceName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Delete if no original service.
			logrus.WithFields(fieldsService(svc)).
				Infof("original service of flat-network service [%v/%v] not found",
					svc.Namespace, originalServiceName)
			return true, nil
		}
		return false, fmt.Errorf("failed to get service [%v/%v] from cache: %w",
			svc.Namespace, originalService.Name, err)
	}

	if len(pods) == 0 {
		logrus.WithFields(fieldsService(svc)).
			Infof("no pods on flat-network service [%v/%v]",
				svc.Namespace, svc.Name)
		return true, nil
	}

	// Workload of this svc disabled flat-network service by annotation.
	for _, pod := range pods {
		if pod == nil {
			continue
		}
		annotations := pod.Annotations
		if annotations != nil && annotations[flv1.AnnotationFlatNetworkService] == "disabled" {
			logrus.WithFields(fieldsService(svc)).
				Infof("annotation [%v: disabled] found, flat-network service disabled",
					flv1.AnnotationFlatNetworkService)
			return true, nil
		}
	}

	// Workload does not enabled flat-network.
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
	if !podUseFlatNetwork {
		logrus.WithFields(fieldsService(svc)).
			Infof("workload does not use flat-network")
	}

	return !podUseFlatNetwork, nil
}

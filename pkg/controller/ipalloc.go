package controller

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"

	corecontroller "github.com/cnrancher/macvlan-operator/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	v1 "github.com/cnrancher/macvlan-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	"github.com/cnrancher/macvlan-operator/pkg/ipcalc"
	"github.com/sirupsen/logrus"
)

const (
	messageNoEnoughIP = "no enough ip resouce in subnet: %s"
	k8sCNINetworksKey = "k8s.v1.cni.cncf.io/networks"
	netAttatchDefName = "static-macvlan-cni-attach"
	cniARPPolicyEnv   = "PANDARIA_MACVLAN_CNI_ARP_POLICY"
	cniProxyARPEnv    = "PANDARIA_MACVLAN_CNI_PROXY_ARP"

	defaultARPPolicy = "arping"
)

var (
	netAttatchDef = schema.GroupVersionResource{
		Group:    "k8s.cni.cncf.io",
		Version:  "v1",
		Resource: "network-attachment-definitions",
	}
)

func (h *Handler) allocateAutoIP(pod *corev1.Pod, subnet *v1.MacvlanSubnet, annotationMac string) (net.IP, string, string, error) {
	h.mux.Lock()
	defer h.mux.Unlock()

	hosts, err := CalcSubnetHosts(subnet)
	if err != nil {
		return nil, "", "", err
	}

	usable := ipcalc.RemoveUsedHosts(hosts, getUsedIPsInSyncmap(h.inUsedIPs, subnet.Name))
	if len(usable) == 0 {
		// try get deleting pod which uses auto mode
		logrus.Debugf("allocateAutoIP: try to getPotentialIPInAutoMode for %s", pod.Name)
		potentialIP := getPotentialIPInAutoMode(h.inUsedIPs, h.pods, subnet.Name)
		if potentialIP == nil {
			return nil, "", "", fmt.Errorf(messageNoEnoughIP, subnet.Name)
		}
		logrus.Debugf("allocateAutoIP: done to getPotentialIPInAutoMode %s for %s", potentialIP.String(), pod.Name)
		usable = append(usable, potentialIP)
	}

	var ip net.IP
	var cidr string

	// ignore the ip which used in the specific mode workload
	for _, uIP := range usable {
		if err := h.checkFixedIPsFromWorkloadInformer(uIP.String(), subnet.Name); err == nil {
			ip = uIP
			cidr = convertIPtoCIDR(uIP, subnet.Spec.CIDR)
			break
		} else {
			logrus.Debugf("allocateAutoIP: %v", err)
		}
	}
	if ip == nil {
		return nil, "", "", fmt.Errorf(messageNoEnoughIP, subnet.Name)
	}

	// empty annotation mac address
	if annotationMac == "" {
		return ip, cidr, "", nil
	}

	// multiple mac address
	if strings.Contains(annotationMac, "-") {
		macs := strings.Split(strings.Trim(annotationMac, " "), "-")
		for _, v := range macs {
			if _, err := net.ParseMAC(v); err != nil {
				return nil, "", "", fmt.Errorf("allocateAutoIP: parse multipe mac address error: %v %v", err, v)
			}
			vv, ok := h.inUsedMacForAuto.Load(v)
			if !ok {
				return ip, cidr, v, nil
			}
			temp := strings.SplitN(vv.(string), ":", 2)
			// sameMacPod, _ := h.podLister.Pods(temp[0]).Get(temp[1])
			sameMacPod, err := h.pods.Get(temp[0], temp[1], metav1.GetOptions{})
			if err != nil {
				logrus.Warnf("allocateAutoIP: failed to get pod: %v", err)
			}
			if sameMacPod != nil && sameMacPod.DeletionTimestamp != nil {
				// we can use this mac as the pod will be deleted
				return ip, cidr, v, nil
			}
		}
		return nil, "", "", fmt.Errorf("allocateAutoIP: not enough mac resouce in annotations: %s", annotationMac)
	}

	// single mac address
	if _, err := net.ParseMAC(annotationMac); err != nil {
		return nil, "", "", fmt.Errorf("allocateAutoIP: parse single mac addresses %s, err %v", annotationMac, err)
	}
	if v, ok := h.inUsedMacForAuto.Load(annotationMac); ok {
		temp := strings.SplitN(v.(string), ":", 2)
		// sameMacPod, _ := h.podLister.Pods(temp[0]).Get(temp[1])
		sameMacPod, err := h.pods.Get(temp[0], temp[1], metav1.GetOptions{})
		if err != nil {
			logrus.Warnf("allocateAutoIP: failed to get pod: %v", err)
		}
		if sameMacPod != nil && sameMacPod.DeletionTimestamp == nil {
			return nil, "", "", fmt.Errorf("allocateAutoIP: not enough mac resouce in annotations: %s", annotationMac)
		}
	}

	return ip, cidr, annotationMac, nil
}

func (h *Handler) allocateSingleIP(pod *corev1.Pod, subnet *v1.MacvlanSubnet, ipString string, annotationMac string) (net.IP, string, string, error) {
	hosts, err := CalcSubnetHosts(subnet)
	if err != nil {
		return nil, "", "", err
	}

	ip := net.ParseIP(ipString)

	if err := checkIPValidation(ip, hosts); err != nil {
		return nil, "", "", err
	}
	if err := h.checkIPConflictInSpecificMode(pod, ipString, subnet.Name); err != nil {
		return nil, "", "", err
	}

	cidr := convertIPtoCIDR(ip, subnet.Spec.CIDR)

	if annotationMac != "" {
		if _, err := net.ParseMAC(annotationMac); err != nil {
			return nil, "", "", fmt.Errorf("allocateSingleIP: parse single mac address error: %v %s", err, annotationMac)
		}
	}
	return ip, cidr, annotationMac, nil
}

func (h *Handler) allocateMultipleIP(pod *corev1.Pod, subnet *v1.MacvlanSubnet, annotationIP string, annotationMac string) (net.IP, string, string, error) {
	h.mux.Lock()
	defer h.mux.Unlock()

	macs := []string{}
	ips := strings.Split(strings.Trim(annotationIP, " "), "-")

	// auto allcated mac
	if annotationMac != "" {
		// multiple mac
		if strings.Contains(annotationMac, "-") {
			macs = strings.Split(strings.Trim(annotationMac, " "), "-")
			if len(macs) != len(ips) {
				return nil, "", "", fmt.Errorf("allocateMultipleIP: count of multiple IP and Mac not equal: %s %s", annotationIP, annotationMac)
			}
			for _, v := range macs {
				if _, err := net.ParseMAC(v); err != nil {
					return nil, "", "", fmt.Errorf("allocateMultipleIP: parse multipe mac address error: %v %v", err, v)
				}
			}
		} else {
			return nil, "", "", fmt.Errorf("allocateMultipleIP: count of multiple IP and Mac not equal: %s %s", annotationIP, annotationMac)
		}
	}

	ipPool := map[string]bool{} // the value true means can be used, the value false means cannot be used
	for _, v := range ips {
		ipPool[v] = true
	}

	hosts, err := CalcSubnetHosts(subnet)
	if err != nil {
		return nil, "", "", err
	}

	var validIP net.IP
	var validMAC string
	for i, key := range ips {
		logrus.Debugf("allocateMultipleIP: try to checkIPValidation %s", key)
		ip := net.ParseIP(key)
		if err := checkIPValidation(ip, hosts); err != nil {
			logrus.Debugf("allocateMultipleIP: failed to checkIPValidation %s, err %v, continue", key, err)
			continue
		}

		logrus.Debugf("allocateMultipleIP: try to checkIPConflictInSpecificMode %s", key)
		if err := h.checkIPConflictInSpecificMode(pod, key, subnet.Name); err != nil {
			logrus.Debugf("allocateMultipleIP: try to checkIPConflictInSpecificMode %s, err %v, continue", key, err)
			continue
		}

		mac := ""
		if len(macs) != 0 {
			mac = macs[i]
		}

		validIP = ip
		validMAC = mac
		break
	}

	if validIP == nil {
		// error event: no enough ip
		return nil, "", "", fmt.Errorf("No enough ip resouce in subnet: %s", annotationIP)
	}

	return validIP, convertIPtoCIDR(validIP, subnet.Spec.CIDR), validMAC, nil
}

func (h *Handler) checkIPConflictInSpecificMode(pod *corev1.Pod, ip, subnet string) error {
	key := fmt.Sprintf("%s:%s", ip, subnet)
	comparedValue, ok := h.inUsedIPs.Load(key)
	if !ok {
		return nil
	}
	logrus.Debugf("checkIPConflictInSpecificMode: got same key %s from syncmap, %s", key, comparedValue)
	expectedValue := fmt.Sprintf("%s:%s", pod.Namespace, pod.Name)
	if comparedValue != expectedValue {
		temp := strings.SplitN(comparedValue.(string), ":", 2)
		// comparedPod, _ := h.podLister.Pods(temp[0]).Get(temp[1])
		comparedPod, err := h.pods.Get(temp[0], temp[1], metav1.GetOptions{})
		if err != nil {
			logrus.Warnf("checkIPConflictInSpecificMode: failed to get pod: %v", err)
		}
		if comparedPod != nil {
			// delete the compared Pod if it uses the same IP and is auto mode
			workload := pod.Labels[v1.LabelWorkloadSelector]
			comparedWorkload := comparedPod.Labels[v1.LabelWorkloadSelector]
			if comparedPod.Annotations[v1.AnnotationIP] == "auto" && comparedWorkload != workload {
				logrus.Warnf("checkIPConflictInSpecificMode: got a pod using the specific IP, will delete it, %s %s", comparedPod.Namespace, comparedPod.Name)
				// h.kubeClientset.CoreV1().Pods(comparedPod.Namespace).Delete(context.TODO(), comparedPod.Name, metav1.DeleteOptions{})
				if err := h.pods.Delete(comparedPod.Namespace, comparedPod.Name, &metav1.DeleteOptions{}); err != nil {
					logrus.Warnf("checkIPConflictInSpecificMode: failed to delete pod: %v", err)
				}
				return nil
			}
			// will return error if the compared Pod is not on deleting
			if comparedPod.DeletionTimestamp == nil {
				return fmt.Errorf("checkIPConflictInSpecificMode: the %s has been allocaed by %s from syncmap", ip, comparedValue)
			}
		}
	}
	return nil
}

func getPotentialIPInAutoMode(
	cacheIPs *sync.Map,
	podsClient corecontroller.PodClient,
	subnetName string,
) net.IP {
	// pods, err := podLister.List(labels.Everything())
	labels.Everything()
	pods, err := podsClient.List("", metav1.ListOptions{
		LabelSelector: labels.Everything().String(),
	})
	if err != nil {
		logrus.Warnf("getPotentialIPInAutoMode: Failed to list pod: %v", err)
		return nil
	}
	if len(pods.Items) == 0 {
		return nil
	}
	usableMap := map[string]string{}
	for _, pod := range pods.Items {
		if pod.Annotations == nil {
			continue
		}
		podSubnetName := pod.Annotations[v1.AnnotationSubnet]
		// if a Pod is about to be deleted, its IP can be occupied
		if pod.DeletionTimestamp != nil && podSubnetName == subnetName {
			if selectedIP := pod.Labels[v1.LabelSelectedIP]; selectedIP != "" {
				key := fmt.Sprintf("%s:%s", selectedIP, subnetName)
				owner := fmt.Sprintf("%s:%s", pod.Namespace, pod.Name)
				usableMap[key] = owner
			}
		}
	}

	for usableK, usableV := range usableMap {
		value, ok := cacheIPs.Load(usableK)
		ip := strings.SplitN(usableK, ":", 2)[0]
		if !ok {
			// no pod use ip, we can use this ip for new pod
			return net.ParseIP(ip)
		}

		if value == usableV {
			// means that other Pods did not preempt this IP in advance
			return net.ParseIP(ip)
		}
	}
	return nil
}

func getUsedIPsInSyncmap(ips *sync.Map, subnetName string) []net.IP {
	used := []net.IP{}
	ips.Range(func(k, v any) bool {
		temp := strings.SplitN(k.(string), ":", 2)
		ip := net.ParseIP(temp[0])
		if ip != nil && subnetName == temp[1] {
			used = append(used, ip)
		}
		return true
	})
	return used
}

func convertIPtoCIDR(ip net.IP, cidr string) string {
	nets := strings.Split(cidr, "/")
	suffix := ""
	if len(nets) == 2 {
		suffix = nets[1]
	}
	return ip.String() + "/" + suffix
}

func IsIPsInSubnet(ips []string, subnet *v1.MacvlanSubnet) error {
	hosts, err := CalcSubnetHosts(subnet)
	if err != nil {
		return fmt.Errorf("calc subnet hosts error: %v", err)
	}
	for _, checkip := range ips {
		if checkip == subnet.Spec.Gateway {
			return fmt.Errorf("ip use by subnet as gateway: %v", checkip)
		}

		err := func() error {
			for _, hostip := range hosts {
				if hostip.String() == checkip {
					return nil
				}
			}
			return fmt.Errorf("ip %v not in subnet %v hosts: ", checkip, subnet.Name)
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

func CalcSubnetHosts(subnet *v1.MacvlanSubnet) ([]net.IP, error) {
	hosts, err := ipcalc.CIDRtoHosts(subnet.Spec.CIDR)
	if err != nil {
		return nil, err
	}

	ranges := calcHostsInRanges(subnet.Spec.Ranges)
	if len(ranges) != 0 {
		usable := ipcalc.GetUsableHosts(hosts, ranges)
		return usable, nil
	}

	return hosts, nil
}

func calcHostsInRanges(ranges []v1.IPRange) []net.IP {
	hosts := []net.IP{}
	for _, v := range ranges {
		ips := ipcalc.ParseIPRange(v.RangeStart, v.RangeEnd)
		hosts = append(hosts, ips...)
	}
	return removeDuplicatesFromSlice(hosts)
}

func removeDuplicatesFromSlice(hosts []net.IP) []net.IP {
	m := make(map[string]bool)
	result := []net.IP{}
	for _, item := range hosts {
		if _, ok := m[item.String()]; ok {

		} else {
			m[item.String()] = true
			result = append(result, item)
		}
	}
	return result
}

func isSingleIP(ip string) bool {
	return nil != net.ParseIP(ip)
}

func checkIPValidation(ip net.IP, hosts []net.IP) error {
	if !isInHosts(hosts, ip) {
		return fmt.Errorf("checkIPValidation: %s is invalid", ip.String())
	}
	return nil
}

func isInHosts(h []net.IP, ip net.IP) bool {
	for _, v := range h {
		if bytes.Compare(v, ip) == 0 {
			return true
		}
	}
	return false
}

func isMultipleIP(ip string) bool {
	if !strings.Contains(ip, "-") {
		return false
	}
	ips := strings.Split(strings.Trim(ip, " "), "-")

	if len(ips) < 2 {
		return false
	}

	for _, v := range ips {
		if net.ParseIP(v) == nil {
			return false
		}
	}
	return true
}

func ListReservedFixedIPsExcept(kubeClientset kubernetes.Interface, subnet string, kind string, namespace string, name string) map[string][]string {
	listOpts := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s,%s=%s",
			v1.LabelSubnet, subnet,
			v1.LabelMacvlanIPType, "specific"),
	}

	result, err := getWorkloadFixedIPListExcept(kubeClientset, listOpts, kind, namespace, name)
	if err != nil {
		logrus.Errorf("%v", err)
		return result
	}

	return result
}

func getWorkloadFixedIPListExcept(kubeClientset kubernetes.Interface, opts metav1.ListOptions, kind string, namespace string, name string) (map[string][]string, error) {
	var err error
	ips := map[string][]string{}

	dp, err := kubeClientset.AppsV1().Deployments("").List(context.TODO(), opts)
	if err == nil && len(dp.Items) != 0 {
		for _, workload := range dp.Items {
			if !("Deployment" == kind && workload.Namespace == namespace && workload.Name == name) {
				if workload.Spec.Template.Annotations != nil {
					annotationIPs := workload.Spec.Template.Annotations[v1.AnnotationIP]
					if annotationIPs != "" && annotationIPs != "auto" {
						if isSingleIP(annotationIPs) {
							ips[annotationIPs] = append(ips[annotationIPs], fmt.Sprintf("%s:%s", workload.Namespace, workload.Name))
							ips[annotationIPs] = unique(ips[annotationIPs])
						} else if isMultipleIP(annotationIPs) {
							workloadIPs := strings.Split(strings.Trim(annotationIPs, " "), "-")
							for _, ip := range workloadIPs {
								ips[ip] = append(ips[ip], fmt.Sprintf("%s:%s", workload.Namespace, workload.Name))
								ips[ip] = unique(ips[ip])
							}
						}
					}
				}
			}
		}
	}

	ds, err := kubeClientset.AppsV1().DaemonSets("").List(context.TODO(), opts)
	if err == nil && len(ds.Items) != 0 {
		for _, workload := range ds.Items {
			if !("DaemonSet" == kind && workload.Namespace == namespace && workload.Name == name) {
				if workload.Spec.Template.Annotations != nil {
					annotationIPs := workload.Spec.Template.Annotations[v1.AnnotationIP]
					if annotationIPs != "" && annotationIPs != "auto" {
						if isSingleIP(annotationIPs) {
							ips[annotationIPs] = append(ips[annotationIPs], fmt.Sprintf("%s:%s", workload.Namespace, workload.Name))
							ips[annotationIPs] = unique(ips[annotationIPs])
						} else if isMultipleIP(annotationIPs) {
							workloadIPs := strings.Split(strings.Trim(annotationIPs, " "), "-")
							for _, ip := range workloadIPs {
								ips[ip] = append(ips[ip], fmt.Sprintf("%s:%s", workload.Namespace, workload.Name))
								ips[ip] = unique(ips[ip])
							}
						}
					}
				}
			}
		}
	}

	ss, err := kubeClientset.AppsV1().StatefulSets("").List(context.TODO(), opts)
	if err == nil && len(ss.Items) != 0 {
		for _, workload := range ss.Items {
			if !("StatefulSet" == kind && workload.Namespace == namespace && workload.Name == name) {
				if workload.Spec.Template.Annotations != nil {
					annotationIPs := workload.Spec.Template.Annotations[v1.AnnotationIP]
					if annotationIPs != "" && annotationIPs != "auto" {
						if isSingleIP(annotationIPs) {
							ips[annotationIPs] = append(ips[annotationIPs], fmt.Sprintf("%s:%s", workload.Namespace, workload.Name))
							ips[annotationIPs] = unique(ips[annotationIPs])
						} else if isMultipleIP(annotationIPs) {
							workloadIPs := strings.Split(strings.Trim(annotationIPs, " "), "-")
							for _, ip := range workloadIPs {
								ips[ip] = append(ips[ip], fmt.Sprintf("%s:%s", workload.Namespace, workload.Name))
								ips[ip] = unique(ips[ip])
							}
						}
					}
				}
			}
		}
	}

	cj, err := kubeClientset.BatchV1beta1().CronJobs("").List(context.TODO(), opts)
	if err == nil && len(cj.Items) != 0 {
		for _, workload := range cj.Items {
			if !("CronJob" == kind && workload.Namespace == namespace && workload.Name == name) {
				if workload.Spec.JobTemplate.Spec.Template.Annotations != nil {
					annotationIPs := workload.Spec.JobTemplate.Spec.Template.Annotations[v1.AnnotationIP]
					if annotationIPs != "" && annotationIPs != "auto" {
						if isSingleIP(annotationIPs) {
							ips[annotationIPs] = append(ips[annotationIPs], fmt.Sprintf("%s:%s", workload.Namespace, workload.Name))
							ips[annotationIPs] = unique(ips[annotationIPs])
						} else if isMultipleIP(annotationIPs) {
							workloadIPs := strings.Split(strings.Trim(annotationIPs, " "), "-")
							for _, ip := range workloadIPs {
								ips[ip] = append(ips[ip], fmt.Sprintf("%s:%s", workload.Namespace, workload.Name))
								ips[ip] = unique(ips[ip])
							}
						}
					}
				}
			}
		}
	}

	j, err := kubeClientset.BatchV1().Jobs("").List(context.TODO(), opts)
	if err == nil && len(j.Items) != 0 {
		for _, workload := range j.Items {
			if len(workload.OwnerReferences) != 0 {
				continue
			}
			if !("Job" == kind && workload.Namespace == namespace && workload.Name == name) {
				if workload.Spec.Template.Annotations != nil {
					annotationIPs := workload.Spec.Template.Annotations[v1.AnnotationIP]
					if annotationIPs != "" && annotationIPs != "auto" {
						if isSingleIP(annotationIPs) {
							ips[annotationIPs] = append(ips[annotationIPs], fmt.Sprintf("%s:%s", workload.Namespace, workload.Name))
							ips[annotationIPs] = unique(ips[annotationIPs])
						} else if isMultipleIP(annotationIPs) {
							workloadIPs := strings.Split(strings.Trim(annotationIPs, " "), "-")
							for _, ip := range workloadIPs {
								ips[ip] = append(ips[ip], fmt.Sprintf("%s:%s", workload.Namespace, workload.Name))
								ips[ip] = unique(ips[ip])
							}
						}
					}
				}
			}
		}
	}

	return ips, err
}

func unique(s []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range s {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

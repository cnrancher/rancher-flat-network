package webhook

import (
	"encoding/json"
	"fmt"
	"net"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/common"
	"github.com/cnrancher/rancher-flat-network/pkg/ipcalc"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	nettypes "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
)

type WorkloadReview struct {
	AdmissionReview *admissionv1.AdmissionReview
	ObjectMeta      metav1.ObjectMeta
	Deployment      appsv1.Deployment
	DaemonSet       appsv1.DaemonSet
	StatefulSet     appsv1.StatefulSet
	CronJob         batchv1.CronJob
	Job             batchv1.Job
}

func (r *WorkloadReview) PodTemplateAnnotations(key string) string {
	switch r.AdmissionReview.Request.Kind.Kind {
	case kindDeployment:
		return r.Deployment.Spec.Template.Annotations[key]
	case kindDaemonSet:
		return r.DaemonSet.Spec.Template.Annotations[key]
	case kindStatefulSet:
		return r.StatefulSet.Spec.Template.Annotations[key]
	case kindCronJob:
		return r.CronJob.Spec.JobTemplate.Spec.Template.Annotations[key]
	case kindJob:
		return r.Job.Spec.Template.Annotations[key]
	default:
		return ""
	}
}

func deserializeWorkloadReview(ar *admissionv1.AdmissionReview) (*WorkloadReview, error) {
	var err error
	/* unmarshal workloadss from AdmissionReview request */
	workload := WorkloadReview{
		AdmissionReview: ar,
	}

	switch ar.Request.Kind.Kind {
	case kindDeployment:
		err = json.Unmarshal(ar.Request.Object.Raw, &workload.Deployment)
		workload.ObjectMeta = workload.Deployment.ObjectMeta
	case kindDaemonSet:
		err = json.Unmarshal(ar.Request.Object.Raw, &workload.DaemonSet)
		workload.ObjectMeta = workload.DaemonSet.ObjectMeta
	case kindStatefulSet:
		err = json.Unmarshal(ar.Request.Object.Raw, &workload.StatefulSet)
		workload.ObjectMeta = workload.StatefulSet.ObjectMeta
	case kindCronJob:
		err = json.Unmarshal(ar.Request.Object.Raw, &workload.CronJob)
		workload.ObjectMeta = workload.CronJob.ObjectMeta
	case kindJob:
		err = json.Unmarshal(ar.Request.Object.Raw, &workload.Job)
		workload.ObjectMeta = workload.Job.ObjectMeta
	default:
		return nil, fmt.Errorf("unsupported workload kind %q", ar.Request.Kind.Kind)
	}
	if err != nil {
		err = errors.Wrap(err, "error deserialize workload admission request")
	}

	return &workload, err
}

func (h *Handler) validateWorkload(ar *admissionv1.AdmissionReview) (bool, error) {
	workload, err := deserializeWorkloadReview(ar)
	if err != nil {
		return false, fmt.Errorf("deserializeWorkloadReview: %w", err)
	}
	if workload == nil || workload.ObjectMeta.Name == "" || workload.ObjectMeta.DeletionTimestamp != nil {
		return true, nil
	}
	if workload.PodTemplateAnnotations(nettypes.NetworkAttachmentAnnot) == "" &&
		workload.PodTemplateAnnotations("v1.multus-cni.io/default-network") == "" {
		return true, nil
	}
	subnetName := workload.PodTemplateAnnotations(flv1.AnnotationSubnet)
	if subnetName == "" {
		return true, nil
	}
	if h.isUpdatingWorkloadSubnetLabel(workload) {
		return true, nil
	}
	// Check the ip is available in subnet CIDR and not gateway
	subnet, err := h.subnetClient.Get(
		flv1.SubnetNamespace, subnetName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to get subnet %v: %w", subnetName, err)
	}
	if err := h.validateAnnotationIP(workload, subnet); err != nil {
		return false, fmt.Errorf("validate annotation IP failed: %w", err)
	}
	if err := h.validateAnnotationMac(workload); err != nil {
		return false, fmt.Errorf("validate annotation mac failed: %w", err)
	}
	if err := h.validateIPsInReserved(workload, subnet); err != nil {
		return false, fmt.Errorf("validate IP reserved failed: %w", err)
	}

	flatNetworkIPs, err := h.getWorkloadPodFlatNetworkIPs(workload)
	if err != nil {
		return false, err
	}
	if err := h.validateIPsInUsed(workload, subnet, flatNetworkIPs); err != nil {
		return false, fmt.Errorf("validate IP used failed: %w", err)
	}
	if err := h.validateMACsInUsed(workload, subnet, flatNetworkIPs); err != nil {
		return false, fmt.Errorf("validate MAC used failed: %w", err)
	}

	logrus.Infof("handle workload [%v] validate request [%v/%v]",
		workload.AdmissionReview.Request.Kind.Kind, workload.ObjectMeta.Namespace, workload.ObjectMeta.Name)
	return true, nil
}

func (h *Handler) validateAnnotationIP(
	workload *WorkloadReview, subnet *flv1.FlatNetworkSubnet,
) error {
	// Check annotation IP format.
	ips, err := common.CheckPodAnnotationIPs(workload.PodTemplateAnnotations(flv1.AnnotationIP))
	if err != nil {
		return err
	}
	// IP allocation mode is auto
	if len(ips) == 0 {
		return nil
	}

	// Check the ip is not duplicated
	err = checkIPDuplicate(ips)
	if err != nil {
		return err
	}

	err = checkIPsInSubnet(ips, subnet)
	if err != nil {
		return err
	}

	return nil
}

func checkIPDuplicate(ips []net.IP) error {
	if len(ips) == 0 {
		return nil
	}

	set := map[string]bool{}
	for _, v := range ips {
		if set[v.String()] {
			return fmt.Errorf("ip [%v] is duplicate in list", v)
		}
		set[v.String()] = true
	}
	return nil
}

func checkIPsInSubnet(ips []net.IP, subnet *flv1.FlatNetworkSubnet) error {
	if len(ips) == 0 {
		return nil
	}

	_, network, err := net.ParseCIDR(subnet.Spec.CIDR)
	if err != nil {
		return fmt.Errorf("failed to parse CIDR [%v]: %w",
			subnet.Spec.CIDR, err)
	}
	for _, ip := range ips {
		if ip.Equal(subnet.Spec.Gateway) {
			return fmt.Errorf("ip [%v] is the gateway of subnet %v",
				ip, subnet.Name)
		}
		if !ipcalc.IPInNetwork(ip, network) {
			return fmt.Errorf("ip [%v] is not in subnet CIDR %v",
				ip, subnet.Name)
		}
		if !ipcalc.IsAvailableIP(ip, network) {
			return fmt.Errorf("ip [%v] is not available in subnet CIDR %v",
				ip, subnet.Name)
		}
	}
	return nil
}

func (h *Handler) validateAnnotationMac(workload *WorkloadReview) error {
	ips, err := common.CheckPodAnnotationIPs(workload.PodTemplateAnnotations(flv1.AnnotationIP))
	if err != nil {
		return err
	}
	macs, err := common.CheckPodAnnotationMACs(workload.PodTemplateAnnotations(flv1.AnnotationMac))
	if err != nil {
		return err
	}
	// MAC allocation mode is auto
	if len(macs) == 0 {
		return nil
	}
	// The number of IPs should be equal to MAC addresses if both in specific mode.
	if len(ips) != 0 {
		if len(macs) != len(ips) {
			return fmt.Errorf("pod annotation defines %v MAC addresses but have %v IPs defined, "+
				"the number of MACs and IPs are not same", len(macs), len(ips))
		}
	}

	if err := checkMacDuplicate(macs); err != nil {
		return err
	}
	return nil
}

func checkMacDuplicate(macs []string) error {
	set := map[string]bool{}
	for _, m := range macs {
		if set[m] {
			return fmt.Errorf("mac %v is duplicated", m)
		}
		set[m] = true
	}
	return nil
}

func (h *Handler) validateIPsInReserved(
	workload *WorkloadReview, subnet *flv1.FlatNetworkSubnet,
) error {
	ips, err := common.CheckPodAnnotationIPs(workload.PodTemplateAnnotations(flv1.AnnotationIP))
	if err != nil {
		return err
	}
	if subnet == nil || len(ips) == 0 {
		return nil
	}
	if workload.AdmissionReview.Request.Kind.Kind == "Job" {
		if len(workload.Job.OwnerReferences) != 0 {
			// Skip validate CronJob created Jobs
			return nil
		}
	}
	if len(subnet.Status.ReservedIP) == 0 {
		// Subnet does not have reserved IPs used by workloads.
		return nil
	}
	var w metav1.Object
	switch workload.AdmissionReview.Request.Kind.Kind {
	case kindDeployment:
		w = &workload.Deployment
	case kindDaemonSet:
		w = &workload.DaemonSet
	case kindStatefulSet:
		w = &workload.StatefulSet
	case kindCronJob:
		w = &workload.CronJob
	case kindJob:
		w = &workload.Job
	}
	key := common.GetWorkloadReservdIPKey(w)
	for _, ip := range ips {
		for k, ipRange := range subnet.Status.ReservedIP {
			if k == key {
				continue
			}
			if ipcalc.IPInRanges(ip, ipRange) {
				return fmt.Errorf("IP [%v] already reserved by workload [%v]",
					ip.String(), k)
			}
		}
	}

	return nil
}

func (h *Handler) getWorkloadPodFlatNetworkIPs(
	workload *WorkloadReview,
) ([]flv1.FlatNetworkIP, error) {
	var selector string
	switch workload.AdmissionReview.Request.Kind.Kind {
	case kindDeployment, kindDaemonSet, kindStatefulSet:
		// apps.<kind>-<namespace>-<name>
		selector = fmt.Sprintf("%s=apps.%s-%s-%s",
			flv1.LabelWorkloadSelector,
			strings.ToLower(workload.AdmissionReview.Request.Kind.Kind),
			workload.ObjectMeta.Namespace,
			workload.ObjectMeta.Name)
	case kindCronJob, kindJob:
		// <kind>-<namespace>-<name>
		selector = fmt.Sprintf("%s=%s-%s-%s",
			flv1.LabelWorkloadSelector,
			strings.ToLower(workload.AdmissionReview.Request.Kind.Kind),
			workload.ObjectMeta.Namespace,
			workload.ObjectMeta.Name)
	}

	var ipList *flv1.FlatNetworkIPList
	var err error
	var result = []flv1.FlatNetworkIP{}
	opts := metav1.ListOptions{
		LabelSelector: selector,
		Limit:         50,
		Continue:      "",
	}
	for ipList == nil || opts.Continue != "" {
		ipList, err = h.ipClient.List(workload.ObjectMeta.Namespace, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list FlatNetworkIPs by %q: %w",
				selector, err)
		}
		opts.Continue = ipList.Continue
		if len(ipList.Items) == 0 {
			continue
		}
		result = append(result, ipList.Items...)
	}
	return result, nil
}

func (h *Handler) validateIPsInUsed(
	workload *WorkloadReview,
	subnet *flv1.FlatNetworkSubnet,
	flatnetworkIPs []flv1.FlatNetworkIP,
) error {
	ips, err := common.CheckPodAnnotationIPs(workload.PodTemplateAnnotations(flv1.AnnotationIP))
	if err != nil {
		return err
	}
	if subnet == nil {
		return nil
	}
	if flatnetworkIPs == nil {
		flatnetworkIPs = []flv1.FlatNetworkIP{}
	}
	usedIP := subnet.Status.DeepCopy().UsedIP
	for _, ip := range flatnetworkIPs {
		if len(ip.Status.Addr) == 0 {
			continue
		}
		usedIP = ipcalc.RemoveIPFromRange(ip.Status.Addr, usedIP)
	}
	for _, ip := range ips {
		if ipcalc.IPInRanges(ip, usedIP) {
			return fmt.Errorf("IP %q already uesd by other pods", ip.String())
		}
	}

	return nil
}

func (h *Handler) validateMACsInUsed(
	workload *WorkloadReview,
	subnet *flv1.FlatNetworkSubnet,
	flatnetworkIPs []flv1.FlatNetworkIP,
) error {
	macs, err := common.CheckPodAnnotationMACs(workload.PodTemplateAnnotations(flv1.AnnotationMac))
	if err != nil {
		return err
	}
	if subnet == nil {
		return nil
	}
	usedMAC := subnet.Status.DeepCopy().UsedMAC
	logrus.Infof("XXXX anno macs %v", utils.Print(macs))
	logrus.Infof("XXXX usedMac %v", utils.Print(usedMAC))
	slices.Sort(usedMAC)
	if flatnetworkIPs == nil {
		flatnetworkIPs = []flv1.FlatNetworkIP{}
	}
	for _, ip := range flatnetworkIPs {
		if len(ip.Status.MAC) == 0 {
			continue
		}
		index, ok := slices.BinarySearch(usedMAC, ip.Status.MAC)
		if !ok {
			continue
		}
		logrus.Infof("XXXX index %v", index)
		usedMAC = append(usedMAC[:index], usedMAC[index+1:]...)
	}
	logrus.Infof("XXXX filted usedMac %v", utils.Print(usedMAC))
	for _, mac := range macs {
		if _, ok := slices.BinarySearch(usedMAC, mac); ok {
			return fmt.Errorf("MAC %q already used by other pods", mac)
		}
	}
	return nil
}

func (h *Handler) isUpdatingWorkloadSubnetLabel(workload *WorkloadReview) bool {
	name, namespace := workload.ObjectMeta.Name, workload.ObjectMeta.Namespace
	switch workload.AdmissionReview.Request.Kind.Kind {
	case kindDeployment:
		old, err := h.deploymentClient.Get(namespace, name, metav1.GetOptions{})
		if err != nil {
			return false
		}
		if old.Labels[flv1.LabelFlatNetworkIPType] != workload.Deployment.Labels[flv1.LabelFlatNetworkIPType] ||
			old.Labels[flv1.LabelSubnet] != workload.Deployment.Labels[flv1.LabelSubnet] {
			return true
		}
	case kindDaemonSet:
		old, err := h.daemonSetClient.Get(namespace, name, metav1.GetOptions{})
		if err != nil {
			return false
		}
		if old.Labels[flv1.LabelFlatNetworkIPType] != workload.DaemonSet.Labels[flv1.LabelFlatNetworkIPType] ||
			old.Labels[flv1.LabelSubnet] != workload.DaemonSet.Labels[flv1.LabelSubnet] {
			return true
		}
	case kindStatefulSet:
		old, err := h.statefulSetClient.Get(namespace, name, metav1.GetOptions{})
		if err != nil {
			return false
		}
		if old.Labels[flv1.LabelFlatNetworkIPType] != workload.StatefulSet.Labels[flv1.LabelFlatNetworkIPType] ||
			old.Labels[flv1.LabelSubnet] != workload.StatefulSet.Labels[flv1.LabelSubnet] {
			return true
		}
	case kindCronJob:
		old, err := h.cronJobClient.Get(namespace, name, metav1.GetOptions{})
		if err != nil {
			return false
		}
		if old.Labels[flv1.LabelFlatNetworkIPType] != workload.CronJob.Labels[flv1.LabelFlatNetworkIPType] ||
			old.Labels[flv1.LabelSubnet] != workload.CronJob.Labels[flv1.LabelSubnet] {
			return true
		}
	case kindJob:
		old, err := h.jobClient.Get(namespace, name, metav1.GetOptions{})
		if err != nil {
			return false
		}
		if old.Labels[flv1.LabelFlatNetworkIPType] != workload.Job.Labels[flv1.LabelFlatNetworkIPType] ||
			old.Labels[flv1.LabelSubnet] != workload.Job.Labels[flv1.LabelSubnet] {
			return true
		}
	}
	return false
}

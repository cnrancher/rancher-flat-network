package webhook

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/common"
	"github.com/cnrancher/rancher-flat-network/pkg/ipcalc"
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
	if workload.PodTemplateAnnotations("k8s.v1.cni.cncf.io/networks") == "" &&
		workload.PodTemplateAnnotations("v1.multus-cni.io/default-network") == "" {
		return true, nil
	}
	if workload.PodTemplateAnnotations(flv1.AnnotationSubnet) == "" {
		return true, nil
	}
	if h.isUpdatingWorkloadSubnetLabel(workload) {
		return true, nil
	}
	if err := h.validateAnnotationIP(workload); err != nil {
		return false, fmt.Errorf("validateAnnotationIP: %w", err)
	}
	if err := h.validateAnnotationMac(workload); err != nil {
		return false, fmt.Errorf("validateAnnotationMac: %w", err)
	}

	logrus.Infof("handle workload [%v] validate request [%v/%v]",
		workload.AdmissionReview.Request.Kind.Kind, workload.ObjectMeta.Namespace, workload.ObjectMeta.Name)
	return true, nil
}

func (h *Handler) validateAnnotationIP(workload *WorkloadReview) error {
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
	// Check the ip is available in subnet CIDR and not gateway
	subnet, err := h.subnetClient.Get(
		flv1.SubnetNamespace, workload.PodTemplateAnnotations(flv1.AnnotationSubnet), metav1.GetOptions{})
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

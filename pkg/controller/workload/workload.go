package workload

import (
	"context"
	"fmt"
	"maps"
	"reflect"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/common"
	"github.com/cnrancher/rancher-flat-network/pkg/controller/wrangler"
	appscontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/apps/v1"
	batchcontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/batch/v1"
	flcontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/ipcalc"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
)

// The workload is an apps/v1 Deployment, DaemonSet, StatefulSet
// and batch/v1 Job, CronJob resource.
// NOTE: ReplicaSet is managed by Deployment and is not a Workload
type Workload interface {
	*appsv1.Deployment | *appsv1.DaemonSet | *appsv1.StatefulSet |
		*batchv1.Job | *batchv1.CronJob

	// Workload implements metav1.Object interface
	metav1.Object
}

const (
	handlerName = "rancher-flat-network-workload"
)

type handler struct {
	deploymentClient  appscontroller.DeploymentClient
	daemonsetClient   appscontroller.DaemonSetClient
	statefulsetClient appscontroller.StatefulSetClient
	cronjobClient     batchcontroller.CronJobClient
	jobClient         batchcontroller.JobClient

	subnetClient flcontroller.FlatNetworkSubnetClient
	subnetCache  flcontroller.FlatNetworkSubnetCache
}

var workloadHandler *handler

func Register(
	ctx context.Context,
	wctx *wrangler.Context,
) {
	h := &handler{
		deploymentClient:  wctx.Apps.Deployment(),
		daemonsetClient:   wctx.Apps.DaemonSet(),
		statefulsetClient: wctx.Apps.StatefulSet(),
		cronjobClient:     wctx.Batch.CronJob(),
		jobClient:         wctx.Batch.Job(),
		subnetClient:      wctx.FlatNetwork.FlatNetworkSubnet(),
		subnetCache:       wctx.FlatNetwork.FlatNetworkSubnet().Cache(),
	}
	workloadHandler = h

	wctx.Apps.Deployment().OnChange(ctx, handlerName, syncWorkload)
	wctx.Apps.DaemonSet().OnChange(ctx, handlerName, syncWorkload)
	wctx.Apps.StatefulSet().OnChange(ctx, handlerName, syncWorkload)
	wctx.Batch.CronJob().OnChange(ctx, handlerName, syncWorkload)
	wctx.Batch.Job().OnChange(ctx, handlerName, syncWorkload)
}

func syncWorkload[T Workload](_ string, w T) (T, error) {
	if workloadHandler == nil {
		err := fmt.Errorf("failed to sync workload: handler not initialized")
		logrus.WithFields(fieldsWorkload(w)).Error(err)
		return w, err
	}
	if w == nil || w.GetName() == "" {
		return w, nil
	}
	if w.GetDeletionTimestamp() != nil {
		if err := workloadHandler.removeWorkloadReservedIP(w); err != nil {
			return w, err
		}
		return w, nil
	}

	isFlatNetworkEnabled, labels, err := getTemplateFlatNetworkLabel(w)
	if err != nil {
		return w, fmt.Errorf("getTemplateFlatNetworkLabel: %w", err)
	}
	if !isFlatNetworkEnabled {
		logrus.WithFields(fieldsWorkload(w)).
			Debugf("skip update workload as flat-network not enabled")
		return w, nil
	}
	o, err := workloadHandler.updateWorkloadLabel(w, labels)
	if err != nil {
		logrus.WithFields(fieldsWorkload(w)).
			Errorf("failed to update workload label: %v", err)
		return w, err
	}
	if err := workloadHandler.syncWorkloadReservedIP(w); err != nil {
		err = fmt.Errorf("failed to update subnet reservedIP: %w", err)
		logrus.WithFields(fieldsWorkload(w)).Errorf("%v", err)
		return w, err
	}
	w, _ = o.(T)
	return w, err
}

func getTemplateFlatNetworkLabel(
	w metav1.Object,
) (isFlatNetworkEnabled bool, labels map[string]string, err error) {
	m := GetTemplateObjectMeta(w)
	if m == nil {
		return isFlatNetworkEnabled, labels, nil
	}
	if m.Annotations == nil {
		m.Annotations = map[string]string{}
	}
	a := m.Annotations

	var (
		ipType     string
		subnetName string
	)
	switch a[flv1.AnnotationIP] {
	case flv1.AllocateModeAuto:
		ipType = flv1.AllocateModeAuto
	case "":
	default:
		ipType = flv1.AllocateModeSpecific
	}
	subnetName = a[flv1.AnnotationSubnet]
	isFlatNetworkEnabled = (ipType != "" && subnetName != "")

	labels = map[string]string{
		flv1.LabelFlatNetworkIPType: ipType,
		flv1.LabelSubnet:            subnetName,
	}
	if !isFlatNetworkEnabled {
		return isFlatNetworkEnabled, labels, nil
	}

	subnet, err := workloadHandler.subnetCache.Get(flv1.SubnetNamespace, subnetName)
	if err != nil {
		return isFlatNetworkEnabled, labels, fmt.Errorf(
			"failed to get subnet %q from cache: %w", subnetName, err)
	}
	labels[flv1.LabelFlatMode] = subnet.Spec.FlatMode
	return isFlatNetworkEnabled, labels, nil
}

func (h *handler) updateWorkloadLabel(
	w metav1.Object, labels map[string]string,
) (metav1.Object, error) {
	if labels == nil {
		return w, nil
	}

	wCopy := DeepCopy(w)
	if wCopy == nil {
		logrus.WithFields(fieldsWorkload(w)).
			Warnf("updateWorkloadLabel: skip unrecognized workload: %T", w)
		return w, nil
	}
	w = wCopy
	wl := w.GetLabels()
	if wl == nil {
		wl = map[string]string{}
	}
	needUpdate := false
	for k, v := range labels {
		if wl[k] != v {
			needUpdate = true
			wl[k] = v
		}
	}
	if !needUpdate {
		return w, nil
	}
	logrus.WithFields(fieldsWorkload(w)).
		Infof("request to update workload [%v/%v] label: %v",
			w.GetNamespace(), w.GetName(), utils.Print(labels))

	switch o := w.(type) {
	case *appsv1.Deployment:
		return h.deploymentClient.Update(o)
	case *appsv1.DaemonSet:
		return h.daemonsetClient.Update(o)
	case *appsv1.StatefulSet:
		return h.statefulsetClient.Update(o)
	case *batchv1.CronJob:
		return h.cronjobClient.Update(o)
	case *batchv1.Job:
		return h.jobClient.Update(o)
	}
	return w, nil
}

func (h *handler) removeWorkloadReservedIP(w metav1.Object) error {
	m := GetTemplateObjectMeta(w)
	if m == nil {
		return nil
	}
	annotationIP := m.Annotations[flv1.AnnotationIP]
	subnetName := m.Annotations[flv1.AnnotationSubnet]
	ips, err := common.CheckPodAnnotationIPs(annotationIP)
	if err != nil {
		return err
	}
	if len(ips) == 0 || subnetName == "" {
		return nil
	}
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		subnet, err := h.subnetCache.Get(flv1.SubnetNamespace, subnetName)
		if err != nil {
			return fmt.Errorf("failed to get subnet %v from cache: %w",
				subnetName, err)
		}
		key := common.GetWorkloadReservdIPKey(w)
		if key == "" {
			return nil
		}
		reservedIP := maps.Clone(subnet.Status.ReservedIP)
		if len(reservedIP) == 0 {
			return nil
		}
		delete(reservedIP, key)
		if reflect.DeepEqual(subnet.Status.ReservedIP, reservedIP) {
			// already updated, skip
			return nil
		}
		subnet = subnet.DeepCopy()
		subnet.Status.ReservedIP = reservedIP
		_, err = h.subnetClient.UpdateStatus(subnet)
		if err != nil {
			return err
		}
		logrus.WithFields(fieldsWorkload(w)).
			Infof("remove subnet workload reservd IP as workload deleted")
		return nil
	})
	if err != nil {
		logrus.WithFields(fieldsWorkload(w)).
			Errorf("failed to remove subnet workload reserved IP: %v", err)
		return err
	}
	return nil
}

func (h *handler) syncWorkloadReservedIP(w metav1.Object) error {
	m := GetTemplateObjectMeta(w)
	if m == nil {
		return nil
	}
	annotationIP := m.Annotations[flv1.AnnotationIP]
	subnetName := m.Annotations[flv1.AnnotationSubnet]
	ips, err := common.CheckPodAnnotationIPs(annotationIP)
	if err != nil {
		return err
	}
	if len(ips) == 0 || subnetName == "" {
		return nil
	}
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		subnet, err := h.subnetCache.Get(flv1.SubnetNamespace, subnetName)
		if err != nil {
			return fmt.Errorf("failed to get subnet %v from cache: %w",
				subnetName, err)
		}
		key := common.GetWorkloadReservdIPKey(w)
		if key == "" {
			return nil
		}
		reservedIP := maps.Clone(subnet.Status.ReservedIP)
		if reservedIP == nil {
			reservedIP = make(map[string][]flv1.IPRange)
		}
		ipRange := []flv1.IPRange{}
		for _, ip := range ips {
			ipRange = ipcalc.AddIPToRange(ip, ipRange)
		}
		reservedIP[key] = ipRange
		if reflect.DeepEqual(subnet.Status.ReservedIP, reservedIP) {
			// already updated, skip
			return nil
		}
		subnet = subnet.DeepCopy()
		subnet.Status.ReservedIP = reservedIP
		_, err = h.subnetClient.UpdateStatus(subnet)
		if err != nil {
			return err
		}
		logrus.WithFields(fieldsWorkload(w)).
			Infof("update subnet workload reservd IP to %v",
				utils.Print(ipRange))
		return nil
	})
	if err != nil {
		logrus.WithFields(fieldsWorkload(w)).
			Errorf("failed to update subnet workload reserved IP: %v", err)
		return err
	}
	return nil
}

func fieldsWorkload(obj metav1.Object) logrus.Fields {
	if obj == nil {
		return logrus.Fields{}
	}
	fields := logrus.Fields{
		"GID": utils.GID(),
	}
	s := fmt.Sprintf("%v/%v", obj.GetNamespace(), obj.GetName())
	switch obj.(type) {
	case *appsv1.Deployment:
		fields["Deployment"] = s
	case *appsv1.DaemonSet:
		fields["DaemonSet"] = s
	case *appsv1.StatefulSet:
		fields["StatefulSet"] = s
	case *batchv1.Job:
		fields["Job"] = s
	}
	return fields
}

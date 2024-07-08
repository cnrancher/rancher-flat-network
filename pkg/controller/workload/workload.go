package workload

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flv1 "github.com/cnrancher/rancher-flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/controller/wrangler"
	appscontroller "github.com/cnrancher/rancher-flat-network-operator/pkg/generated/controllers/apps/v1"
	batchcontroller "github.com/cnrancher/rancher-flat-network-operator/pkg/generated/controllers/batch/v1"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/utils"
)

type Workload interface {
	*appsv1.Deployment | *appsv1.DaemonSet | *appsv1.ReplicaSet |
		*appsv1.StatefulSet | *batchv1.Job | *batchv1.CronJob

	// Workload implements metav1.Object interface
	metav1.Object
}

const (
	handlerName = "rancher-flat-network-workload"
)

type handler struct {
	deploymentClient  appscontroller.DeploymentClient
	daemonsetClient   appscontroller.DaemonSetClient
	replicasetClient  appscontroller.ReplicaSetClient
	statefulsetClient appscontroller.StatefulSetClient
	cronjobClient     batchcontroller.CronJobClient
	jobClient         batchcontroller.JobClient
}

var workloadHandler *handler

func Register(
	ctx context.Context,
	wctx *wrangler.Context,
) {
	h := &handler{
		deploymentClient:  wctx.Apps.Deployment(),
		daemonsetClient:   wctx.Apps.DaemonSet(),
		replicasetClient:  wctx.Apps.ReplicaSet(),
		statefulsetClient: wctx.Apps.StatefulSet(),
	}
	workloadHandler = h

	wctx.Apps.Deployment().OnChange(ctx, handlerName, syncWorkload)
	wctx.Apps.DaemonSet().OnChange(ctx, handlerName, syncWorkload)
	wctx.Apps.ReplicaSet().OnChange(ctx, handlerName, syncWorkload)
	wctx.Apps.StatefulSet().OnChange(ctx, handlerName, syncWorkload)
}

func syncWorkload[T Workload](_ string, w T) (T, error) {
	if workloadHandler == nil {
		err := fmt.Errorf("failed to sync workload: handler not initialized")
		logrus.WithFields(fieldsWorkload(w)).Error(err)
		return w, err
	}
	if w == nil || w.GetName() == "" || w.GetDeletionTimestamp() != nil {
		return w, nil
	}

	isFlatNetworkEnabled, labels := getFlatNetworkLabel(w)
	if !isFlatNetworkEnabled {
		return w, nil
	}
	o, err := workloadHandler.updateWorkloadLabel(w, labels)
	if err != nil {
		logrus.WithFields(fieldsWorkload(w)).
			Errorf("failed to update workload label: %v", err)
		return w, err
	}
	w, _ = o.(T)
	return w, err
}

func getFlatNetworkLabel(w metav1.Object) (isFlatNetworkEnabled bool, labels map[string]string) {
	m := getTemplateObjectMeta(w)
	if m == nil {
		return false, nil
	}
	if m.Annotations == nil {
		m.Annotations = map[string]string{}
	}
	a := m.Annotations

	var (
		ipType string
		subnet string
	)
	switch a[flv1.LabelFlatNetworkIPType] {
	case flv1.AllocateModeAuto:
		ipType = flv1.AllocateModeAuto
		isFlatNetworkEnabled = true
	case "":
	default:
		ipType = flv1.AllocateModeSpecific
		isFlatNetworkEnabled = true
	}
	subnet = a[flv1.AnnotationSubnet]

	labels = map[string]string{
		flv1.LabelFlatNetworkIPType: ipType,
		flv1.LabelSubnet:            subnet,
	}
	return
}

func (h *handler) updateWorkloadLabel(
	w metav1.Object, labels map[string]string,
) (metav1.Object, error) {
	if labels == nil {
		return w, nil
	}

	wCopy := deepCopy(w)
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
	case *appsv1.ReplicaSet:
		return h.replicasetClient.Update(o)
	case *batchv1.CronJob:
		return h.cronjobClient.Update(o)
	case *batchv1.Job:
		return h.jobClient.Update(o)
	}
	return w, nil
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
	case *appsv1.ReplicaSet:
		fields["ReplicaSet"] = s
	case *batchv1.Job:
		fields["Job"] = s
	}
	return fields
}

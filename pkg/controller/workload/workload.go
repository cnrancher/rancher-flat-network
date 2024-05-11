package workload

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	appscontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/apps/v1"
	batchcontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/batch/v1"
)

type Workload interface {
	*appsv1.Deployment | *appsv1.DaemonSet | *appsv1.ReplicaSet |
		*appsv1.StatefulSet | *batchv1.Job | *batchv1.CronJob

	// Workload implements metav1.Object interface
	metav1.Object
}

const (
	controllerName       = "workload"
	controllerRemoveName = "workload-remove"
)

type handler struct {
	deployments  appscontroller.DeploymentClient
	daemonsets   appscontroller.DaemonSetClient
	replicasets  appscontroller.ReplicaSetClient
	statefulsets appscontroller.StatefulSetClient
	cronjobs     batchcontroller.CronJobClient
	jobs         batchcontroller.JobClient

	deploymentEnqueueAfter func(string, string, time.Duration)
	deploymentEnqueue      func(string, string)

	daemonsetEnqueueAfter func(string, string, time.Duration)
	daemonsetEnqueue      func(string, string)

	replicasetEnqueueAfter func(string, string, time.Duration)
	replicasetEnqueue      func(string, string)

	statefulsetEnqueueAfter func(string, string, time.Duration)
	statefulsetEnqueue      func(string, string)
}

var workloadHandler *handler = nil

func Register(
	ctx context.Context,
	deployments appscontroller.DeploymentController,
	daemonsets appscontroller.DaemonSetController,
	replicasets appscontroller.ReplicaSetController,
	statefulsets appscontroller.StatefulSetController,
) {
	h := &handler{
		deployments:  deployments,
		daemonsets:   daemonsets,
		replicasets:  replicasets,
		statefulsets: statefulsets,

		deploymentEnqueueAfter:  deployments.EnqueueAfter,
		deploymentEnqueue:       daemonsets.Enqueue,
		daemonsetEnqueueAfter:   daemonsets.EnqueueAfter,
		daemonsetEnqueue:        daemonsets.Enqueue,
		replicasetEnqueueAfter:  replicasets.EnqueueAfter,
		replicasetEnqueue:       replicasets.Enqueue,
		statefulsetEnqueueAfter: statefulsets.EnqueueAfter,
		statefulsetEnqueue:      statefulsets.Enqueue,
	}
	workloadHandler = h

	logrus.Infof("Setting up Deployment/DaemonSet/ReplicaSet/StatefulSet event handlers")
	deployments.OnChange(ctx, controllerName, syncWorkload)
	daemonsets.OnChange(ctx, controllerName, syncWorkload)
	replicasets.OnChange(ctx, controllerName, syncWorkload)
	statefulsets.OnChange(ctx, controllerName, syncWorkload)
}

func syncWorkload[T Workload](name string, w T) (T, error) {
	update, iptype, subnet := needUpdateWorkloadLabel(w)
	if !update {
		return w, nil
	}
	o, err := workloadHandler.updateWorkloadLabel(w, iptype, subnet)
	w, _ = o.(T)
	return w, err
}

func needUpdateWorkloadLabel(
	workload metav1.Object,
) (bool, string, string) {
	workloadLabels := workload.GetLabels()
	if workloadLabels == nil {
		workloadLabels = make(map[string]string)
	}
	podMeta := getTemplateObjectMeta(workload)

	update := false
	iptype := workloadLabels[macvlanv1.LabelMacvlanIPType]
	subnet := podMeta.Annotations[macvlanv1.AnnotationSubnet]
	if podMeta.Annotations[macvlanv1.AnnotationSubnet] != workloadLabels[macvlanv1.LabelSubnet] {
		update = true
		subnet = podMeta.Annotations[macvlanv1.AnnotationSubnet]
	}
	if podMeta.Annotations[macvlanv1.AnnotationIP] == "auto" {
		if workloadLabels[macvlanv1.LabelMacvlanIPType] != "auto" {
			update = true
			iptype = "auto"
		}
	}
	if podMeta.Annotations[macvlanv1.AnnotationIP] != "auto" &&
		podMeta.Annotations[macvlanv1.AnnotationIP] != "" &&
		workloadLabels[macvlanv1.LabelMacvlanIPType] != "specific" {
		update = true
		iptype = "specific"
	}
	return update, iptype, subnet
}

func (h *handler) updateWorkloadLabel(w metav1.Object, iptype, subnet string) (metav1.Object, error) {
	wCopy := deepCopy(w)
	if w == nil {
		logrus.Warnf("updateWorkloadLabel: skip unrecognized workload: %T", w)
		return w, nil
	}
	w = wCopy

	logrus.Infof("Update workload %T [%s/%s] label [macvlanIpType: %v] [subnet: %v]",
		w, w.GetNamespace(), w.GetName(), iptype, subnet)
	labels := w.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[macvlanv1.LabelMacvlanIPType] = iptype
	labels[macvlanv1.LabelSubnet] = subnet
	switch o := w.(type) {
	case *appsv1.Deployment:
		return h.deployments.Update(o)
	case *appsv1.DaemonSet:
		return h.daemonsets.Update(o)
	case *appsv1.StatefulSet:
		return h.statefulsets.Update(o)
	case *appsv1.ReplicaSet:
		return h.replicasets.Update(o)
	case *batchv1.CronJob:
		return h.cronjobs.Update(o)
	case *batchv1.Job:
		return h.jobs.Update(o)
	}
	return w, nil
}

package controller

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	macvlanv1 "github.com/cnrancher/macvlan-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	appscontroller "github.com/cnrancher/macvlan-operator/pkg/generated/controllers/apps/v1"
	batchcontroller "github.com/cnrancher/macvlan-operator/pkg/generated/controllers/batch/v1"
	corecontroller "github.com/cnrancher/macvlan-operator/pkg/generated/controllers/core/v1"
	macvlancontroller "github.com/cnrancher/macvlan-operator/pkg/generated/controllers/macvlan.cluster.cattle.io/v1"
)

const (
	controllerName       = "macvlan-operator"
	controllerRemoveName = "macvlan-operator-remove"
)

type Handler struct {
	macvlanIPs     macvlancontroller.MacvlanIPClient
	macvlanSubnets macvlancontroller.MacvlanSubnetClient
	pods           corecontroller.PodClient
	services       corecontroller.ServiceClient
	namespaces     corecontroller.NamespaceClient
	deployments    appscontroller.DeploymentClient
	daemonsets     appscontroller.DaemonSetClient
	replicasets    appscontroller.ReplicaSetClient
	statefulsets   appscontroller.StatefulSetClient
	cronjobs       batchcontroller.CronJobClient
	jobs           batchcontroller.JobClient

	ipEnqueueAfter     func(namespace, name string, duration time.Duration)
	ipEnqueue          func(namespace, name string)
	subnetEnqueueAfter func(namespace, name string, duration time.Duration)
	subnetEnqueue      func(namespace, name string)
	podEnqueueAfter    func(namespace, name string, duration time.Duration)
	podEnqueue         func(namespace, name string)

	inUsedIPs        *sync.Map
	inUsedMacForAuto *sync.Map
	inUsedFixedIPs   *sync.Map
	mux              *sync.Mutex
}

type RegisterOpts struct {
	MacvlanIPs     macvlancontroller.MacvlanIPController
	MacvlanSubnets macvlancontroller.MacvlanSubnetController
	Pods           corecontroller.PodController
	Services       corecontroller.ServiceController
	Namespaces     corecontroller.NamespaceController
	Deployments    appscontroller.DeploymentController
	Daemonsets     appscontroller.DaemonSetController
	Replicasets    appscontroller.ReplicaSetController
	Statefulsets   appscontroller.StatefulSetController
	Cronjobs       batchcontroller.CronJobController
	Jobs           batchcontroller.JobController
}

func Register(
	ctx context.Context,
	opts *RegisterOpts,
) {
	h := &Handler{
		macvlanIPs:     opts.MacvlanIPs,
		macvlanSubnets: opts.MacvlanSubnets,
		pods:           opts.Pods,
		services:       opts.Services,
		namespaces:     opts.Namespaces,
		deployments:    opts.Deployments,
		daemonsets:     opts.Daemonsets,
		replicasets:    opts.Replicasets,
		statefulsets:   opts.Statefulsets,
		cronjobs:       opts.Cronjobs,
		jobs:           opts.Jobs,

		ipEnqueueAfter:     opts.MacvlanIPs.EnqueueAfter,
		ipEnqueue:          opts.MacvlanIPs.Enqueue,
		subnetEnqueueAfter: opts.MacvlanSubnets.EnqueueAfter,
		subnetEnqueue:      opts.MacvlanSubnets.Enqueue,
		podEnqueueAfter:    opts.Pods.EnqueueAfter,
		podEnqueue:         opts.Pods.Enqueue,

		inUsedIPs:        &sync.Map{},
		inUsedMacForAuto: &sync.Map{},
		inUsedFixedIPs:   &sync.Map{},
		mux:              &sync.Mutex{},
	}

	// Register handlers
	logrus.Info("Setting up event handlers")

	opts.MacvlanIPs.OnChange(ctx, controllerName, func(s string, mi *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
		if mi == nil {
			return nil, nil
		}
		logrus.Infof("XXXX macvlanip change: %v", mi.Name)
		return mi, nil
	})
	opts.MacvlanIPs.OnRemove(ctx, controllerName, func(s string, mi *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
		if mi == nil {
			return nil, nil
		}
		logrus.Infof("XXXX macvlanip remove: %v", mi.Name)
		return mi, nil
	})

	opts.MacvlanSubnets.OnChange(ctx, controllerName, tmp_subnets)
	opts.MacvlanSubnets.OnRemove(ctx, controllerName, tmp_subnets)

	opts.Pods.OnChange(ctx, controllerName, h.onPodUpdate)
	opts.Pods.OnRemove(ctx, controllerName, h.onPodRemove)

	opts.Deployments.OnChange(ctx, controllerName, tmp_deployments)
	opts.Deployments.OnRemove(ctx, controllerName, tmp_deployments)

	opts.Statefulsets.OnChange(ctx, controllerName, tmp_statefulsets)
	opts.Statefulsets.OnRemove(ctx, controllerName, tmp_statefulsets)

	opts.Cronjobs.OnChange(ctx, controllerName, tmp_cronJobs)
	opts.Cronjobs.OnRemove(ctx, controllerName, tmp_cronJobs)
}

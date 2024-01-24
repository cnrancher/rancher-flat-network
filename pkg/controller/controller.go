package controller

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	appscontroller "github.com/cnrancher/macvlan-operator/pkg/generated/controllers/apps/v1"
	batchcontroller "github.com/cnrancher/macvlan-operator/pkg/generated/controllers/batch/v1"
	corecontroller "github.com/cnrancher/macvlan-operator/pkg/generated/controllers/core/v1"
	macvlancontroller "github.com/cnrancher/macvlan-operator/pkg/generated/controllers/macvlan.cluster.cattle.io/v1"
)

const (
	controllerName       = "macvlan-operator"
	controllerRemoveName = "macvlan-operator-remove"
)

const (
	eventMacvlanSubnetError = "MacvlanSubnetError"
	eventMacvlanIPError     = "MacvlanIPError"
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

	macvlanipEnqueueAfter     func(namespace, name string, duration time.Duration)
	macvlanipEnqueue          func(namespace, name string)
	macvlansubnetEnqueueAfter func(namespace, name string, duration time.Duration)
	macvlansubnetEnqueue      func(namespace, name string)
	podEnqueueAfter           func(namespace, name string, duration time.Duration)
	podEnqueue                func(namespace, name string)

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

		macvlanipEnqueueAfter:     opts.MacvlanIPs.EnqueueAfter,
		macvlanipEnqueue:          opts.MacvlanIPs.Enqueue,
		macvlansubnetEnqueueAfter: opts.MacvlanSubnets.EnqueueAfter,
		macvlansubnetEnqueue:      opts.MacvlanSubnets.Enqueue,
		podEnqueueAfter:           opts.Pods.EnqueueAfter,
		podEnqueue:                opts.Pods.Enqueue,

		inUsedIPs:        &sync.Map{},
		inUsedMacForAuto: &sync.Map{},
		inUsedFixedIPs:   &sync.Map{},
		mux:              &sync.Mutex{},
	}

	// Register handlers.
	logrus.Info("Setting up event handlers")
	opts.MacvlanIPs.OnChange(ctx, controllerName, h.handleMacvlanIPError(h.onMacvlanIPChanged))
	opts.MacvlanIPs.OnRemove(ctx, controllerRemoveName, h.onMacvlanIPRemoved)

	opts.MacvlanSubnets.OnChange(ctx, controllerName, h.handleMacvlanSubnetError(h.onMacvlanSubnetChanged))
	opts.MacvlanSubnets.OnRemove(ctx, controllerRemoveName, h.onMacvlanSubnetRemove)

	opts.Pods.OnChange(ctx, controllerName, h.handlePodError(h.onPodChanged))
	opts.Pods.OnRemove(ctx, controllerRemoveName, h.onPodRemoved)

	opts.Services.OnChange(ctx, controllerName, h.handleServiceError(h.onServiceChanged))
	opts.Services.OnRemove(ctx, controllerRemoveName, h.onServiceRemoved)

	opts.Namespaces.OnChange(ctx, controllerName, h.handleNamespaceError(h.onNamespaceChanged))
	opts.Namespaces.OnRemove(ctx, controllerRemoveName, h.onNamespaceRemoved)

	opts.Deployments.OnChange(ctx, controllerName, h.handleDeploymentError(h.onDeploymentChanged))
	opts.Deployments.OnRemove(ctx, controllerRemoveName, h.onDeploymentRemoved)

	opts.Statefulsets.OnChange(ctx, controllerName, h.handleStatefulSetError(h.onStatefulSetChanged))
	opts.Statefulsets.OnRemove(ctx, controllerRemoveName, h.onStatefulSetRemoved)

	opts.Cronjobs.OnChange(ctx, controllerName, h.handleCronJobError(h.onCronJobChanged))
	opts.Cronjobs.OnRemove(ctx, controllerRemoveName, h.onCronJobRemoved)

	opts.Jobs.OnChange(ctx, controllerName, h.handleJobsError(h.onJobsChanged))
	opts.Jobs.OnRemove(ctx, controllerRemoveName, h.onJobsRemoved)
}

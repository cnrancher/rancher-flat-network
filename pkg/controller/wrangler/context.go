package wrangler

import (
	"context"

	"github.com/cnrancher/flat-network-operator/pkg/generated/controllers/apps"
	appsv1 "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/apps/v1"
	"github.com/cnrancher/flat-network-operator/pkg/generated/controllers/batch"
	batchv1 "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/batch/v1"
	"github.com/cnrancher/flat-network-operator/pkg/generated/controllers/core"
	corev1 "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/core/v1"
	macvlan "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/macvlan.cluster.cattle.io"
	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/macvlan.cluster.cattle.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/generated/controllers/networking.k8s.io"
	networkingv1 "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/networking.k8s.io/v1"
	"github.com/rancher/wrangler/v2/pkg/start"
	"k8s.io/client-go/rest"
)

type Context struct {
	RESTConfig *rest.Config

	Macvlan    macvlanv1.Interface
	Core       corev1.Interface
	Apps       appsv1.Interface
	Networking networkingv1.Interface
	Batch      batchv1.Interface

	starters []start.Starter
}

func NewContext(cfg *rest.Config) (*Context, error) {
	// panic on error
	macvlan := macvlan.NewFactoryFromConfigOrDie(cfg)
	core := core.NewFactoryFromConfigOrDie(cfg)
	apps := apps.NewFactoryFromConfigOrDie(cfg)
	networking := networking.NewFactoryFromConfigOrDie(cfg)
	batch := batch.NewFactoryFromConfigOrDie(cfg)

	c := &Context{
		RESTConfig: cfg,
		Macvlan:    macvlan.Macvlan().V1(),
		Core:       core.Core().V1(),
		Apps:       apps.Apps().V1(),
		Networking: networking.Networking().V1(),
		Batch:      batch.Batch().V1(),
	}
	c.starters = append(c.starters, macvlan, core, apps, networking, batch)
	return c, nil
}

func (c *Context) Start(ctx context.Context, worker int) error {
	return start.All(ctx, worker, c.starters...)
}

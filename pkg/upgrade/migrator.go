package upgrade

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cnrancher/rancher-flat-network/pkg/controller/wrangler"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

type Migrator interface {
	Run(context.Context) error
}

type migrator struct {
	wctx             *wrangler.Context
	dynamicClientSet *dynamic.DynamicClient
	workloadKinds    []string
	interval         time.Duration
	listLimit        int64
	autoYes          bool
}

var _ Migrator = &migrator{}

type MigratorOpts struct {
	Config        *rest.Config
	WorkloadKinds string
	Interval      time.Duration
	ListLimit     int64
	AutoYes       bool
}

func NewResourceMigrator(
	opts *MigratorOpts,
) Migrator {
	wctx := wrangler.NewContextOrDie(opts.Config)
	dc := dynamic.NewForConfigOrDie(opts.Config)
	spec := strings.Split(strings.TrimSpace(opts.WorkloadKinds), ",")
	kinds := []string{}
	for _, s := range spec {
		if s != "" {
			kinds = append(kinds, s)
		}
	}
	limit := opts.ListLimit
	if limit < 0 {
		limit = 100
	}

	return &migrator{
		wctx:             wctx,
		dynamicClientSet: dc,
		workloadKinds:    kinds,
		interval:         opts.Interval,
		listLimit:        limit,
		autoYes:          opts.AutoYes,
	}
}

func (m *migrator) Run(ctx context.Context) error {
	if err := m.migrateSubnet(ctx); err != nil {
		return fmt.Errorf("failed to migrate subnet resource: %w", err)
	}
	if len(m.workloadKinds) == 0 {
		logrus.Infof("skip migrate workloads as no workload kinds specified")
		return nil
	}
	for _, v := range m.workloadKinds {
		if err := m.migrateWorkload(ctx, v); err != nil {
			return fmt.Errorf("failed to migrate %v: %w",
				v, err)
		}
	}
	return nil
}

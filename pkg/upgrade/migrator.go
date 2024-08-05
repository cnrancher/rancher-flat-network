package upgrade

import (
	"context"
	"fmt"
	"strings"

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
}

var _ Migrator = &migrator{}

func NewResourceMigrator(cfg *rest.Config, workloadKinds string) Migrator {
	wctx := wrangler.NewContextOrDie(cfg)
	dc := dynamic.NewForConfigOrDie(cfg)
	spec := strings.Split(strings.TrimSpace(workloadKinds), ",")
	kinds := []string{}
	for _, s := range spec {
		if s != "" {
			kinds = append(kinds, s)
		}
	}

	return &migrator{
		wctx:             wctx,
		dynamicClientSet: dc,
		workloadKinds:    kinds,
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
		if err := m.migrateWorkload(v); err != nil {
			return fmt.Errorf("failed to migrate %v: %w",
				v, err)
		}
	}
	return nil
}

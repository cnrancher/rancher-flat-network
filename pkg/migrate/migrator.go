package migrate

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cnrancher/rancher-flat-network/pkg/controller/wrangler"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

type Migrator interface {
	Run(context.Context) error
	BackupV1(context.Context, io.Writer) error
	BackupV2(context.Context, io.Writer) error
	Clean(context.Context) error
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
	if err := m.migrateSubnets(ctx); err != nil {
		return fmt.Errorf("failed to migrate subnet resource: %w", err)
	}
	if len(m.workloadKinds) == 0 {
		logrus.Infof("Skip migrate workloads as no workload kinds specified")
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

func (m *migrator) BackupV1(ctx context.Context, w io.Writer) error {
	logrus.Infof("Start backup %q", v1SubnetCRD)
	var objs = []metav1.Object{}
	crd, err := m.getV1SubnetCRD(ctx)
	if err != nil {
		return fmt.Errorf("failed to get %q CRD: %w", v1SubnetCRD, err)
	}
	if crd != nil {
		objs = append(objs, crd)
	}
	subnets, err := m.listV1Subnet(ctx)
	if err != nil {
		return fmt.Errorf("failed to list %v: %w", v1SubnetCRD, err)
	}
	objs = append(objs, subnets...)
	return saveYAML(objs, w)
}

func (m *migrator) BackupV2(ctx context.Context, w io.Writer) error {
	logrus.Infof("Start backup 'flatnetworksubnets.flatnetwork.pandaria.io'")
	var objs = []metav1.Object{}
	crd, err := m.getV2SubnetCRD(ctx)
	if err != nil {
		return fmt.Errorf("failed to get %q CRD: %w", v2SubnetCRD, err)
	}
	if crd != nil {
		objs = append(objs, crd)
	}
	subnets, err := m.listV2Subnet(ctx)
	if err != nil {
		return fmt.Errorf("failed to list flatnetworksubnets.flatnetwork.pandaria.io: %w", err)
	}
	objs = append(objs, subnets...)
	return saveYAML(objs, w)
}

func saveYAML(
	objs []metav1.Object, w io.Writer,
) error {
	if len(objs) == 0 {
		return nil
	}

	for _, o := range objs {
		b, err := yaml.Marshal(o)
		if err != nil {
			return fmt.Errorf("failed to marshal yaml: %w", err)
		}
		if _, err = w.Write(b); err != nil {
			return fmt.Errorf("failed to write data to file: %w", err)
		}
		if _, err = w.Write([]byte("---\n")); err != nil {
			return fmt.Errorf("failed to write data to file: %w", err)
		}
		logrus.Infof("Backup [%v/%v]", o.GetNamespace(), o.GetName())
	}
	return nil
}

func (m *migrator) Clean(ctx context.Context) error {
	return nil
}

package migrate

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/controller/wrangler"
	"github.com/cnrancher/rancher-flat-network/pkg/migrate/types"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

type Migrator interface {
	Run(context.Context) error
	BackupV1(context.Context, io.Writer) error
	BackupV2(context.Context, io.Writer) error
	BackupV1Service(context.Context, io.Writer) error
	Restore(context.Context, string) error
	Clean(context.Context) error
}

type migrator struct {
	wctx             *wrangler.Context
	dynamicClientSet *dynamic.DynamicClient
	workloadKinds    []string
	migrateServices  bool
	interval         time.Duration
	listLimit        int64
	autoYes          bool
}

var _ Migrator = &migrator{}

type MigratorOpts struct {
	Config          *rest.Config
	WorkloadKinds   string
	MigrateServices bool
	Interval        time.Duration
	ListLimit       int64
	AutoYes         bool
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
		migrateServices:  opts.MigrateServices,
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

	if m.migrateServices {
		if err := m.migrateService(ctx); err != nil {
			return fmt.Errorf("failed to migrate service: %w", err)
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
	subnets, err := m.listV1Resources(ctx, macvlanSubnetResource())
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
	ns, err := m.getV2Namespace(ctx)
	if err != nil {
		return fmt.Errorf("failed to get %q CRD: %w", v2SubnetCRD, err)
	}
	if ns != nil {
		objs = append(objs, ns)
	}
	subnets, err := m.listV2Subnets(ctx)
	if err != nil {
		return fmt.Errorf("failed to list flatnetworksubnets.flatnetwork.pandaria.io: %w", err)
	}
	objs = append(objs, subnets...)
	return saveYAML(objs, w)
}

func (m *migrator) BackupV1Service(ctx context.Context, w io.Writer) error {
	logrus.Infof("Start backup macvlan V1 services")
	var objs = []metav1.Object{}
	services, err := m.listV1Service(ctx)
	if err != nil {
		return fmt.Errorf("failed to list corev1.Service: %w", err)
	}
	objs = append(objs, services...)
	return saveYAML(objs, w)
}

func saveYAML(
	objs []metav1.Object, w io.Writer,
) error {
	if len(objs) == 0 {
		return nil
	}

	for _, o := range objs {
		o.SetCreationTimestamp(metav1.Time{})
		o.SetResourceVersion("")
		o.SetGeneration(0)
		o.SetUID("")
		o.SetManagedFields(nil)
		a := o.GetAnnotations()
		if a == nil {
			a = make(map[string]string)
		}
		a["kubectl.kubernetes.io/last-applied-configuration"] = ""
		o.SetAnnotations(a)

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
		if o.GetNamespace() != "" {
			logrus.Infof("Backup [%v/%v]", o.GetNamespace(), o.GetName())
		} else {
			logrus.Infof("Backup [%v]", o.GetName())
		}
	}
	return nil
}

func (m *migrator) Restore(ctx context.Context, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to open %q: %w", filePath, err)
	}
	spec := strings.Split(string(data), "\n---")
	if len(spec) == 0 {
		logrus.Infof("no data, skip")
		return nil
	}
	for _, s := range spec {
		if s == "" || s == "\n" {
			continue
		}
		o := unstructured.Unstructured{}
		if err := yaml.Unmarshal([]byte(s), &o); err != nil {
			return fmt.Errorf("failed to decode yaml: %w", err)
		}
		var err error
		switch o.GetKind() {
		case "Namespace":
			ns := corev1.Namespace{}
			err = yaml.Unmarshal([]byte(s), &ns)
			if err != nil {
				return fmt.Errorf("failed to unmarshal %v %v: %w", o.GetKind(), o.GetName(), err)
			}
			_, err = m.wctx.Core.Namespace().Create(&ns)
		case "CustomResourceDefinition":
			_, err = m.dynamicClientSet.Resource(crdResource()).Create(
				ctx, &o, metav1.CreateOptions{})
		case "FlatNetworkSubnet":
			subnet := flv1.FlatNetworkSubnet{}
			err = yaml.Unmarshal([]byte(s), &subnet)
			if err != nil {
				return fmt.Errorf("failed to unmarshal %v %v: %w", o.GetKind(), o.GetName(), err)
			}
			_, err = m.wctx.FlatNetwork.FlatNetworkSubnet().Create(&subnet)
		case "MacvlanSubnet":
			_, err = m.dynamicClientSet.Resource(macvlanSubnetResource()).
				Namespace(o.GetNamespace()).Create(ctx, &o, metav1.CreateOptions{})
		case "Service":
			svc := corev1.Service{}
			err = yaml.Unmarshal([]byte(s), &svc)
			if err != nil {
				return fmt.Errorf("failed to unmarshal %v %v: %w", o.GetKind(), o.GetName(), err)
			}
			_, err = m.wctx.Core.Service().Create(&svc)
		default:
			logrus.Warnf("skip kind %v", o.GetKind())
		}
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				logrus.Warnf("skip create %v %v: %v", o.GetKind(), o.GetName(), err)
				time.Sleep(m.interval)
				continue
			}
			return fmt.Errorf("failed to create %v %v: %w", o.GetKind(), o.GetName(), err)
		}
		logrus.Infof("create %v: %v", o.GetKind(), o.GetName())
		time.Sleep(m.interval)
	}

	return nil
}

func (m *migrator) Clean(ctx context.Context) error {
	ips, err := m.listV1Resources(ctx, macvlanIPResource())
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to list %q: %w", v1IPCRD, err)
		}
	}
	if len(ips) != 0 {
		logrus.Warnf("unable to cleanup macvlanIP Resources: some pods still using Macvlan V1")
		logrus.Warnf("pods using Macvlan V1:")
		for _, ip := range ips {
			ip, _ := ip.(*types.MacvlanIP)
			if ip == nil {
				continue
			}
			logrus.Warnf("POD [%v/%v]: subnet [%v]",
				ip.Namespace, ip.Name, ip.Spec.Subnet)
		}
		logrus.Warnf("please ensure no workloads are using Macvlan V1 before cleanup CRDs")
		return fmt.Errorf("unable to cleanup: pods are still using Macvlan V1")
	}

	subnets, err := m.listV1Resources(ctx, macvlanSubnetResource())
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to list %q: %w", v1SubnetCRD, err)
		}
	}
	for _, s := range subnets {
		if err := m.deleteV1Subnet(ctx, s.GetNamespace(), s.GetName()); err != nil {
			return err
		}
	}
	if err := m.deleteV1CRD(ctx); err != nil {
		return err
	}
	return nil
}

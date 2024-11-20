package migrate

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nettypes "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
)

// listV1Service list all macvlan (v1) service resources
func (m *migrator) listV1Service(_ context.Context) ([]metav1.Object, error) {
	listOptions := metav1.ListOptions{
		Limit: m.listLimit,
	}
	var (
		v1Services = []metav1.Object{}
		r          *corev1.ServiceList
		err        error
	)
	for r == nil || listOptions.Continue != "" {
		r, err = m.wctx.Core.Service().List("", listOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to list corev1.Service resource: %w", err)
		}
		for i := 0; i < len(r.Items); i++ {
			s := r.Items[i]
			if !utils.IsMacvlanV1Service(&s) {
				continue
			}
			s.APIVersion = "v1"
			s.Kind = "Service"
			v1Services = append(v1Services, s.DeepCopy())
		}
		listOptions.Continue = r.GetContinue()
		time.Sleep(m.interval)
	}

	return v1Services, nil
}

func (m *migrator) migrateService(ctx context.Context) error {
	var err error
	var listOption = metav1.ListOptions{
		Limit: 100,
	}
	var svcs *corev1.ServiceList
	for svcs == nil || svcs.Continue != "" {
		svcs, err = m.wctx.Core.Service().List("", listOption)
		if err != nil {
			return fmt.Errorf("failed to list service: %w", err)
		}

		if err := m.migrateServiceList(ctx, svcs); err != nil {
			return err
		}
		listOption.Continue = svcs.Continue
	}

	logrus.Infof("Done migrating V1 Service resources")
	logrus.Infof("You need to delete old V1 Macvlan Services manually")
	logrus.Infof("====================================================")
	return nil
}

func (m *migrator) migrateServiceList(ctx context.Context, svcs *corev1.ServiceList) error {
	if svcs == nil || len(svcs.Items) == 0 {
		logrus.Infof("no existing macvlan V1 services, skip")
		return nil
	}
	for _, svc := range svcs.Items {
		if !utils.IsMacvlanV1Service(&svc) {
			continue
		}
		fs := newFlatNetworkService(&svc)
		logrus.Infof("Will create V2 service [%v/%v] from V1 service [%v/%v]",
			fs.Namespace, fs.Name, svc.Namespace, svc.Name)
		if err := utils.PromptUser(ctx, "Continue?", m.autoYes); err != nil {
			return fmt.Errorf("failed to create service %v: %w",
				fs.Name, err)
		}

		time.Sleep(m.interval)

		_, err := m.wctx.Core.Service().Create(fs)
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				logrus.Warnf("service [%v/%v] already exists, skip", fs.Namespace, fs.Name)
				continue
			}

			return fmt.Errorf("failed to create service %v: %w",
				fs.Name, err)
		}
		logrus.Infof("create service [%v/%v]", fs.Namespace, fs.Name)
		time.Sleep(m.interval)
	}
	return nil
}

func newFlatNetworkService(svc *corev1.Service) *corev1.Service {
	svc = svc.DeepCopy()
	svc.Name = strings.TrimSuffix(svc.Name, "-macvlan") + utils.FlatNetworkServiceNameSuffix
	ports := []corev1.ServicePort{}
	for _, v := range svc.Spec.Ports {
		port := v.DeepCopy()
		port.NodePort = 0
		ports = append(ports, *port)
	}

	ownerReference := svc.OwnerReferences
	s := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            svc.Name,
			Namespace:       svc.Namespace,
			OwnerReferences: ownerReference,
			Annotations: map[string]string{
				nettypes.NetworkAttachmentAnnot: utils.NetAttatchDefName,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports:    ports,
			Selector: maps.Clone(svc.Spec.Selector),
			// Setting this to "None" makes a "headless service" (no virtual IP),
			// which is useful when direct endpoint connections are preferred and
			// proxying is not required.
			ClusterIP:  corev1.ClusterIPNone,
			ClusterIPs: []string{"None"},
			Type:       "ClusterIP",
		},
	}

	return s
}

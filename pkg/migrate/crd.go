package migrate

import (
	"context"
	"fmt"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	v1SubnetCRD = "macvlansubnets.macvlan.cluster.cattle.io"
	v1IPCRD     = "macvlanips.macvlan.cluster.cattle.io"
	v2SubnetCRD = "flatnetworksubnets.flatnetwork.pandaria.io"
)

func (m *migrator) getV1SubnetCRD(ctx context.Context) (metav1.Object, error) {
	result, err := m.dynamicClientSet.Resource(crdResource()).Get(
		ctx, v1SubnetCRD, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Warnf("skip backup macvlan.cluster.cattle.io CRD: not found")
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get %q CRD: %w", v1SubnetCRD, err)
	}
	return result, nil
}

func (m *migrator) getV2SubnetCRD(ctx context.Context) (metav1.Object, error) {
	result, err := m.dynamicClientSet.Resource(crdResource()).Get(
		ctx, v2SubnetCRD, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Warnf("skip backup macvlan.cluster.cattle.io CRD: not found")
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get %q CRD: %w", v2SubnetCRD, err)
	}
	return result, nil
}

func (m *migrator) getV2Namespace(_ context.Context) (metav1.Object, error) {
	result, err := m.wctx.Core.Namespace().Get(flv1.SubnetNamespace, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Warnf("skip backup Namespace %v: not found", flv1.SubnetNamespace)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get %q namespace: %w", flv1.SubnetNamespace, err)
	}
	return result, nil
}

func crdResource() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}
}

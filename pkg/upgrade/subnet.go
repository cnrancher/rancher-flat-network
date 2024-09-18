package upgrade

import (
	"context"
	"fmt"
	"maps"
	"net"
	"time"

	"github.com/cnrancher/rancher-flat-network/pkg/upgrade/types"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
)

func (m *migrator) migrateSubnet(ctx context.Context) error {
	listOptions := metav1.ListOptions{
		Limit: m.listLimit,
	}
	var (
		macvlanSubnets *unstructured.UnstructuredList
		err            error
	)
	for macvlanSubnets == nil || listOptions.Continue != "" {
		macvlanSubnets, err = m.dynamicClientSet.Resource(macvlanSubnetResource()).List(ctx, listOptions)
		if err != nil {
			return fmt.Errorf("failed to list MacvlanSubnet resource: %w", err)
		}
		if err := m.migrateSubnetList(ctx, macvlanSubnets); err != nil {
			return err
		}
		listOptions.Continue = macvlanSubnets.GetContinue()
	}
	logrus.Infof("Done creating V2 FlatNetwork Subnet resources")
	logrus.Infof("You need to delete old V1 MacvlanSubnets manually")
	logrus.Infof("====================================================")
	time.Sleep(time.Second)

	return nil
}

func (m *migrator) migrateSubnetList(
	_ context.Context, macvlanSubnets *unstructured.UnstructuredList,
) error {
	if len(macvlanSubnets.Items) == 0 {
		logrus.Infof("macvlansubnets.macvlan.cluster.cattle.io resources already migrated")
		return nil
	}
	logrus.Debugf("Start migrating %d subnets", len(macvlanSubnets.Items))
	var err error
	for _, item := range macvlanSubnets.Items {
		subnet := types.MacvlanSubnet{}
		err = runtime.DefaultUnstructuredConverter.
			FromUnstructured(item.Object, &subnet)
		if err != nil {
			return fmt.Errorf("failed to convert unstruct object to macvlan.cluster.cattle.io/v1 MacvlanSubnet: %w", err)
		}

		// Create new flatNetwork Subnet
		fs, err := newFlatNetworkSubnet(&subnet)
		if err != nil {
			return err
		}
		_, err = m.wctx.FlatNetwork.FlatNetworkSubnet().Create(fs)
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				logrus.Warnf("Skip create V2 FlatNetwork Subnet [%v]: already exists", fs.Name)
				time.Sleep(m.interval)
				continue
			}
			return fmt.Errorf("failed to create V2 FlatNetwork Subnet [%v]: %w", fs.Name, err)
		}
		logrus.Infof("Create V2 FlatNetworkSubnet [%v]", fs.Name)
		time.Sleep(m.interval)
	}
	return nil
}

func newFlatNetworkSubnet(ms *types.MacvlanSubnet) (*flv1.FlatNetworkSubnet, error) {
	if ms == nil {
		return nil, nil
	}

	fs := &flv1.FlatNetworkSubnet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: flv1.SchemeGroupVersion.String(),
			Kind:       "FlatNetworkSubnet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        ms.Name,
			Namespace:   flv1.SubnetNamespace,
			Labels:      maps.Clone(ms.Labels),
			Annotations: maps.Clone(ms.Annotations),
		},
		Spec: flv1.SubnetSpec{
			FlatMode:   flv1.FlatModeMacvlan,
			Master:     ms.Spec.Master,
			VLAN:       ms.Spec.VLAN,
			CIDR:       ms.Spec.CIDR,
			Mode:       ms.Spec.Mode,
			IPvlanFlag: "",
			Gateway:    net.ParseIP(ms.Spec.Gateway),
			Ranges:     nil,
			Routes:     nil,
			RouteSettings: flv1.RouteSettings{
				AddClusterCIDR:            false,
				AddServiceCIDR:            false,
				AddNodeCIDR:               false,
				AddPodIPToHost:            false,
				FlatNetworkDefaultGateway: ms.Spec.PodDefaultGateway.Enable,
			},
		},
	}

	if fs.Annotations == nil {
		fs.Annotations = make(map[string]string)
	}
	if fs.Labels == nil {
		fs.Labels = make(map[string]string)
	}

	if len(ms.Spec.Ranges) > 0 {
		for _, mr := range ms.Spec.Ranges {
			r := flv1.IPRange{
				From: net.ParseIP(mr.RangeStart),
				To:   net.ParseIP(mr.RangeEnd),
			}
			if len(r.From) == 0 || len(r.To) == 0 {
				return nil, fmt.Errorf("failed to parse MacvlanSubnet [%v] custom range [%v - %v]: invalid IP address",
					ms.Name, mr.RangeStart, mr.RangeEnd)
			}
			fs.Spec.Ranges = append(fs.Spec.Ranges, r)
		}
	}

	if len(ms.Spec.Routes) > 0 {
		for _, mr := range ms.Spec.Routes {
			r := flv1.Route{
				Dev:      mr.Iface,
				Dst:      mr.Dst,
				Src:      nil,
				Via:      net.ParseIP(mr.GW),
				Priority: 0,
			}
			if len(r.Via) == 0 && mr.GW != "" {
				return nil, fmt.Errorf("failed to parse MacvlanSubnet [%v] custom route gateway [%v]: invalid IP address",
					ms.Name, mr.GW)
			}
			fs.Spec.Routes = append(fs.Spec.Routes, r)
		}
	}
	return fs, nil
}

func macvlanSubnetResource() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "macvlan.cluster.cattle.io",
		Version:  "v1",
		Resource: "macvlansubnets",
	}
}

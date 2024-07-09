package webhook

import (
	"net"
	"testing"

	flv1 "github.com/cnrancher/rancher-flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCheckRoutesGW(t *testing.T) {
	subnet := &flv1.FlatNetworkSubnet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vlan10-a",
			Namespace: flv1.SubnetNamespace,
		},
		Spec: flv1.SubnetSpec{
			FlatMode: "macvlan",
			VLAN:     10,
			CIDR:     "10.27.18.0/24",
			Routes: []flv1.Route{
				{
					Dst:   "192.168.10.0/24",
					GW:    net.ParseIP("10.27.18.254"),
					Iface: "eth1",
				},
			},
		},
	}

	err := validateSubnetRouteGateway(subnet)
	assert.Nil(t, err)

	// 10.27.18.0/28: 10.27.18.0 - 10.27.18.15
	subnet = &flv1.FlatNetworkSubnet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vlan10-b",
			Namespace: flv1.SubnetNamespace,
		},
		Spec: flv1.SubnetSpec{
			VLAN: 10,
			CIDR: "10.27.18.0/28",
			Routes: []flv1.Route{
				{
					Dst:   "192.168.10.0/24",
					GW:    net.ParseIP("10.27.18.254"),
					Iface: "eth1",
				},
			},
		},
	}

	err = validateSubnetRouteGateway(subnet)
	assert.NotNil(t, err)
}

func Test_checkSubnetFlatMode(t *testing.T) {
	subnets := []*flv1.FlatNetworkSubnet{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vlan10-a",
				Namespace: flv1.SubnetNamespace,
			},
			Spec: flv1.SubnetSpec{
				FlatMode: flv1.FlatModeMacvlan,
				Master:   "eth0",
				VLAN:     10,
				CIDR:     "192.168.12.0/24",
				Mode:     "bridge",
				Gateway:  net.IPv4(192, 168, 12, 1),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vlan10-b",
				Namespace: flv1.SubnetNamespace,
			},
			Spec: flv1.SubnetSpec{
				FlatMode: flv1.FlatModeMacvlan,
				Master:   "eth0",
				VLAN:     10,
				CIDR:     "192.168.92.0/24",
				Mode:     "bridge",
				Gateway:  net.IPv4(192, 168, 92, 1),
			},
		},
	}
	subnet := &flv1.FlatNetworkSubnet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vlan10-N",
			Namespace: flv1.SubnetNamespace,
		},
		Spec: flv1.SubnetSpec{
			FlatMode: flv1.FlatModeMacvlan,
			Master:   "eth0",
			VLAN:     10,
			CIDR:     "192.168.12.0/24",
			Mode:     "bridge",
			Gateway:  net.IPv4(192, 168, 12, 1),
		},
	}
	assert.Nil(t, checkSubnetFlatMode(subnet, subnets))

	subnet.Spec.FlatMode = flv1.FlatModeIPvlan
	assert.NotNil(t, checkSubnetFlatMode(subnet, subnets))
}

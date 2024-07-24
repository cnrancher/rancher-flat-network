package common

import (
	"net"
	"testing"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
	assert.Nil(t, CheckSubnetFlatMode(subnet, subnets))

	subnet.Spec.FlatMode = flv1.FlatModeIPvlan
	assert.NotNil(t, CheckSubnetFlatMode(subnet, subnets))
}
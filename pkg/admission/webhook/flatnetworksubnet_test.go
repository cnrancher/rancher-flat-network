package webhook

import (
	"net"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flatnetworkv1 "github.com/cnrancher/rancher-flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
)

func TestCheckRoutesGW(t *testing.T) {
	wantSubnetA := flatnetworkv1.FlatNetworkSubnet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vlan10-a",
			Namespace: flatnetworkv1.SubnetNamespace,
		},
		Spec: flatnetworkv1.SubnetSpec{
			VLAN: 10,
			CIDR: "10.27.18.0/24",
			Routes: []flatnetworkv1.Route{
				{
					Destination: net.ParseIP("192.168.10.0/24"),
					Gateway:     net.ParseIP("10.27.18.254"),
					Interface:   "eth1",
				},
			},
		},
	}

	errA := validateSubnetRouteGateway(wantSubnetA)
	if errA != nil {
		t.Fatalf("expect no error, but got: %v", errA)
	}

	// 10.27.18.0/28: 10.27.18.0 - 10.27.18.15
	wantSubnetB := flatnetworkv1.FlatNetworkSubnet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vlan10-b",
			Namespace: flatnetworkv1.SubnetNamespace,
		},
		Spec: flatnetworkv1.SubnetSpec{
			VLAN: 10,
			CIDR: "10.27.18.0/28",
			Routes: []flatnetworkv1.Route{
				{
					Destination: net.ParseIP("192.168.10.0/24"),
					Gateway:     net.ParseIP("10.27.18.254"),
					Interface:   "eth1",
				},
			},
		},
	}

	errB := validateSubnetRouteGateway(wantSubnetB)
	if errB == nil {
		t.Fatal("expect an error occur, but got nil")
	}
}

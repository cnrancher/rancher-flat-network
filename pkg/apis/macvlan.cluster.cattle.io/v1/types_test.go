package v1_test

import (
	"encoding/json"
	"net"
	"os"
	"testing"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_MacvlanIP(t *testing.T) {
	macvlanIP := macvlanv1.MacvlanIP{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "macvlan.cluster.cattle.io/v1",
			Kind:       "MacvlanIP",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "example-ip",
		},
		Spec: macvlanv1.MacvlanIPSpec{
			Subnet: "example-subnet",
			PodID:  "DE6F1529-3C77-4E4E-8D46-8294E025DE80",
			// CIDR:   "192.168.0.0/24",
			MAC: "aa:bb:cc:dd:ee:ff",
		},
	}

	b, _ := json.MarshalIndent(macvlanIP, "", "  ")
	err := os.WriteFile("../../../../docs/ip-example.yaml", b, 0644)
	if err != nil {
		t.Error(err)
	}
}

func Test_MacvlanSubnet(t *testing.T) {
	subnet := macvlanv1.MacvlanSubnet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "macvlan.cluster.cattle.io/v1",
			Kind:       "MacvlanSubnet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-subnet",
			Namespace: macvlanv1.MacvlanSubnetNamespace,
			Labels: map[string]string{
				"project": "",
			},
		},
		Spec: macvlanv1.MacvlanSubnetSpec{
			Master:  "eth0",
			VLAN:    0,
			CIDR:    "192.168.1.0/24",
			Mode:    "",
			Gateway: nil,
			Ranges: []macvlanv1.IPRange{
				{
					RangeStart: net.ParseIP("192.168.1.100"),
					RangeEnd:   net.ParseIP("192.168.1.120"),
				},
				{
					RangeStart: net.ParseIP("192.168.1.150"),
					RangeEnd:   net.ParseIP("192.168.1.160"),
				},
				{
					RangeStart: net.ParseIP("192.168.1.200"),
					RangeEnd:   net.ParseIP("192.168.1.220"),
				},
			},
			Routes: []macvlanv1.Route{},
			PodDefaultGateway: macvlanv1.PodDefaultGateway{
				Enable:      false,
				ServiceCIDR: "",
			},
			IPDelayReuse: 0,
		},
	}

	b, _ := json.MarshalIndent(subnet, "", "  ")
	err := os.WriteFile("../../../../docs/subnet-example.yaml", b, 0644)
	if err != nil {
		t.Error(err)
	}
}

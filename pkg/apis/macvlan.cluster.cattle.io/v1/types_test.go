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
	ip := macvlanv1.MacvlanIP{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "macvlan.cluster.cattle.io/v1",
			Kind:       "MacvlanIP",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "example-ip",
		},
		Spec: macvlanv1.MacvlanIPSpec{
			Subnet: "",
			PodID:  "",
			CIDR:   "192.168.0.0/24",
			MAC:    "aa:bb:cc:dd:ee:ff",
		},
	}

	b, _ := json.MarshalIndent(ip, "", "  ")
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
			CIDR:    "192.168.0.0/24",
			Mode:    "",
			Gateway: net.ParseIP("192.168.1.1"),
			Ranges: []macvlanv1.IPRange{
				{
					RangeStart: net.ParseIP("192.168.1.100"),
					RangeEnd:   net.ParseIP("192.168.1.200"),
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

package v1_test

import (
	"encoding/json"
	"net"
	"os"
	"testing"

	flv1 "github.com/cnrancher/flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var subnet = flv1.FlatNetworkSubnet{
	TypeMeta: metav1.TypeMeta{
		APIVersion: "flatnetwork.pandaria.io/v1",
		Kind:       "FlatNetworkSubnet",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "example-macvlan-subnet",
		Namespace: flv1.SubnetNamespace,
		Labels: map[string]string{
			"project": "",
		},
	},
	Spec: flv1.SubnetSpec{
		FlatMode: "macvlan",
		Master:   "eth0",
		VLAN:     0,
		CIDR:     "192.168.1.0/24",
		Mode:     "",
		Gateway:  nil,
		Ranges: []flv1.IPRange{
			{
				Start: net.ParseIP("192.168.1.100"),
				End:   net.ParseIP("192.168.1.200"),
			},
		},
		Routes: []flv1.Route{},
		PodDefaultGateway: flv1.PodDefaultGateway{
			Enable:      false,
			ServiceCIDR: "",
		},
	},
}

func saveYaml(obj any, path string) error {
	b, _ := json.MarshalIndent(obj, "", "  ")
	m := map[string]any{}
	err := json.Unmarshal(b, &m)
	if err != nil {
		return err
	}
	b, _ = yaml.Marshal(m)
	return os.WriteFile(path, b, 0644)
}

func Test_FlatNetworkSubnet_Macvlan(t *testing.T) {
	subnet.Name = "example-macvlan-subnet"
	subnet.Spec.FlatMode = "macvlan"
	err := saveYaml(subnet, "../../../../docs/macvlan/subnet-example.yaml")
	if err != nil {
		t.Error(err)
	}
}

func Test_FlatNetworkSubnet_Macvlan_IPv6(t *testing.T) {
	subnet.Name = "example-macvlan-subnet-ipv6"
	subnet.Spec.FlatMode = "macvlan"
	subnet.Spec.CIDR = "fd00:aaaa::/64"
	subnet.Spec.Ranges = []flv1.IPRange{
		{
			Start: net.ParseIP("fd00:aaaa::1000"),
			End:   net.ParseIP("fd00:aaaa::ffff"),
		},
	}
	err := saveYaml(subnet, "../../../../docs/macvlan/subnet-example-ipv6.yaml")
	if err != nil {
		t.Error(err)
	}
}

func Test_FlatNetworkSubnet_IPvlan(t *testing.T) {
	subnet.Name = "example-ipvaln-subnet"
	subnet.Spec.FlatMode = "ipvlan"
	subnet.Spec.CIDR = "192.168.2.0/24"
	subnet.Spec.Ranges = []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.2.100"),
			End:   net.ParseIP("192.168.2.150"),
		},
	}
	err := saveYaml(subnet, "../../../../docs/ipvlan/subnet-example.yaml")
	if err != nil {
		t.Error(err)
	}
}

func Test_FlatNetworkSubnet_IPvlan_IPv6(t *testing.T) {
	subnet.Name = "example-ipvlan-subnet-ipv6"
	subnet.Spec.FlatMode = "ipvlan"
	subnet.Spec.Ranges = []flv1.IPRange{
		{
			Start: net.ParseIP("fd00:eeee::1000"),
			End:   net.ParseIP("fd00:eeee::ffff"),
		},
	}
	err := saveYaml(subnet, "../../../../docs/ipvlan/subnet-example-ipv6.yaml")
	if err != nil {
		t.Error(err)
	}
}

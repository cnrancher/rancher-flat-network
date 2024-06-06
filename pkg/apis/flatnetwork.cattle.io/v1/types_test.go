package v1_test

import (
	"encoding/json"
	"net"
	"os"
	"testing"

	flv1 "github.com/cnrancher/flat-network-operator/pkg/apis/flatnetwork.cattle.io/v1"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_IP(t *testing.T) {
	IP := flv1.IP{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "flatnetwork.cattle.io/v1",
			Kind:       "IP",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "example-ip",
		},
		Spec: flv1.IPSpec{
			Subnet: "example-subnet",
			PodID:  "DE6F1529-3C77-4E4E-8D46-8294E025DE80",
			// CIDR:   "192.168.0.0/24",
			MAC: "aa:bb:cc:dd:ee:ff",
		},
	}

	b, _ := json.MarshalIndent(IP, "", "  ")
	m := map[string]any{}

	json.Unmarshal(b, &m)
	b, _ = yaml.Marshal(m)

	err := os.WriteFile("../../../../docs/ip-example.yaml", b, 0644)
	if err != nil {
		t.Error(err)
	}
}

func Test_Subnet(t *testing.T) {
	subnet := flv1.Subnet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "flatnetwork.cattle.io/v1",
			Kind:       "Subnet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-subnet",
			Namespace: flv1.SubnetNamespace,
			Labels: map[string]string{
				"project": "",
			},
		},
		Spec: flv1.SubnetSpec{
			Master:  "eth0",
			VLAN:    0,
			CIDR:    "192.168.1.0/24",
			Mode:    "",
			Gateway: nil,
			Ranges: []flv1.IPRange{
				{
					Start: net.ParseIP("192.168.1.100"),
					End:   net.ParseIP("192.168.1.120"),
				},
				{
					Start: net.ParseIP("192.168.1.150"),
					End:   net.ParseIP("192.168.1.160"),
				},
				{
					Start: net.ParseIP("192.168.1.200"),
					End:   net.ParseIP("192.168.1.220"),
				},
			},
			Routes: []flv1.Route{},
			PodDefaultGateway: flv1.PodDefaultGateway{
				Enable:      false,
				ServiceCIDR: "",
			},
		},
	}

	b, _ := json.MarshalIndent(subnet, "", "  ")

	m := map[string]any{}
	err := json.Unmarshal(b, &m)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = yaml.Marshal(m)

	err = os.WriteFile("../../../../docs/subnet-example.yaml", b, 0644)
	if err != nil {
		t.Error(err)
	}
}

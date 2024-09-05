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

func Test_CheckPodAnnotationIPs(t *testing.T) {
	ips, err := CheckPodAnnotationIPs("")
	assert.Empty(t, ips)
	assert.Nil(t, err)

	ips, err = CheckPodAnnotationIPs("auto")
	assert.Empty(t, ips)
	assert.Nil(t, err)

	ips, err = CheckPodAnnotationIPs("192.168.1.111")
	assert.Equal(t, ips, []net.IP{net.ParseIP("192.168.1.111")})
	assert.Nil(t, err)

	ips, err = CheckPodAnnotationIPs("192.168.1.111-192.168.1.112-192.168.1.113")
	assert.Equal(t, ips, []net.IP{
		net.ParseIP("192.168.1.111"),
		net.ParseIP("192.168.1.112"),
		net.ParseIP("192.168.1.113"),
	})
	assert.Nil(t, err)

	ips, err = CheckPodAnnotationIPs("192.168.1.111-192.168.1.112-192.168.1.1a3")
	assert.Empty(t, ips)
	assert.NotNil(t, err)
}

func Test_CheckPodAnnotationMACs(t *testing.T) {
	macs, err := CheckPodAnnotationMACs("")
	assert.Empty(t, macs)
	assert.Nil(t, err)

	macs, err = CheckPodAnnotationMACs("auto")
	assert.Empty(t, macs)
	assert.Nil(t, err)

	macs, err = CheckPodAnnotationMACs("aa:bb:cc:dd:ef:01")
	assert.Equal(t, macs, []string{"aa:bb:cc:dd:ef:01"})
	assert.Nil(t, err)

	macs, err = CheckPodAnnotationMACs("aa:bb:cc:dd:ef:01-aa:bb:cc:dd:ef:02-aa:bb:cc:dd:ef:03")
	assert.Equal(t, macs, []string{
		"aa:bb:cc:dd:ef:01",
		"aa:bb:cc:dd:ef:02",
		"aa:bb:cc:dd:ef:03",
	})
	assert.Nil(t, err)

	macs, err = CheckPodAnnotationMACs("aa:bb:cc:dd:ef:01-aa:bb:cc:dd:ef:02-aa:bb:cc:dd:ef:03:aa")
	assert.Empty(t, macs)
	assert.NotNil(t, err)
}

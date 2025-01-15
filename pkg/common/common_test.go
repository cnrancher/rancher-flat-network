package common

import (
	"net"
	"testing"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/ipcalc"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// All subnets on eth0.10 are using same flatMode: macvlan
	assert.Nil(t, CheckSubnetFlatMode(subnet, subnets))

	// eth0.10 already used by macvlan
	subnet.Spec.FlatMode = flv1.FlatModeIPvlan
	subnet.Spec.Mode = "l2"
	subnet.Spec.IPvlanFlag = "bridge"
	err := CheckSubnetFlatMode(subnet, subnets)
	assert.ErrorContains(t, err, "already using master iface [eth0.10]")

	subnet.Spec.VLAN = 20
	assert.Nil(t, CheckSubnetFlatMode(subnet, subnets))
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

func Test_CheckSubnetConflict(t *testing.T) {
	assert := assert.New(t)
	var err error

	subnets := []*flv1.FlatNetworkSubnet{
		{
			ObjectMeta: v1.ObjectMeta{
				Name: "t1",
			},
			Spec: flv1.SubnetSpec{
				FlatMode: "macvlan",
				Master:   "eth0",
				VLAN:     10,
				CIDR:     "192.168.1.0/24",
				Mode:     "bridge",
				Ranges: []flv1.IPRange{
					{
						From: net.ParseIP("192.168.1.10"),
						To:   net.ParseIP("192.168.1.20"),
					},
				},
			},
		},
		{
			ObjectMeta: v1.ObjectMeta{
				Name: "t2",
			},
			Spec: flv1.SubnetSpec{
				FlatMode: "macvlan",
				Master:   "eth0",
				VLAN:     20,
				CIDR:     "192.168.1.0/24",
				Mode:     "bridge",
				Ranges:   []flv1.IPRange{},
			},
		},
	}

	s1 := &flv1.FlatNetworkSubnet{
		ObjectMeta: v1.ObjectMeta{
			Name: "a1",
		},
		Spec: flv1.SubnetSpec{
			FlatMode: "macvlan",
			Master:   "eth0",
			VLAN:     10,
			CIDR:     "192.168.1.0/24",
			Mode:     "bridge",
			Ranges:   []flv1.IPRange{},
		},
	}
	// default range is 192.168.1.1 - 192.168.1.254
	err = CheckSubnetConflict(s1, subnets)
	assert.ErrorIs(err, ipcalc.ErrNetworkConflict) // should return CIDR conflict
	t.Log(err)

	s1.Spec.Ranges = append(s1.Spec.Ranges, flv1.IPRange{ // 192.168.1.10 - 192.168.1.20
		From: net.ParseIP("192.168.1.10"),
		To:   net.ParseIP("192.168.1.20"),
	})
	err = CheckSubnetConflict(s1, subnets)
	assert.ErrorIs(err, ipcalc.ErrIPRangesConflict) // should return ip ranges conflict
	t.Log(err)

	s1.Spec.Ranges[0].From = net.ParseIP("192.168.1.15") // 192.168.1.15 - 192.168.1.20
	err = CheckSubnetConflict(s1, subnets)
	assert.ErrorIs(err, ipcalc.ErrIPRangesConflict) // should return ip ranges conflict
	t.Log(err)

	s1.Spec.Ranges[0].To = net.ParseIP("192.168.1.25") // 192.168.1.15 - 192.168.1.25
	err = CheckSubnetConflict(s1, subnets)
	assert.ErrorIs(err, ipcalc.ErrIPRangesConflict) // should return ip ranges conflict
	t.Log(err)

	s1.Spec.Ranges[0].From = net.ParseIP("192.168.1.5") // 192.168.1.5 - 192.168.1.10
	s1.Spec.Ranges[0].To = net.ParseIP("192.168.1.10")
	err = CheckSubnetConflict(s1, subnets)
	assert.ErrorIs(err, ipcalc.ErrIPRangesConflict) // should return ip ranges conflict
	t.Log(err)

	s1.Spec.Ranges[0].From = net.ParseIP("192.168.1.20") // 192.168.1.20 - 192.168.1.25
	s1.Spec.Ranges[0].To = net.ParseIP("192.168.1.25")
	err = CheckSubnetConflict(s1, subnets)
	assert.ErrorIs(err, ipcalc.ErrIPRangesConflict) // should return ip ranges conflict
	t.Log(err)

	s1.Spec.Ranges[0].From = net.ParseIP("192.168.1.5") // 192.168.1.5 - 192.168.1.25
	err = CheckSubnetConflict(s1, subnets)
	assert.ErrorIs(err, ipcalc.ErrIPRangesConflict) // should return ip ranges conflict
	t.Log(err)

	// revert the test ranges.
	err = CheckSubnetConflict(subnets[0], []*flv1.FlatNetworkSubnet{s1})
	assert.ErrorIs(err, ipcalc.ErrIPRangesConflict) // should return ip ranges conflict
	t.Log(err)

	s1.Spec.Ranges[0].From = net.ParseIP("192.168.1.13") // 192.168.1.13 - 192.168.1.16
	s1.Spec.Ranges[0].To = net.ParseIP("192.168.1.16")
	err = CheckSubnetConflict(s1, subnets)
	assert.ErrorIs(err, ipcalc.ErrIPRangesConflict) // should return ip ranges conflict
	t.Log(err)

	s1.Spec.Ranges[0].From = net.ParseIP("192.168.1.100")
	s1.Spec.Ranges[0].To = net.ParseIP("192.168.1.200") // 192.168.1.100 - 192.168.1.200
	err = CheckSubnetConflict(s1, subnets)
	assert.Nil(err) // should not return error
	t.Log(err)

	s1.Spec.Ranges[0].From = net.ParseIP("192.168.1.0")
	s1.Spec.Ranges[0].To = net.ParseIP("192.168.1.9") // 192.168.1.0 - 192.168.1.9
	err = CheckSubnetConflict(s1, subnets)
	assert.Nil(err) // should not return error
	t.Log(err)

	s1.Spec.Ranges[0].From = net.ParseIP("192.168.1.15")
	s1.Spec.Ranges[0].To = net.ParseIP("192.168.1.25") // 192.168.1.0 - 192.168.1.9
	s1.Spec.VLAN = 5                                   // using diff VLAN ID
	err = CheckSubnetConflict(s1, subnets)
	assert.Nil(err) // should not return error
	t.Log(err)

	s1.Spec.VLAN = 20 // using same VLAN ID with t2
	err = CheckSubnetConflict(s1, subnets)
	// subnet t2 does not have custom ip range specified,
	// should return CIDR conflict.
	assert.ErrorIs(err, ipcalc.ErrNetworkConflict)
	t.Log(err)
}

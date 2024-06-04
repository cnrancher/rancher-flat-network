package ipcalc

import (
	"net"
	"testing"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	"gotest.tools/v3/assert"
)

func Test_IPIncrease(t *testing.T) {
	ip := net.IPv4(192, 168, 1, 1)
	IPIncrease(ip)
	assert.DeepEqual(t, ip, net.IPv4(192, 168, 1, 2))

	ip = net.IPv4(192, 168, 1, 255)
	IPIncrease(ip)
	assert.DeepEqual(t, ip, net.IPv4(192, 168, 2, 0))
}

func Test_IPDecrease(t *testing.T) {
	ip := net.IPv4(192, 168, 1, 1)
	IPDecrease(ip)
	assert.DeepEqual(t, ip, net.IPv4(192, 168, 1, 0))
	IPDecrease(ip)
	assert.DeepEqual(t, ip, net.IPv4(192, 168, 0, 255))
}

func Test_CalcDefaultGateway(t *testing.T) {
	ip, _ := GetDefaultGateway("192.168.1.0/24")
	assert.DeepEqual(t, net.ParseIP("192.168.1.1"), ip)
	ip, _ = GetDefaultGateway("")
	assert.Check(t, ip == nil)
}

func Test_IPInRanges(t *testing.T) {
	var ip net.IP = []byte{1, 2, 3}
	if IPInRanges(ip, nil) {
		t.Fatal("failed")
	}
	if IPInRanges(ip, []macvlanv1.IPRange{}) {
		t.Fatal("failed")
	}
	if IPInRanges(ip, []macvlanv1.IPRange{
		{
			RangeStart: nil,
			RangeEnd:   nil,
		},
	}) {
		t.Fatal("failed")
	}

	ip = net.ParseIP("10.0.0.1")
	if IPInRanges(ip, nil) {
		t.Fatal("failed")
	}
	if IPInRanges(ip, []macvlanv1.IPRange{}) {
		t.Fatal("failed")
	}
	if IPInRanges(ip, []macvlanv1.IPRange{
		{
			RangeStart: nil,
			RangeEnd:   nil,
		},
	}) {
		t.Fatal("failed")
	}

	var ipRanges = []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("10.0.0.1"),
			RangeEnd:   net.ParseIP("10.0.0.1"),
		},
	}
	if !IPInRanges(ip, ipRanges) {
		t.Fatal("failed")
	}

	ipRanges = []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("10.0.0.1"),
			RangeEnd:   net.ParseIP("10.0.0.255"),
		},
	}
	if !IPInRanges(ip, ipRanges) {
		t.Fatal("failed")
	}

	ip = net.ParseIP("10.0.0.100")
	if !IPInRanges(ip, ipRanges) {
		t.Fatal("failed")
	}

	ip = net.ParseIP("192.168.0.1")
	if IPInRanges(ip, ipRanges) {
		t.Fatal("failed")
	}
}

func Test_IPNotUsed(t *testing.T) {
	var ip net.IP = []byte{1, 2, 3}
	if !IPNotUsed(ip, nil) {
		t.Fatal("failed")
	}
	if !IPNotUsed(ip, []macvlanv1.IPRange{}) {
		t.Fatal("failed")
	}
	if !IPNotUsed(ip, []macvlanv1.IPRange{
		{
			RangeStart: nil,
			RangeEnd:   nil,
		},
	}) {
		t.Fatal("failed")
	}

	ip = net.ParseIP("10.0.0.1")
	if !IPNotUsed(ip, nil) {
		t.Fatal("failed")
	}
	if !IPNotUsed(ip, []macvlanv1.IPRange{}) {
		t.Fatal("failed")
	}
	if !IPNotUsed(ip, []macvlanv1.IPRange{
		{
			RangeStart: nil,
			RangeEnd:   nil,
		},
	}) {
		t.Fatal("failed")
	}

	var usedRanges = []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("10.0.0.1"),
			RangeEnd:   net.ParseIP("10.0.0.1"),
		},
	}
	if IPNotUsed(ip, usedRanges) {
		t.Fatal("failed")
	}

	usedRanges = []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("10.0.0.100"),
			RangeEnd:   net.ParseIP("10.0.0.200"),
		},
	}
	if !IPNotUsed(ip, usedRanges) {
		t.Fatal("failed")
	}
	ip = net.ParseIP("10.0.0.110")
	if IPNotUsed(ip, usedRanges) {
		t.Fatal("failed")
	}
}

func Test_GetAvailableIP(t *testing.T) {
	ip, err := GetAvailableIP("invalid data", nil, nil)
	assert.ErrorContains(t, err, "invalid")
	assert.Equal(t, len(ip), 0)

	ip, err = GetAvailableIP("10.0.0.0/24", nil, nil)
	assert.NilError(t, err)
	assert.DeepEqual(t, ip, net.ParseIP("10.0.0.1"))

	ip, err = GetAvailableIP("10.0.0.0/24", []macvlanv1.IPRange{}, []macvlanv1.IPRange{})
	assert.NilError(t, err)
	assert.DeepEqual(t, ip, net.ParseIP("10.0.0.1"))

	ip, _ = GetAvailableIP(
		"10.0.0.0/24",
		[]macvlanv1.IPRange{
			{
				RangeStart: net.ParseIP("10.0.0.100"),
				RangeEnd:   net.ParseIP("10.0.0.200"),
			},
		},
		[]macvlanv1.IPRange{},
	)
	assert.DeepEqual(t, ip, net.ParseIP("10.0.0.100"))

	ip, _ = GetAvailableIP(
		"10.0.0.0/24",
		[]macvlanv1.IPRange{
			{
				RangeStart: net.ParseIP("10.0.0.100"),
				RangeEnd:   net.ParseIP("10.0.0.200"),
			},
			{
				RangeStart: net.ParseIP("10.0.0.210"),
				RangeEnd:   net.ParseIP("10.0.0.220"),
			},
		},
		[]macvlanv1.IPRange{
			{
				RangeStart: net.ParseIP("10.0.0.100"),
				RangeEnd:   net.ParseIP("10.0.0.200"),
			},
		},
	)
	assert.DeepEqual(t, ip, net.ParseIP("10.0.0.210"))

	ip, _ = GetAvailableIP(
		"10.0.0.0/24",
		[]macvlanv1.IPRange{
			{
				RangeStart: net.ParseIP("10.0.0.100"),
				RangeEnd:   net.ParseIP("10.0.0.200"),
			},
			{
				RangeStart: net.ParseIP("10.0.0.210"),
				RangeEnd:   net.ParseIP("10.0.0.220"),
			},
		},
		[]macvlanv1.IPRange{
			{
				RangeStart: net.ParseIP("10.0.0.100"),
				RangeEnd:   net.ParseIP("10.0.0.200"),
			},
			{
				RangeStart: net.ParseIP("10.0.0.210"),
				RangeEnd:   net.ParseIP("10.0.0.210"),
			},
		},
	)
	assert.DeepEqual(t, ip, net.ParseIP("10.0.0.211"))

	ip, _ = GetAvailableIP(
		"10.0.0.0/8",
		[]macvlanv1.IPRange{
			{
				RangeStart: net.ParseIP("10.255.255.254"),
				RangeEnd:   net.ParseIP("10.255.255.254"),
			},
		},
		[]macvlanv1.IPRange{},
	)
	assert.DeepEqual(t, ip, net.ParseIP("10.255.255.254"))

	ip, err = GetAvailableIP(
		"10.0.0.0/8",
		[]macvlanv1.IPRange{
			{
				RangeStart: net.ParseIP("10.255.255.254"),
				RangeEnd:   net.ParseIP("10.255.255.254"),
			},
		},
		[]macvlanv1.IPRange{
			{
				RangeStart: net.ParseIP("10.255.255.254"),
				RangeEnd:   net.ParseIP("10.255.255.254"),
			},
		},
	)
	assert.ErrorIs(t, err, ErrNoAvailableIP)
	assert.Equal(t, len(ip), 0)

	ip, err = GetAvailableIP(
		"10.0.0.0/8",
		[]macvlanv1.IPRange{},
		[]macvlanv1.IPRange{
			{
				RangeStart: net.ParseIP("10.0.0.0"),
				RangeEnd:   net.ParseIP("10.255.255.254"),
			},
		},
	)
	assert.ErrorIs(t, err, ErrNoAvailableIP)
	assert.Equal(t, len(ip), 0)
}

func Test_AddCIDRSuffix(t *testing.T) {
	ip := net.ParseIP("192.168.1.12")
	c := AddCIDRSuffix(ip, "192.168.1.0/24")
	assert.Equal(t, "192.168.1.12/24", c)

	ip = net.ParseIP("10.0.0.1")
	c = AddCIDRSuffix(ip, "10.0.0.0/8")
	assert.Equal(t, "10.0.0.1/8", c)

	ip = net.ParseIP("172.31.1.100")
	c = AddCIDRSuffix(ip, "172.16.0.0/16")
	assert.Equal(t, "172.31.1.100/16", c)

	ip = net.ParseIP("172.31.1.100")
	c = AddCIDRSuffix(ip, "172.16.0.0")
	assert.Equal(t, "172.31.1.100/32", c)
}

func Test_AddIPToRange(t *testing.T) {
	r := AddIPToRange(nil, nil)
	assert.Equal(t, len(r), 0)

	r = AddIPToRange(net.ParseIP("192.168.1.12"), nil)
	assert.DeepEqual(t, r, []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("192.168.1.12"),
			RangeEnd:   net.ParseIP("192.168.1.12"),
		},
	})

	r = AddIPToRange(net.ParseIP("192.168.1.12"), r)
	assert.DeepEqual(t, r, []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("192.168.1.12"),
			RangeEnd:   net.ParseIP("192.168.1.12"),
		},
	})

	// Re-add existing ip in range.
	r = AddIPToRange(net.ParseIP("192.168.1.12"), r)
	assert.DeepEqual(t, r, []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("192.168.1.12"),
			RangeEnd:   net.ParseIP("192.168.1.12"),
		},
	})

	r = AddIPToRange(net.ParseIP("192.168.1.13"), r)
	assert.DeepEqual(t, r, []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("192.168.1.12"),
			RangeEnd:   net.ParseIP("192.168.1.13"),
		},
	})

	r = AddIPToRange(net.ParseIP("192.168.1.11"), r)
	assert.DeepEqual(t, r, []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("192.168.1.11"),
			RangeEnd:   net.ParseIP("192.168.1.13"),
		},
	})

	r = AddIPToRange(net.ParseIP("192.168.1.20"), r)
	assert.DeepEqual(t, r, []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("192.168.1.11"),
			RangeEnd:   net.ParseIP("192.168.1.13"),
		},
		{
			RangeStart: net.ParseIP("192.168.1.20"),
			RangeEnd:   net.ParseIP("192.168.1.20"),
		},
	})

	r = AddIPToRange(net.ParseIP("192.168.1.1"), r)
	assert.DeepEqual(t, r, []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("192.168.1.1"),
			RangeEnd:   net.ParseIP("192.168.1.1"),
		},
		{
			RangeStart: net.ParseIP("192.168.1.11"),
			RangeEnd:   net.ParseIP("192.168.1.13"),
		},
		{
			RangeStart: net.ParseIP("192.168.1.20"),
			RangeEnd:   net.ParseIP("192.168.1.20"),
		},
	})

	r = AddIPToRange(net.ParseIP("192.168.1.2"), r)
	assert.DeepEqual(t, r, []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("192.168.1.1"),
			RangeEnd:   net.ParseIP("192.168.1.2"),
		},
		{
			RangeStart: net.ParseIP("192.168.1.11"),
			RangeEnd:   net.ParseIP("192.168.1.13"),
		},
		{
			RangeStart: net.ParseIP("192.168.1.20"),
			RangeEnd:   net.ParseIP("192.168.1.20"),
		},
	})

	r = AddIPToRange(net.ParseIP("192.168.1.0"), r)
	assert.DeepEqual(t, r, []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("192.168.1.0"),
			RangeEnd:   net.ParseIP("192.168.1.2"),
		},
		{
			RangeStart: net.ParseIP("192.168.1.11"),
			RangeEnd:   net.ParseIP("192.168.1.13"),
		},
		{
			RangeStart: net.ParseIP("192.168.1.20"),
			RangeEnd:   net.ParseIP("192.168.1.20"),
		},
	})
}

func Test_RemoveIPFromRange(t *testing.T) {
	r := []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("192.168.1.100"),
			RangeEnd:   net.ParseIP("192.168.1.200"),
		},
	}
	r = RemoveIPFromRange(nil, r)
	assert.DeepEqual(t, r, []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("192.168.1.100"),
			RangeEnd:   net.ParseIP("192.168.1.200"),
		},
	})

	r = RemoveIPFromRange(net.ParseIP("192.168.1.101"), r)
	assert.DeepEqual(t, r, []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("192.168.1.100"),
			RangeEnd:   net.ParseIP("192.168.1.100"),
		},
		{
			RangeStart: net.ParseIP("192.168.1.102"),
			RangeEnd:   net.ParseIP("192.168.1.200"),
		},
	})

	r = RemoveIPFromRange(net.ParseIP("192.168.1.100"), r)
	assert.DeepEqual(t, r, []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("192.168.1.102"),
			RangeEnd:   net.ParseIP("192.168.1.200"),
		},
	})

	// Re-delete non-existing ip from range.
	r = RemoveIPFromRange(net.ParseIP("192.168.1.100"), r)
	assert.DeepEqual(t, r, []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("192.168.1.102"),
			RangeEnd:   net.ParseIP("192.168.1.200"),
		},
	})

	r = RemoveIPFromRange(net.ParseIP("192.168.1.102"), r)
	assert.DeepEqual(t, r, []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("192.168.1.103"),
			RangeEnd:   net.ParseIP("192.168.1.200"),
		},
	})

	r = RemoveIPFromRange(net.ParseIP("192.168.1.200"), r)
	assert.DeepEqual(t, r, []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("192.168.1.103"),
			RangeEnd:   net.ParseIP("192.168.1.199"),
		},
	})

	r = []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("192.168.1.100"),
			RangeEnd:   net.ParseIP("192.168.1.100"),
		},
	}
	r = RemoveIPFromRange(net.ParseIP("192.168.1.111"), r)
	assert.DeepEqual(t, r, []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("192.168.1.100"),
			RangeEnd:   net.ParseIP("192.168.1.100"),
		},
	})

	r = RemoveIPFromRange(net.ParseIP("192.168.1.100"), r)
	assert.DeepEqual(t, r, []macvlanv1.IPRange{})
}

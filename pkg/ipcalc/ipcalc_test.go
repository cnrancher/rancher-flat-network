package ipcalc

import (
	"net"
	"testing"
	"time"

	flv1 "github.com/cnrancher/flat-network-operator/pkg/apis/flatnetwork.cattle.io/v1"
	"github.com/stretchr/testify/assert"
)

func Test_IPIncrease(t *testing.T) {
	ip := net.IPv4(192, 168, 1, 1)
	IPIncrease(ip)
	assert.Equal(t, ip, net.IPv4(192, 168, 1, 2))

	ip = net.IPv4(192, 168, 1, 255)
	IPIncrease(ip)
	assert.Equal(t, ip, net.IPv4(192, 168, 2, 0))

	ip = net.ParseIP("fdaa:bbcc::1")
	IPIncrease(ip)
	assert.Equal(t, ip, net.ParseIP("fdaa:bbcc::2"))

	ip = net.ParseIP("fdaa:bbcc::ffff")
	IPIncrease(ip)
	assert.Equal(t, ip, net.ParseIP("fdaa:bbcc::1:0000"))
}

func Test_IPDecrease(t *testing.T) {
	ip := net.IPv4(192, 168, 1, 1)
	IPDecrease(ip)
	assert.Equal(t, ip, net.IPv4(192, 168, 1, 0))
	IPDecrease(ip)
	assert.Equal(t, ip, net.IPv4(192, 168, 0, 255))

	ip = net.ParseIP("fdaa:bbcc::1")
	IPDecrease(ip)
	assert.Equal(t, ip, net.ParseIP("fdaa:bbcc::"))
	IPDecrease(ip)
	assert.Equal(t, ip, net.ParseIP("fdaa:bbcb:ffff:ffff:ffff:ffff:ffff:ffff"))
}

func Test_CalcDefaultGateway(t *testing.T) {
	ip, _ := GetDefaultGateway("")
	assert.True(t, ip == nil)

	ip, _ = GetDefaultGateway("192.168.1.0/24")
	assert.Equal(t, net.ParseIP("192.168.1.1"), ip)

	ip, _ = GetDefaultGateway("fdab:cdef::/64")
	assert.Equal(t, net.ParseIP("fdab:cdef::1"), ip)
}

func Test_IPInRanges(t *testing.T) {
	var ip net.IP = []byte{1, 2, 3}
	if IPInRanges(ip, nil) {
		t.Fatal("failed")
	}
	if IPInRanges(ip, []flv1.IPRange{}) {
		t.Fatal("failed")
	}
	if IPInRanges(ip, []flv1.IPRange{
		{
			Start: nil,
			End:   nil,
		},
	}) {
		t.Fatal("failed")
	}

	ip = net.ParseIP("10.0.0.1")
	if IPInRanges(ip, nil) {
		t.Fatal("failed")
	}
	if IPInRanges(ip, []flv1.IPRange{}) {
		t.Fatal("failed")
	}
	if IPInRanges(ip, []flv1.IPRange{
		{
			Start: nil,
			End:   nil,
		},
	}) {
		t.Fatal("failed")
	}

	var ipRanges = []flv1.IPRange{
		{
			Start: net.ParseIP("10.0.0.1"),
			End:   net.ParseIP("10.0.0.1"),
		},
	}
	if !IPInRanges(ip, ipRanges) {
		t.Fatal("failed")
	}

	ipRanges = []flv1.IPRange{
		{
			Start: net.ParseIP("10.0.0.1"),
			End:   net.ParseIP("10.0.0.255"),
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

	// IPv6
	ip = net.ParseIP("fd00::1")
	ipRanges = []flv1.IPRange{
		{
			Start: net.ParseIP("fd00::1"),
			End:   net.ParseIP("fd00::1"),
		},
	}
	if !IPInRanges(ip, ipRanges) {
		t.Fatal("failed")
	}

	ip = net.ParseIP("fd00::2")
	if IPInRanges(ip, ipRanges) {
		t.Fatal("failed")
	}

	ipRanges[0].End = net.ParseIP("fd00::2")
	if !IPInRanges(ip, ipRanges) {
		t.Fatal("failed")
	}
}

func Test_GetAvailableIP(t *testing.T) {
	ip, err := GetAvailableIP("invalid data", nil, nil)
	assert.ErrorContains(t, err, "invalid")
	assert.Equal(t, len(ip), 0)

	ip, err = GetAvailableIP("10.0.0.0/24", nil, nil)
	assert.Nil(t, err)
	assert.Equal(t, ip, net.ParseIP("10.0.0.1"))

	ip, err = GetAvailableIP("10.0.0.0/24", []flv1.IPRange{}, []flv1.IPRange{})
	assert.Nil(t, err)
	assert.Equal(t, ip, net.ParseIP("10.0.0.1"))

	ip, _ = GetAvailableIP(
		"10.0.0.0/24",
		[]flv1.IPRange{
			{
				Start: net.ParseIP("10.0.0.100"),
				End:   net.ParseIP("10.0.0.200"),
			},
		},
		[]flv1.IPRange{},
	)
	assert.Equal(t, ip, net.ParseIP("10.0.0.100"))

	ip, _ = GetAvailableIP(
		"10.0.0.0/24",
		[]flv1.IPRange{
			{
				Start: net.ParseIP("10.0.0.100"),
				End:   net.ParseIP("10.0.0.200"),
			},
			{
				Start: net.ParseIP("10.0.0.210"),
				End:   net.ParseIP("10.0.0.220"),
			},
		},
		[]flv1.IPRange{
			{
				Start: net.ParseIP("10.0.0.100"),
				End:   net.ParseIP("10.0.0.200"),
			},
		},
	)
	assert.Equal(t, ip, net.ParseIP("10.0.0.210"))

	ip, _ = GetAvailableIP(
		"10.0.0.0/24",
		[]flv1.IPRange{
			{
				Start: net.ParseIP("10.0.0.100"),
				End:   net.ParseIP("10.0.0.200"),
			},
			{
				Start: net.ParseIP("10.0.0.210"),
				End:   net.ParseIP("10.0.0.220"),
			},
		},
		[]flv1.IPRange{
			{
				Start: net.ParseIP("10.0.0.100"),
				End:   net.ParseIP("10.0.0.200"),
			},
			{
				Start: net.ParseIP("10.0.0.210"),
				End:   net.ParseIP("10.0.0.210"),
			},
		},
	)
	assert.Equal(t, ip, net.ParseIP("10.0.0.211"))

	ip, _ = GetAvailableIP(
		"10.0.0.0/8",
		[]flv1.IPRange{
			{
				Start: net.ParseIP("10.255.255.254"),
				End:   net.ParseIP("10.255.255.254"),
			},
		},
		[]flv1.IPRange{},
	)
	assert.Equal(t, ip, net.ParseIP("10.255.255.254"))

	ip, err = GetAvailableIP(
		"10.0.0.0/8",
		[]flv1.IPRange{
			{
				Start: net.ParseIP("10.255.255.254"),
				End:   net.ParseIP("10.255.255.254"),
			},
		},
		[]flv1.IPRange{
			{
				Start: net.ParseIP("10.255.255.254"),
				End:   net.ParseIP("10.255.255.254"),
			},
		},
	)
	assert.ErrorIs(t, err, ErrNoAvailableIP)
	assert.Equal(t, len(ip), 0)

	now := time.Now().UnixMilli()
	ip, err = GetAvailableIP(
		"10.0.0.0/8",
		[]flv1.IPRange{},
		[]flv1.IPRange{
			{
				Start: net.ParseIP("10.0.0.1"),
				End:   net.ParseIP("10.255.255.254"),
			},
		},
	)
	assert.ErrorIs(t, err, ErrNoAvailableIP)
	assert.Equal(t, len(ip), 0)
	t.Logf("time consumed %.2f (s)", float64(time.Now().UnixMilli()-now)/1000.0)

	// Get available IP from IPv6
	ip, _ = GetAvailableIP(
		"fdaa::/16",
		[]flv1.IPRange{},
		[]flv1.IPRange{},
	)
	assert.Equal(t, ip, net.ParseIP("fdaa::1"))

	ip, err = GetAvailableIP(
		"fdaa::/16",
		[]flv1.IPRange{},
		[]flv1.IPRange{
			{
				Start: net.ParseIP("fdaa::1"),
				End:   net.ParseIP("fdaa::ffff"),
			},
		},
	)
	assert.Nil(t, err)
	assert.Equal(t, ip, net.ParseIP("fdaa::1:0"))

	ip, err = GetAvailableIP(
		"fdaa::/16",
		[]flv1.IPRange{},
		[]flv1.IPRange{
			{
				Start: net.ParseIP("fdaa::1"),
				End:   net.ParseIP("fdaa:ffff:ffff:ffff:ffff:ffff:ffff:fffe"),
			},
		},
	)
	assert.Nil(t, ip)
	assert.ErrorIs(t, err, ErrNoAvailableIP)
}

func Test_AddIPToRange(t *testing.T) {
	r := AddIPToRange(nil, nil)
	assert.Equal(t, len(r), 0)

	r = AddIPToRange(net.ParseIP("192.168.1.12"), nil)
	assert.Equal(t, r, []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.12"),
			End:   net.ParseIP("192.168.1.12"),
		},
	})

	r = AddIPToRange(net.ParseIP("192.168.1.12"), r)
	assert.Equal(t, r, []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.12"),
			End:   net.ParseIP("192.168.1.12"),
		},
	})

	// Re-add existing ip in range.
	r = AddIPToRange(net.ParseIP("192.168.1.12"), r)
	assert.Equal(t, r, []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.12"),
			End:   net.ParseIP("192.168.1.12"),
		},
	})

	r = AddIPToRange(net.ParseIP("192.168.1.13"), r)
	assert.Equal(t, r, []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.12"),
			End:   net.ParseIP("192.168.1.13"),
		},
	})

	r = AddIPToRange(net.ParseIP("192.168.1.11"), r)
	assert.Equal(t, r, []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.11"),
			End:   net.ParseIP("192.168.1.13"),
		},
	})

	r = AddIPToRange(net.ParseIP("192.168.1.20"), r)
	assert.Equal(t, r, []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.11"),
			End:   net.ParseIP("192.168.1.13"),
		},
		{
			Start: net.ParseIP("192.168.1.20"),
			End:   net.ParseIP("192.168.1.20"),
		},
	})

	r = AddIPToRange(net.ParseIP("192.168.1.1"), r)
	assert.Equal(t, r, []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.1"),
			End:   net.ParseIP("192.168.1.1"),
		},
		{
			Start: net.ParseIP("192.168.1.11"),
			End:   net.ParseIP("192.168.1.13"),
		},
		{
			Start: net.ParseIP("192.168.1.20"),
			End:   net.ParseIP("192.168.1.20"),
		},
	})

	r = AddIPToRange(net.ParseIP("192.168.1.2"), r)
	assert.Equal(t, r, []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.1"),
			End:   net.ParseIP("192.168.1.2"),
		},
		{
			Start: net.ParseIP("192.168.1.11"),
			End:   net.ParseIP("192.168.1.13"),
		},
		{
			Start: net.ParseIP("192.168.1.20"),
			End:   net.ParseIP("192.168.1.20"),
		},
	})

	r = AddIPToRange(net.ParseIP("192.168.1.0"), r)
	assert.Equal(t, r, []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.0"),
			End:   net.ParseIP("192.168.1.2"),
		},
		{
			Start: net.ParseIP("192.168.1.11"),
			End:   net.ParseIP("192.168.1.13"),
		},
		{
			Start: net.ParseIP("192.168.1.20"),
			End:   net.ParseIP("192.168.1.20"),
		},
	})

	r = AddIPToRange(net.ParseIP("fd00::1"), r)
	assert.Equal(t, r, []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.0"),
			End:   net.ParseIP("192.168.1.2"),
		},
		{
			Start: net.ParseIP("192.168.1.11"),
			End:   net.ParseIP("192.168.1.13"),
		},
		{
			Start: net.ParseIP("192.168.1.20"),
			End:   net.ParseIP("192.168.1.20"),
		},
		{
			Start: net.ParseIP("fd00::1"),
			End:   net.ParseIP("fd00::1"),
		},
	})
	r = AddIPToRange(net.ParseIP("fd00::2"), r)
	assert.Equal(t, r, []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.0"),
			End:   net.ParseIP("192.168.1.2"),
		},
		{
			Start: net.ParseIP("192.168.1.11"),
			End:   net.ParseIP("192.168.1.13"),
		},
		{
			Start: net.ParseIP("192.168.1.20"),
			End:   net.ParseIP("192.168.1.20"),
		},
		{
			Start: net.ParseIP("fd00::1"),
			End:   net.ParseIP("fd00::2"),
		},
	})

	r = AddIPToRange(net.ParseIP("fd00::1"), r)
	assert.Equal(t, r, []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.0"),
			End:   net.ParseIP("192.168.1.2"),
		},
		{
			Start: net.ParseIP("192.168.1.11"),
			End:   net.ParseIP("192.168.1.13"),
		},
		{
			Start: net.ParseIP("192.168.1.20"),
			End:   net.ParseIP("192.168.1.20"),
		},
		{
			Start: net.ParseIP("fd00::1"),
			End:   net.ParseIP("fd00::2"),
		},
	})
}

func Test_RemoveIPFromRange(t *testing.T) {
	r := []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.100"),
			End:   net.ParseIP("192.168.1.200"),
		},
	}
	r = RemoveIPFromRange(nil, r)
	assert.Equal(t, r, []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.100"),
			End:   net.ParseIP("192.168.1.200"),
		},
	})

	r = RemoveIPFromRange(net.ParseIP("192.168.1.101"), r)
	assert.Equal(t, r, []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.100"),
			End:   net.ParseIP("192.168.1.100"),
		},
		{
			Start: net.ParseIP("192.168.1.102"),
			End:   net.ParseIP("192.168.1.200"),
		},
	})

	r = RemoveIPFromRange(net.ParseIP("192.168.1.100"), r)
	assert.Equal(t, r, []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.102"),
			End:   net.ParseIP("192.168.1.200"),
		},
	})

	// Re-delete non-existing ip from range.
	r = RemoveIPFromRange(net.ParseIP("192.168.1.100"), r)
	assert.Equal(t, r, []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.102"),
			End:   net.ParseIP("192.168.1.200"),
		},
	})

	r = RemoveIPFromRange(net.ParseIP("192.168.1.102"), r)
	assert.Equal(t, r, []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.103"),
			End:   net.ParseIP("192.168.1.200"),
		},
	})

	r = RemoveIPFromRange(net.ParseIP("192.168.1.200"), r)
	assert.Equal(t, r, []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.103"),
			End:   net.ParseIP("192.168.1.199"),
		},
	})

	r = []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.100"),
			End:   net.ParseIP("192.168.1.100"),
		},
	}
	r = RemoveIPFromRange(net.ParseIP("192.168.1.111"), r)
	assert.Equal(t, r, []flv1.IPRange{
		{
			Start: net.ParseIP("192.168.1.100"),
			End:   net.ParseIP("192.168.1.100"),
		},
	})

	r = RemoveIPFromRange(net.ParseIP("192.168.1.100"), r)
	assert.Equal(t, r, []flv1.IPRange{})

	r = []flv1.IPRange{
		{
			Start: net.ParseIP("fd00::0001"),
			End:   net.ParseIP("fd00::0002"),
		},
	}
	r = RemoveIPFromRange(net.ParseIP("fd00::0002"), r)
	assert.Equal(t, r, []flv1.IPRange{
		{
			Start: net.ParseIP("fd00::0001"),
			End:   net.ParseIP("fd00::0001"),
		},
	})

	r = RemoveIPFromRange(net.ParseIP("fd00::1"), r)
	assert.Equal(t, r, []flv1.IPRange{})
}

func Test_MaskXOR(t *testing.T) {
	mask := MaskXOR(nil)
	assert.Nil(t, mask)

	_, network, err := net.ParseCIDR("10.16.0.0/12")
	assert.Nil(t, err)
	mask = MaskXOR(network.Mask)
	assert.Equal(t, mask, net.IPMask{0x0, 0x0f, 0xff, 0xff})

	_, network, err = net.ParseCIDR("192.168.1.0/24")
	assert.Nil(t, err)
	mask = MaskXOR(network.Mask)
	assert.Equal(t, mask, net.IPMask{0x0, 0x0, 0x0, 0xff})

	_, network, err = net.ParseCIDR("fdaa::/32")
	assert.Nil(t, err)
	mask = MaskXOR(network.Mask)
	expected := make(net.IPMask, 16)
	for i := 0; i < len(expected); i++ {
		if i < 32/8 {
			// set first 32/8 bytes to 0x00
			continue
		}
		// set bytes after 32/8 bytes to 0xff
		expected[i] = 0xff
	}
	assert.Equal(t, mask, expected)
}

func Test_IsBroadCast(t *testing.T) {
	_, network, _ := net.ParseCIDR("192.168.1.0/24")
	assert.True(t, IsBroadCast(net.ParseIP("192.168.1.255"), network))
	assert.False(t, IsBroadCast(net.ParseIP("192.168.1.11"), network))
	assert.False(t, IsBroadCast(net.ParseIP("192.168.1.0"), network))

	_, network, _ = net.ParseCIDR("10.0.0.0/8")
	assert.True(t, IsBroadCast(net.ParseIP("10.255.255.255"), network))
	assert.False(t, IsBroadCast(net.ParseIP("10.0.0.1"), network))
	assert.False(t, IsBroadCast(net.ParseIP("10.0.0.255"), network))
	assert.False(t, IsBroadCast(net.ParseIP("10.0.0.0"), network))

	_, network, _ = net.ParseCIDR("10.0.0.0/12")
	assert.True(t, IsBroadCast(net.ParseIP("10.15.255.255"), network))
	assert.False(t, IsBroadCast(net.ParseIP("10.0.0.1"), network))
	assert.False(t, IsBroadCast(net.ParseIP("10.0.0.255"), network))
	assert.False(t, IsBroadCast(net.ParseIP("10.0.0.0"), network))

	_, network, _ = net.ParseCIDR("fdaa:bbbb:cccc:dddd::/64")
	assert.True(t, IsBroadCast(net.ParseIP("fdaa:bbbb:cccc:dddd:ffff:ffff:ffff:ffff"), network))
	assert.False(t, IsBroadCast(net.ParseIP("fdaa:bbbb:cccc:dddd:0:ffff:ffff:ffff"), network))
	assert.False(t, IsBroadCast(net.ParseIP("fdaa:bbbb:cccc:dddd::ffff"), network))
	assert.False(t, IsBroadCast(net.ParseIP("fdaa:bbbb:cccc:dddd::0"), network))

	_, network, _ = net.ParseCIDR("fdaa::/64")
	assert.True(t, IsBroadCast(net.ParseIP("fdaa::ffff:ffff:ffff:ffff"), network))
	assert.False(t, IsBroadCast(net.ParseIP("fdaa::ffff:ffff:ffff"), network))
	assert.False(t, IsBroadCast(net.ParseIP("fdaa::ffff"), network))
	assert.False(t, IsBroadCast(net.ParseIP("fdaa::"), network))
}

func Test_IsNetwork(t *testing.T) {
	_, network, _ := net.ParseCIDR("192.168.1.0/24")
	assert.True(t, IsNetwork(net.ParseIP("192.168.1.0"), network))
	assert.False(t, IsNetwork(net.ParseIP("192.168.1.11"), network))
	assert.False(t, IsNetwork(net.ParseIP("192.168.1.255"), network))
}

func Test_IsAvailableIP(t *testing.T) {
	_, network, _ := net.ParseCIDR("192.168.1.0/24")
	assert.True(t, IsAvailableIP(net.ParseIP("192.168.1.11"), network))
	assert.False(t, IsAvailableIP(net.ParseIP("192.168.1.0"), network))
	assert.False(t, IsAvailableIP(net.ParseIP("192.168.1.255"), network))

	_, network, _ = net.ParseCIDR("fdaa::/64")
	assert.True(t, IsAvailableIP(net.ParseIP("fdaa::0001"), network))
	assert.False(t, IsAvailableIP(net.ParseIP("fdaa:abab::0001"), network))
	assert.False(t, IsAvailableIP(net.ParseIP("fdaa:abab::0002:0001"), network))
	assert.False(t, IsAvailableIP(net.ParseIP("fdaa:abab::ffff:ffff:ffff:ffff"), network))
	assert.False(t, IsAvailableIP(net.ParseIP("fdaa::ffff:ffff:ffff:ffff"), network))
}

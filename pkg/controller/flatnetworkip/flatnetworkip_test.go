package flatnetworkip

import (
	"net"
	"testing"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/ipcalc"
	"github.com/stretchr/testify/assert"
)

func Test_alreadyAllocateIP(t *testing.T) {
	ip := &flv1.FlatNetworkIP{
		Spec: flv1.IPSpec{
			Addrs: []net.IP{},
		},
		Status: flv1.IPStatus{},
	}
	subnet := &flv1.FlatNetworkSubnet{
		Spec: flv1.SubnetSpec{
			CIDR: "10.128.0.0/16",
		},
		Status: flv1.SubnetStatus{},
	}
	assert.False(t, alreadyAllocateIP(ip, subnet))

	ip.Status.Addr = net.ParseIP("192.168.1.11")
	assert.False(t, alreadyAllocateIP(ip, subnet))

	subnet.Spec.CIDR = "192.168.1.0/24"
	assert.True(t, alreadyAllocateIP(ip, subnet))

	ip.Spec.Addrs = append(ip.Spec.Addrs, net.ParseIP("192.168.1.11"))
	assert.True(t, alreadyAllocateIP(ip, subnet))

	ip.Status.Addr = net.ParseIP("192.168.1.20")
	assert.False(t, alreadyAllocateIP(ip, subnet))

	ip.Spec.Addrs = []net.IP{
		net.ParseIP("192.168.1.11"),
		net.ParseIP("192.168.1.12"),
		net.ParseIP("192.168.1.13"),
		net.ParseIP("192.168.1.14"),
		net.ParseIP("192.168.1.15"),
	}
	ip.Status.Addr = net.ParseIP("192.168.1.13")
	assert.True(t, alreadyAllocateIP(ip, subnet))

	ip.Status.Addr = net.ParseIP("192.168.1.20")
	assert.False(t, alreadyAllocateIP(ip, subnet))
}

func Test_allocateIP(t *testing.T) {
	ip := &flv1.FlatNetworkIP{
		Spec: flv1.IPSpec{
			Addrs: []net.IP{},
		},
		Status: flv1.IPStatus{},
	}
	subnet := &flv1.FlatNetworkSubnet{
		Spec: flv1.SubnetSpec{
			CIDR: "10.128.0.0/16",
		},
		Status: flv1.SubnetStatus{
			UsedIP: []flv1.IPRange{
				{
					// Gateway IP address
					From: net.ParseIP("10.128.0.1"),
					To:   net.ParseIP("10.128.0.1"),
				},
			},
		},
	}
	// Allocate IP in auto mode
	allocatedIP, err := allocateIP(ip, subnet)
	assert.Nil(t, err)
	assert.Equal(t, allocatedIP, net.ParseIP("10.128.0.2"))
	subnet.Status.UsedIP = ipcalc.AddIPToRange(allocatedIP, subnet.Status.UsedIP)

	// Re-allocate IP in auto mode
	allocatedIP, err = allocateIP(ip, subnet)
	assert.Nil(t, err)
	assert.Equal(t, allocatedIP, net.ParseIP("10.128.0.3"))

	// Allocate IP in specific range
	subnet.Spec.Ranges = []flv1.IPRange{
		{
			From: net.IPv4(10, 128, 1, 101),
			To:   net.IPv4(10, 128, 1, 102),
		},
	}
	allocatedIP, err = allocateIP(ip, subnet)
	assert.Nil(t, err)
	assert.Equal(t, allocatedIP, net.ParseIP("10.128.1.101"))
	subnet.Status.UsedIP = ipcalc.AddIPToRange(allocatedIP, subnet.Status.UsedIP)

	// Re-allocate IP in specific range
	subnet.Spec.Ranges = []flv1.IPRange{
		{
			From: net.IPv4(10, 128, 1, 101),
			To:   net.IPv4(10, 128, 1, 102),
		},
	}
	allocatedIP, err = allocateIP(ip, subnet)
	assert.Nil(t, err)
	assert.Equal(t, allocatedIP, net.ParseIP("10.128.1.102"))
	subnet.Status.UsedIP = ipcalc.AddIPToRange(allocatedIP, subnet.Status.UsedIP)

	// Re-allocate IP in specific range, no available IP error expected
	subnet.Spec.Ranges = []flv1.IPRange{
		{
			From: net.IPv4(10, 128, 1, 101),
			To:   net.IPv4(10, 128, 1, 102),
		},
	}
	allocatedIP, err = allocateIP(ip, subnet)
	assert.ErrorIs(t, err, ipcalc.ErrNoAvailableIP)
	assert.Nil(t, allocatedIP)

	// Allocate IP in single specific mode
	subnet.Spec.Ranges = nil
	ip.Spec.Addrs = append(ip.Spec.Addrs, net.ParseIP("10.128.1.200"))
	allocatedIP, err = allocateIP(ip, subnet)
	assert.Nil(t, err)
	assert.Equal(t, allocatedIP, net.ParseIP("10.128.1.200"))
	subnet.Status.UsedIP = ipcalc.AddIPToRange(allocatedIP, subnet.Status.UsedIP)

	// Re-alloc IP in single specific mode
	subnet.Spec.Ranges = nil
	allocatedIP, err = allocateIP(ip, subnet)
	assert.ErrorIs(t, err, ipcalc.ErrNoAvailableIP)
	assert.Nil(t, allocatedIP)

	// Re-alloc IP in single specific mode, but not in subnet ranges.
	subnet.Spec.Ranges = []flv1.IPRange{
		{
			From: net.IPv4(10, 128, 1, 101),
			To:   net.IPv4(10, 128, 1, 102),
		},
	}
	ip.Spec.Addrs = []net.IP{
		net.ParseIP("10.128.1.200"),
	}
	allocatedIP, err = allocateIP(ip, subnet)
	assert.ErrorIs(t, err, ipcalc.ErrNoAvailableIP)
	assert.Nil(t, allocatedIP)

	// Allocate IP in multi specific mode
	subnet.Spec.Ranges = nil
	subnet.Status.UsedIP = nil
	ip.Spec.Addrs = []net.IP{
		net.ParseIP("10.128.1.200"),
		net.ParseIP("10.128.1.201"),
	}
	allocatedIP, err = allocateIP(ip, subnet)
	assert.Nil(t, err)
	assert.Equal(t, allocatedIP, net.ParseIP("10.128.1.200"))
	subnet.Status.UsedIP = ipcalc.AddIPToRange(allocatedIP, subnet.Status.UsedIP)

	// Re-allocate IP in multi specific mode
	allocatedIP, err = allocateIP(ip, subnet)
	assert.Nil(t, err)
	assert.Equal(t, allocatedIP, net.ParseIP("10.128.1.201"))
	subnet.Status.UsedIP = ipcalc.AddIPToRange(allocatedIP, subnet.Status.UsedIP)

	// Re-allocate IP in multi specific mode, but no available IP
	allocatedIP, err = allocateIP(ip, subnet)
	assert.ErrorIs(t, err, ipcalc.ErrNoAvailableIP)
	assert.Nil(t, allocatedIP)

	// Allocate IP in multi specific mode, but not in subnet IP range
	subnet.Spec.Ranges = []flv1.IPRange{
		{
			From: net.IPv4(10, 128, 1, 101),
			To:   net.IPv4(10, 128, 1, 102),
		},
	}
	subnet.Status.UsedIP = nil
	allocatedIP, err = allocateIP(ip, subnet)
	assert.ErrorIs(t, err, ipcalc.ErrNoAvailableIP)
	assert.Nil(t, allocatedIP)
}

func Test_alreadyAllocatedMAC(t *testing.T) {
	ip := &flv1.FlatNetworkIP{
		Spec: flv1.IPSpec{
			MACs: []string{},
		},
		Status: flv1.IPStatus{},
	}
	assert.True(t, alreadyAllocatedMAC(ip))
}

func Test_allocateMAC(t *testing.T) {

}

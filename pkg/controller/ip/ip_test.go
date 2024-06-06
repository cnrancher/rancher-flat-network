package ip

import (
	"net"
	"testing"

	flv1 "github.com/cnrancher/flat-network-operator/pkg/apis/flatnetwork.cattle.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/ipcalc"
	"github.com/stretchr/testify/assert"
)

func Test_alreadyAllocateIP(t *testing.T) {
	ip := &flv1.IP{
		Spec: flv1.IPSpec{
			CIDR: "auto",
		},
		Status: flv1.IPStatus{},
	}
	subnet := &flv1.Subnet{
		Spec: flv1.SubnetSpec{
			CIDR: "10.128.0.0/16",
		},
		Status: flv1.SubnetStatus{},
	}
	assert.False(t, alreadyAllocateIP(ip, subnet))

	ip.Status.Address = net.ParseIP("192.168.1.11")
	assert.False(t, alreadyAllocateIP(ip, subnet))

	subnet.Spec.CIDR = "192.168.1.0/24"
	assert.True(t, alreadyAllocateIP(ip, subnet))

	ip.Spec.CIDR = "192.168.1.11"
	assert.True(t, alreadyAllocateIP(ip, subnet))

	ip.Status.Address = net.ParseIP("192.168.1.20")
	assert.False(t, alreadyAllocateIP(ip, subnet))

	ip.Spec.CIDR = "192.168.1.11-192.168.1.12-192.168.1.13-192.168.1.14-192.168.1.15"
	ip.Status.Address = net.ParseIP("192.168.1.13")
	assert.True(t, alreadyAllocateIP(ip, subnet))

	ip.Status.Address = net.ParseIP("192.168.1.20")
	assert.False(t, alreadyAllocateIP(ip, subnet))

	ip.Spec.CIDR = "192.168.1.22/24"
	ip.Status.Address = net.ParseIP("192.168.1.22")
	assert.True(t, alreadyAllocateIP(ip, subnet))

	ip.Spec.CIDR = "192.168.1.22/24"
	ip.Status.Address = net.ParseIP("192.168.1.1")
	assert.False(t, alreadyAllocateIP(ip, subnet))

	ip.Spec.CIDR = "invalid data"
	ip.Status.Address = nil
	assert.False(t, alreadyAllocateIP(ip, subnet))
}

func Test_allocateIP(t *testing.T) {
	ip := &flv1.IP{
		Spec: flv1.IPSpec{
			CIDR: "auto",
		},
		Status: flv1.IPStatus{},
	}
	subnet := &flv1.Subnet{
		Spec: flv1.SubnetSpec{
			CIDR: "10.128.0.0/16",
		},
		Status: flv1.SubnetStatus{
			UsedIP: []flv1.IPRange{
				{
					// Gateway IP address
					Start: net.ParseIP("10.128.0.1"),
					End:   net.ParseIP("10.128.0.1"),
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
			Start: net.IPv4(10, 128, 1, 101),
			End:   net.IPv4(10, 128, 1, 102),
		},
	}
	allocatedIP, err = allocateIP(ip, subnet)
	assert.Nil(t, err)
	assert.Equal(t, allocatedIP, net.ParseIP("10.128.1.101"))
	subnet.Status.UsedIP = ipcalc.AddIPToRange(allocatedIP, subnet.Status.UsedIP)

	// Re-allocate IP in specific range
	subnet.Spec.Ranges = []flv1.IPRange{
		{
			Start: net.IPv4(10, 128, 1, 101),
			End:   net.IPv4(10, 128, 1, 102),
		},
	}
	allocatedIP, err = allocateIP(ip, subnet)
	assert.Nil(t, err)
	assert.Equal(t, allocatedIP, net.ParseIP("10.128.1.102"))
	subnet.Status.UsedIP = ipcalc.AddIPToRange(allocatedIP, subnet.Status.UsedIP)

	// Re-allocate IP in specific range, no available IP error expected
	subnet.Spec.Ranges = []flv1.IPRange{
		{
			Start: net.IPv4(10, 128, 1, 101),
			End:   net.IPv4(10, 128, 1, 102),
		},
	}
	allocatedIP, err = allocateIP(ip, subnet)
	assert.ErrorIs(t, err, ipcalc.ErrNoAvailableIP)
	assert.Nil(t, allocatedIP)

	// Allocate IP in single specific mode
	subnet.Spec.Ranges = nil
	ip.Spec.CIDR = "10.128.1.200"
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
			Start: net.IPv4(10, 128, 1, 101),
			End:   net.IPv4(10, 128, 1, 102),
		},
	}
	ip.Spec.CIDR = "10.128.1.210"
	allocatedIP, err = allocateIP(ip, subnet)
	assert.ErrorContains(t, err, "subnet range")
	assert.Nil(t, allocatedIP)

	// Allocate IP in multi specific mode
	subnet.Spec.Ranges = nil
	subnet.Status.UsedIP = nil
	ip.Spec.CIDR = "10.128.1.200-10.128.1.201"
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
			Start: net.IPv4(10, 128, 1, 101),
			End:   net.IPv4(10, 128, 1, 102),
		},
	}
	subnet.Status.UsedIP = nil
	allocatedIP, err = allocateIP(ip, subnet)
	assert.ErrorIs(t, err, ipcalc.ErrNoAvailableIP)
	assert.Nil(t, allocatedIP)

	// Allocate IP by CIDR
	subnet.Spec.Ranges = nil
	subnet.Status.UsedIP = nil
	ip.Spec.CIDR = "10.128.1.11/24"
	allocatedIP, err = allocateIP(ip, subnet)
	assert.Nil(t, err)
	assert.Equal(t, allocatedIP, net.ParseIP("10.128.1.11"))
	subnet.Status.UsedIP = ipcalc.AddIPToRange(allocatedIP, subnet.Status.UsedIP)

	// Re-allocate IP by CIDR
	ip.Spec.CIDR = "10.128.1.11/24"
	allocatedIP, err = allocateIP(ip, subnet)
	assert.ErrorIs(t, err, ipcalc.ErrNoAvailableIP)
	assert.Nil(t, allocatedIP)

	// Allocate IP by CIDR, but not in subnet ranges
	subnet.Spec.Ranges = []flv1.IPRange{
		{
			Start: net.IPv4(10, 128, 1, 101),
			End:   net.IPv4(10, 128, 1, 102),
		},
	}
	subnet.Status.UsedIP = nil
	ip.Spec.CIDR = "10.128.1.11/24"
	allocatedIP, err = allocateIP(ip, subnet)
	assert.ErrorIs(t, err, ipcalc.ErrNoAvailableIP)
	assert.Nil(t, allocatedIP)
}

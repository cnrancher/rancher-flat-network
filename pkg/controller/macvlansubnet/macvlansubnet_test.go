package macvlansubnet

import (
	"net"
	"testing"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	"github.com/stretchr/testify/assert"
)

func Test_ip2UsedRanges(t *testing.T) {
	usedIPs := ip2UsedRanges(nil)
	assert.Equal(t, len(usedIPs), 0)

	macvlanIPs := []*macvlanv1.MacvlanIP{
		{
			Spec: macvlanv1.MacvlanIPSpec{},
			Status: macvlanv1.MacvlanIPStatus{
				IP: net.ParseIP("10.1.2.3"),
			},
		},
	}
	usedIPs = ip2UsedRanges(macvlanIPs)
	assert.Equal(t, usedIPs, []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("10.1.2.3"),
			RangeEnd:   net.ParseIP("10.1.2.3"),
		},
	})

	macvlanIPs = append(macvlanIPs, &macvlanv1.MacvlanIP{
		Spec: macvlanv1.MacvlanIPSpec{},
		Status: macvlanv1.MacvlanIPStatus{
			IP: net.ParseIP("10.1.2.4"),
		},
	})
	usedIPs = ip2UsedRanges(macvlanIPs)
	assert.Equal(t, usedIPs, []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("10.1.2.3"),
			RangeEnd:   net.ParseIP("10.1.2.4"),
		},
	})

	macvlanIPs = append(macvlanIPs, &macvlanv1.MacvlanIP{
		Spec: macvlanv1.MacvlanIPSpec{},
		Status: macvlanv1.MacvlanIPStatus{
			IP: net.ParseIP("10.10.1.1"),
		},
	})
	usedIPs = ip2UsedRanges(macvlanIPs)
	assert.Equal(t, usedIPs, []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("10.1.2.3"),
			RangeEnd:   net.ParseIP("10.1.2.4"),
		},
		{
			RangeStart: net.ParseIP("10.10.1.1"),
			RangeEnd:   net.ParseIP("10.10.1.1"),
		},
	})

	macvlanIPs = append(macvlanIPs, &macvlanv1.MacvlanIP{
		Spec: macvlanv1.MacvlanIPSpec{},
		Status: macvlanv1.MacvlanIPStatus{
			IP: net.ParseIP("10.10.1.2"),
		},
	})
	usedIPs = ip2UsedRanges(macvlanIPs)
	assert.Equal(t, usedIPs, []macvlanv1.IPRange{
		{
			RangeStart: net.ParseIP("10.1.2.3"),
			RangeEnd:   net.ParseIP("10.1.2.4"),
		},
		{
			RangeStart: net.ParseIP("10.10.1.1"),
			RangeEnd:   net.ParseIP("10.10.1.2"),
		},
	})
}

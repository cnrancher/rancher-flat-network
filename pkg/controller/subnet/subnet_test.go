package subnet

import (
	"net"
	"testing"

	flv1 "github.com/cnrancher/flat-network-operator/pkg/apis/flatnetwork.cattle.io/v1"
	"github.com/stretchr/testify/assert"
)

func Test_ip2UsedRanges(t *testing.T) {
	usedIPs := ip2UsedRanges(nil)
	assert.Equal(t, len(usedIPs), 0)

	IPs := []*flv1.IP{
		{
			Spec: flv1.IPSpec{},
			Status: flv1.IPStatus{
				Address: net.ParseIP("10.1.2.3"),
			},
		},
	}
	usedIPs = ip2UsedRanges(IPs)
	assert.Equal(t, usedIPs, []flv1.IPRange{
		{
			Start: net.ParseIP("10.1.2.3"),
			End:   net.ParseIP("10.1.2.3"),
		},
	})

	IPs = append(IPs, &flv1.IP{
		Spec: flv1.IPSpec{},
		Status: flv1.IPStatus{
			Address: net.ParseIP("10.1.2.4"),
		},
	})
	usedIPs = ip2UsedRanges(IPs)
	assert.Equal(t, usedIPs, []flv1.IPRange{
		{
			Start: net.ParseIP("10.1.2.3"),
			End:   net.ParseIP("10.1.2.4"),
		},
	})

	IPs = append(IPs, &flv1.IP{
		Spec: flv1.IPSpec{},
		Status: flv1.IPStatus{
			Address: net.ParseIP("10.10.1.1"),
		},
	})
	usedIPs = ip2UsedRanges(IPs)
	assert.Equal(t, usedIPs, []flv1.IPRange{
		{
			Start: net.ParseIP("10.1.2.3"),
			End:   net.ParseIP("10.1.2.4"),
		},
		{
			Start: net.ParseIP("10.10.1.1"),
			End:   net.ParseIP("10.10.1.1"),
		},
	})

	IPs = append(IPs, &flv1.IP{
		Spec: flv1.IPSpec{},
		Status: flv1.IPStatus{
			Address: net.ParseIP("10.10.1.2"),
		},
	})
	usedIPs = ip2UsedRanges(IPs)
	assert.Equal(t, usedIPs, []flv1.IPRange{
		{
			Start: net.ParseIP("10.1.2.3"),
			End:   net.ParseIP("10.1.2.4"),
		},
		{
			Start: net.ParseIP("10.10.1.1"),
			End:   net.ParseIP("10.10.1.2"),
		},
	})
}

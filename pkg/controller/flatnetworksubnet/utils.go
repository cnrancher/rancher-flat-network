package flatnetworksubnet

import (
	"bytes"
	"net"

	flv1 "github.com/cnrancher/flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
)

func isValidRanges(subnet *flv1.FlatNetworkSubnet) bool {
	if len(subnet.Spec.Ranges) == 0 {
		return true
	}

	_, network, err := net.ParseCIDR(subnet.Spec.CIDR)
	if err != nil {
		return false
	}

	for _, r := range subnet.Spec.Ranges {
		s1 := r.Start.To16()
		s2 := r.End.To16()
		if s1 == nil || s2 == nil {
			return false
		}
		if !network.Contains(s1) || !network.Contains(s2) {
			return false
		}
		if bytes.Compare(s1, s2) > 0 {
			return false
		}
	}
	return true
}

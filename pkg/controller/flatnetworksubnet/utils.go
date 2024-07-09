package flatnetworksubnet

import (
	"bytes"
	"net"

	flv1 "github.com/cnrancher/rancher-flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/sirupsen/logrus"
)

func isValidRanges(subnet *flv1.FlatNetworkSubnet) bool {
	if len(subnet.Spec.Ranges) == 0 {
		return true
	}

	_, network, err := net.ParseCIDR(subnet.Spec.CIDR)
	if err != nil {
		logrus.WithFields(fieldsSubnet(subnet)).
			Warnf("invalid subnet CIDR [%v]", subnet.Spec.CIDR)
		return false
	}

	for _, r := range subnet.Spec.Ranges {
		s1 := r.From.To16()
		s2 := r.To.To16()
		if s1 == nil || s2 == nil {
			return false
		}
		if !network.Contains(s1) || !network.Contains(s2) {
			logrus.WithFields(fieldsSubnet(subnet)).
				Warnf("invalid subnet range: start/end not inside network [%v]",
					subnet.Spec.CIDR)
			return false
		}
		if bytes.Compare(s1, s2) > 0 {
			logrus.WithFields(fieldsSubnet(subnet)).
				Warnf("invalid subnet range: start should <= end")
			return false
		}
	}
	return true
}

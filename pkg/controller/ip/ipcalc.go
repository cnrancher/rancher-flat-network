package ip

import (
	"fmt"
	"net"

	flv1 "github.com/cnrancher/flat-network-operator/pkg/apis/flatnetwork.cattle.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/ipcalc"
)

// alreadyAllocateIP checks if the flat-network IP already allocated
// the expected IP address.
func alreadyAllocateIP(
	ip *flv1.IP, subnet *flv1.Subnet,
) bool {
	if len(ip.Status.Addr) == 0 {
		return false
	}

	switch len(ip.Spec.Addrs) {
	case 0:
		// Auto mode
		// Check if the IP address inside the subnet network.
		_, network, err := net.ParseCIDR(subnet.Spec.CIDR)
		if err != nil {
			return false
		}
		return network.Contains(ip.Status.Addr)
	default:
		// Specific mode.
		for _, addr := range ip.Spec.Addrs {
			a := addr.To16()
			if a == nil {
				continue
			}
			if a.Equal(ip.Status.Addr) {
				return true
			}
		}
		return false
	}
}

func allocateIP(
	ip *flv1.IP, subnet *flv1.Subnet,
) (net.IP, error) {
	if alreadyAllocateIP(ip, subnet) {
		return ip.Status.Addr, nil
	}

	switch len(ip.Spec.Addrs) {
	case 0:
		// Auto mode.
		a, err := ipcalc.GetAvailableIP(subnet.Spec.CIDR, subnet.Spec.Ranges, subnet.Status.UsedIP)
		if err != nil {
			return nil, err
		}
		return a, nil
	default:
		// Use custom IP from addresses.
		for _, v := range ip.Spec.Addrs {
			a := v.To16()
			if len(a) == 0 {
				return nil, fmt.Errorf("allocateIP: invalid IP [%v] in addrs", v)
			}
			if len(subnet.Spec.Ranges) != 0 && !ipcalc.IPInRanges(a, subnet.Spec.Ranges) {
				continue
			}
			if !ipcalc.IPInRanges(a, subnet.Status.UsedIP) {
				return a, nil
			}
		}
		return nil, fmt.Errorf("allocateIP: no available IP address from addrs %v: %w",
			ip.Spec.Addrs, ipcalc.ErrNoAvailableIP)
	}
}

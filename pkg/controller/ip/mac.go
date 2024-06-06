package ip

import (
	"bytes"
	"fmt"
	"net"
	"slices"

	flv1 "github.com/cnrancher/flat-network-operator/pkg/apis/flatnetwork.cattle.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/ipcalc"
)

func alreadyAllocatedMAC(ip *flv1.IP) bool {
	if len(ip.Status.MAC) == 0 {
		return len(ip.Spec.MACs) == 0
	}
	allocatedMAC := ip.Status.MAC.String()
	for _, m := range ip.Spec.MACs {
		if m.String() == allocatedMAC {
			return true
		}
	}
	return false
}

func allocateMAC(
	ip *flv1.IP, subnet *flv1.Subnet,
) (net.HardwareAddr, error) {
	if len(ip.Spec.MACs) == 0 {
		// User does not specify custom MAC address, return directly.
		return nil, nil
	}
	if alreadyAllocatedMAC(ip) {
		return ip.Status.MAC, nil
	}

	// Use custom Mac from multiple addresses.
	for _, m := range ip.Spec.MACs {
		_, ok := slices.BinarySearchFunc(subnet.Status.UsedMac, m, func(a, b net.HardwareAddr) int {
			return bytes.Compare(a, b)
		})
		if !ok {
			// Select the unused mac address from multi-mac addresses.
			return m, nil
		}
	}
	return nil, fmt.Errorf("allocateMAC: no available MAC address from MACs %v: %w",
		ip.Spec.MACs, ipcalc.ErrNoAvailableMac)
}

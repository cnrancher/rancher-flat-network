package flatnetworkip

import (
	"fmt"
	"slices"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/ipcalc"
)

func alreadyAllocatedMAC(ip *flv1.FlatNetworkIP) bool {
	// If user does not specify custom MAC address, return true directly.
	if len(ip.Spec.MACs) == 0 {
		return true
	}

	// If CNI not allocate MAC address for pod, check user specified custom
	// mac addresses or not.
	if len(ip.Status.MAC) == 0 {
		return len(ip.Spec.MACs) == 0
	}
	allocatedMAC := ip.Status.MAC
	for _, m := range ip.Spec.MACs {
		if m == allocatedMAC {
			return true
		}
	}
	return false
}

func allocateMAC(
	ip *flv1.FlatNetworkIP, subnet *flv1.FlatNetworkSubnet,
) (string, error) {
	if len(ip.Spec.MACs) == 0 {
		// User does not specify custom MAC address, return directly.
		return "", nil
	}
	if alreadyAllocatedMAC(ip) {
		return ip.Status.MAC, nil
	}

	// Use custom MAC from multiple addresses.
	for _, m := range ip.Spec.MACs {
		_, ok := slices.BinarySearch(subnet.Status.UsedMAC, m)
		if !ok {
			// Select the unused mac address from multi-mac addresses.
			return m, nil
		}
	}
	return "", fmt.Errorf("allocateMAC: no available MAC address from MACs %v: %w",
		ip.Spec.MACs, ipcalc.ErrNoAvailableMac)
}

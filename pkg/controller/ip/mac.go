package ip

import (
	"bytes"
	"fmt"
	"net"
	"slices"
	"strings"

	flv1 "github.com/cnrancher/flat-network-operator/pkg/apis/flatnetwork.cattle.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/utils"
)

func alreadyAllocatedMAC(ip *flv1.IP) bool {
	if len(ip.Status.MAC) == 0 {
		return ip.Spec.MAC == ""
	}
	allocatedMAC := ip.Status.MAC.String()
	switch {
	case utils.IsSingleMAC(ip.Spec.MAC):
		// Use single custom MAC address.
		mac, _ := net.ParseMAC(ip.Spec.MAC)
		if mac.String() == allocatedMAC {
			return true
		}
	case utils.IsMultipleMAC(ip.Spec.MAC):
		spec := strings.Split(ip.Spec.MAC, "-")
		for _, v := range spec {
			mac, _ := net.ParseMAC(v)
			if mac.String() == allocatedMAC {
				return true
			}
		}
	}
	return false
}

func allocateMAC(
	ip *flv1.IP, subnet *flv1.Subnet,
) (net.HardwareAddr, error) {
	if ip.Spec.MAC == "" {
		// User does not specify custom MAC address, return directly.
		return nil, nil
	}
	if alreadyAllocatedMAC(ip) {
		return ip.Status.MAC, nil
	}

	switch {
	case utils.IsSingleMAC(ip.Spec.MAC):
		// Use single custom MAC address.
		mac, err := net.ParseMAC(ip.Spec.MAC)
		if err != nil {
			return nil, fmt.Errorf("allocateMAC: failed to parse MAC [%v]: %w",
				ip.Spec.MAC, err)
		}
		_, ok := slices.BinarySearchFunc(subnet.Status.UsedMac, mac, func(a, b net.HardwareAddr) int {
			return bytes.Compare(a, b)
		})
		if ok {
			return nil, fmt.Errorf("allocateMAC: MAC [%v] already in use", ip.Spec.MAC)
		}
		return mac, nil
	case utils.IsMultipleMAC(ip.Spec.MAC):
		// Use custom Mac from multiple addresses.
		spec := strings.Split(ip.Spec.MAC, "-")
		for _, v := range spec {
			mac, err := net.ParseMAC(v)
			if err != nil {
				return nil, fmt.Errorf("allocateMAC: invalid MAC [%v]: %w", mac, err)
			}
			_, ok := slices.BinarySearchFunc(subnet.Status.UsedMac, mac, func(a, b net.HardwareAddr) int {
				return bytes.Compare(a, b)
			})
			if !ok {
				// Select the unused mac address from multi-mac addresses.
				return mac, nil
			}
		}
		return nil, fmt.Errorf("allocateMAC: no available MAC address from [%v]",
			ip.Spec.MAC)
	}
	return nil, nil
}

package macvlanip

import (
	"fmt"
	"net"
	"strings"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/ipcalc"
	"github.com/cnrancher/flat-network-operator/pkg/utils"
)

func alreadyAllocateIP(ip *macvlanv1.MacvlanIP) bool {
	if len(ip.Status.IP) == 0 {
		return false
	}

	switch {
	case ip.Spec.CIDR == "auto":
		return true
	case utils.IsSingleIP(ip.Spec.CIDR):
		a := net.ParseIP(ip.Spec.CIDR).To16()
		if a == nil {
			return false
		}
		return a.Equal(ip.Status.IP)
	case utils.IsMultipleIP(ip.Spec.CIDR):
		spec := strings.Split(ip.Spec.CIDR, "-")
		for _, v := range spec {
			a := net.ParseIP(v).To16()
			if a == nil {
				continue
			}
			if a.Equal(ip.Status.IP) {
				return true
			}
		}
		return false
	case strings.Contains(ip.Spec.CIDR, "/"):
		s := strings.Split(ip.Spec.CIDR, "/")[0]
		a := net.ParseIP(s).To16()
		if len(a) == 0 {
			return false
		}
		return a.Equal(ip.Status.IP)
	}
	return false
}

func (h *handler) allocateIP(
	ip *macvlanv1.MacvlanIP, subnet *macvlanv1.MacvlanSubnet,
) (net.IP, error) {
	if alreadyAllocateIP(ip) {
		return ip.Status.IP, nil
	}

	switch {
	case ip.Spec.CIDR == "auto":
		a, err := ipcalc.GetAvailableIP(subnet.Spec.CIDR, subnet.Spec.Ranges, subnet.Status.UsedIP)
		if err != nil {
			return nil, err
		}
		return a, nil
	case utils.IsSingleIP(ip.Spec.CIDR):
		a := net.ParseIP(ip.Spec.CIDR).To16()
		if len(a) == 0 {
			return nil, fmt.Errorf("allocateIP: invalid IP address [%v]", ip.Spec.CIDR)
		}
		if ipcalc.IPInRanges(a, subnet.Status.UsedIP) {
			return a, fmt.Errorf("allocateIP: IP [%v] already in use", ip.Spec.CIDR)
		}
		return a, nil
	case utils.IsMultipleIP(ip.Spec.CIDR):
		// Use custom IP from multiple addresses.
		spec := strings.Split(ip.Spec.CIDR, "-")
		for _, v := range spec {
			a := net.ParseIP(v).To16()
			if len(a) == 0 {
				return nil, fmt.Errorf("allocateIP: invalid IP [%v]", ip.Spec.CIDR)
			}
			if !ipcalc.IPInRanges(a, subnet.Status.UsedIP) {
				return a, nil
			}
		}
		return nil, fmt.Errorf("allocateIP: no available IP address from [%v]",
			ip.Spec.CIDR)
	case strings.Contains(ip.Spec.CIDR, "/"):
		s := strings.Split(ip.Spec.CIDR, "/")[0]
		a := net.ParseIP(s).To16()
		if len(a) == 0 {
			return nil, fmt.Errorf("allocateIP: invalid IP [%v]", ip.Spec.CIDR)
		}
		if ipcalc.IPInRanges(a, subnet.Status.UsedIP) {
			return a, fmt.Errorf("allocateIP: IP [%v] already in use", ip.Spec.CIDR)
		}
		return a, nil
	}

	return nil, fmt.Errorf("allocateIP: invalid CIDR [%v] in macvlanIP [%v/%v]",
		ip.Spec.CIDR, ip.Namespace, ip.Name)
}

package pod

import (
	"bytes"
	"fmt"
	"net"
	"strings"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	corev1 "k8s.io/api/core/v1"
)

func ipInRanges(ip net.IP, ipRanges []macvlanv1.IPRange) bool {
	if len(ipRanges) == 0 {
		return true
	}
	for _, ipRange := range ipRanges {
		if ipRange.RangeStart == nil || ipRange.RangeEnd == nil {
			continue
		}
		if bytes.Compare(ip, ipRange.RangeStart) >= 0 && bytes.Compare(ip, ipRange.RangeEnd) <= 0 {
			return true
		}
	}
	return false
}

func ipNotUsed(ip net.IP, usedRanges []macvlanv1.IPRange) bool {
	if len(usedRanges) == 0 {
		return true
	}
	for _, usedRange := range usedRanges {
		if usedRange.RangeStart == nil || usedRange.RangeEnd == nil {
			continue
		}
		if bytes.Compare(ip, usedRange.RangeStart) >= 0 && bytes.Compare(ip, usedRange.RangeEnd) <= 0 {
			return false
		}
	}
	return true
}

func getAvailableIP(cidr string, ipRanges []macvlanv1.IPRange, usedIPs []macvlanv1.IPRange) (net.IP, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); incip(ip) {
		if ip.To4() != nil {
			// remove network address and broadcast address
			if ip[3] == 0x00 || ip[3] == 0xff {
				continue
			}
		}
		ip = net.IPv4(ip[0], ip[1], ip[2], ip[3])
		if ipInRanges(ip, ipRanges) && ipNotUsed(ip, usedIPs) {
			return ip, nil
		}
	}
	return nil, fmt.Errorf("TODO: no available ip")
}

// http://play.golang.org/p/m8TNTtygK0
func incip(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func convertIPtoCIDR(ip net.IP, cidr string) string {
	nets := strings.Split(cidr, "/")
	suffix := ""
	if len(nets) == 2 {
		suffix = nets[1]
	}
	return ip.String() + "/" + suffix
}

func (h *handler) allocateAutoIP(
	pod *corev1.Pod, subnet *macvlanv1.MacvlanSubnet, annotationMac string,
) (allocatedIP net.IP, CIDR string, mac string, err error) {
	ip, err := getAvailableIP(subnet.Spec.CIDR, subnet.Spec.Ranges, subnet.Status.AllocatedIPs)
	if err != nil {
		return nil, "", "", err
	}
	cidr := convertIPtoCIDR(ip, subnet.Spec.CIDR)

	// empty annotation mac address
	if annotationMac == "" {
		return ip, cidr, "", nil
	}

	// TODO:
	return ip, cidr, "", nil
}

func (h *handler) allocateSingleIP(
	pod *corev1.Pod, subnet *macvlanv1.MacvlanSubnet, annotationIP, annotationMac string,
) (allocatedIP net.IP, CIDR string, mac string, err error) {
	return
}

func (h *handler) allocateMultipleIP(
	pod *corev1.Pod, subnet *macvlanv1.MacvlanSubnet, annotationIP, annotationMac string,
) (allocatedIP net.IP, CIDR string, mac string, err error) {
	return
}

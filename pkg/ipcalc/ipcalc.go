package ipcalc

import (
	"bytes"
	"errors"
	"net"
	"slices"
	"strings"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
)

var (
	ErrNoAvailableIP  = errors.New("no available IP address")
	ErrNoAvailableMac = errors.New("no available MAC address")
)

// IPIncrease increases the provided IP address.
// http://play.golang.org/p/m8TNTtygK0
func IPIncrease(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// IPIncrease decreases the provided IP address.
func IPDecrease(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]--
		if ip[j] != 0xFF {
			break
		}
	}
}

// GetDefaultGateway returns `192.168.1.1` from CIDR `192.168.1.0/24`.
func GetDefaultGateway(cidr string) (net.IP, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	ip = ip.Mask(ipnet.Mask)
	IPIncrease(ip)
	return ip, nil
}

// IPInRanges checks whether the address is in the IPRange.
func IPInRanges(ip net.IP, ipRanges []macvlanv1.IPRange) bool {
	if len(ipRanges) == 0 {
		return false
	}
	a := ip.To16() // Ensure 16bytes length.
	if a == nil {
		return false
	}

	for _, ipRange := range ipRanges {
		if ipRange.RangeStart == nil || ipRange.RangeEnd == nil {
			continue
		}
		start := ipRange.RangeStart.To16()
		end := ipRange.RangeEnd.To16()
		if bytes.Compare(a, start) >= 0 && bytes.Compare(a, end) <= 0 {
			return true
		}
	}
	return false
}

// IPNotUsed checks whether the IP address is used or not.
func IPNotUsed(ip net.IP, usedRanges []macvlanv1.IPRange) bool {
	if len(usedRanges) == 0 {
		return true
	}
	a := ip.To16() // Ensure 16bytes length.
	if a == nil {
		return false
	}

	for _, usedRange := range usedRanges {
		if usedRange.RangeStart == nil || usedRange.RangeEnd == nil {
			continue
		}
		start := usedRange.RangeStart.To16()
		end := usedRange.RangeEnd.To16()
		if bytes.Compare(a, start) >= 0 && bytes.Compare(a, end) <= 0 {
			return false
		}
	}
	return true
}

// GetAvailableIP gets a **16bytes length** IP address by CIDR and IPRange.
// ErrNoAvailableIP error will be returned if no IP address resource available.
func GetAvailableIP(
	cidr string, ipRanges []macvlanv1.IPRange, usedIPs []macvlanv1.IPRange,
) (net.IP, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	// Iterate to get an available IP address.
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); IPIncrease(ip) {
		l := len(ip)
		if l != net.IPv4len && l != net.IPv6len {
			continue
		}
		// Remove network address and broadcast address.
		// FIXME: should check the CIDR length instead of using 0xFF here.
		if ip[l-1] == 0x00 || ip[l-1] == 0xff {
			continue
		}
		if len(ipRanges) == 0 && !IPInRanges(ip, usedIPs) {
			return ip, nil
		}
		if IPInRanges(ip, ipRanges) && !IPInRanges(ip, usedIPs) {
			return ip, nil
		}
	}
	return nil, ErrNoAvailableIP
}

// AddCIDRSuffix adds CIDR string suffix to IP address.
//
//	ip: `192.168.1.10` CIDR: `192.168.1.0/24` return `192.168.1.10/24`
//	ip: `192.168.1.10` CIDR: `empty string` return `192.168.1.10/32`
func AddCIDRSuffix(ip net.IP, CIDR string) string {
	s := strings.Split(CIDR, "/")
	if len(s) != 2 {
		return ip.String() + "/32"
	}
	return ip.String() + "/" + s[1]
}

// AddIPToRange adds an IP address to IPRange.
func AddIPToRange(ip net.IP, ipRanges []macvlanv1.IPRange) []macvlanv1.IPRange {
	if ip == nil {
		return ipRanges
	}
	if len(ipRanges) == 0 {
		ipRanges = []macvlanv1.IPRange{}
	}
	if IPInRanges(ip, ipRanges) {
		// Skip if ip already in ranges.
		return ipRanges
	}
	for i := range ipRanges {
		var s1 net.IP = bytes.Clone(ipRanges[i].RangeStart)
		var s2 net.IP = bytes.Clone(ipRanges[i].RangeEnd)
		IPDecrease(s1)
		IPIncrease(s2)
		if ip.Equal(s1) {
			ipRanges[i].RangeStart = s1
			return ipRanges
		}
		if ip.Equal(s2) {
			ipRanges[i].RangeEnd = s2
			return ipRanges
		}
	}
	ipRanges = append(ipRanges, macvlanv1.IPRange{
		RangeStart: bytes.Clone(ip),
		RangeEnd:   bytes.Clone(ip),
	})
	slices.SortFunc(ipRanges, func(a, b macvlanv1.IPRange) int {
		return bytes.Compare(a.RangeStart, b.RangeStart)
	})
	return ipRanges
}

// RemoveIPFromRange removes an IP address from IPRange.
func RemoveIPFromRange(ip net.IP, ipRanges []macvlanv1.IPRange) []macvlanv1.IPRange {
	ip = ip.To16() // ensure 16 bytes length.
	if ip == nil {
		return ipRanges
	}
	if !IPInRanges(ip, ipRanges) {
		// Skip if ip not in ranges.
		return ipRanges
	}
	newRanges := []macvlanv1.IPRange{}
	for _, r := range ipRanges {
		start := r.RangeStart.To16()
		end := r.RangeEnd.To16()
		a := bytes.Compare(start, ip)
		b := bytes.Compare(end, ip)
		switch {
		case a < 0 && b > 0:
			var s1 net.IP = bytes.Clone(ip)
			var s2 net.IP = bytes.Clone(ip)
			IPDecrease(s1)
			IPIncrease(s2)
			newRanges = append(newRanges, macvlanv1.IPRange{
				RangeStart: start,
				RangeEnd:   s1,
			})
			newRanges = append(newRanges, macvlanv1.IPRange{
				RangeStart: s2,
				RangeEnd:   end,
			})
		case a == 0 && b > 0:
			var s1 net.IP = bytes.Clone(ip)
			IPIncrease(s1)
			newRanges = append(newRanges, macvlanv1.IPRange{
				RangeStart: s1,
				RangeEnd:   end,
			})
		case a < 0 && b == 0:
			var s2 net.IP = bytes.Clone(ip)
			IPDecrease(s2)
			newRanges = append(newRanges, macvlanv1.IPRange{
				RangeStart: start,
				RangeEnd:   s2,
			})
		case a == 0 && b == 0:
		default:
			newRanges = append(newRanges, macvlanv1.IPRange{
				RangeStart: start,
				RangeEnd:   end,
			})
		}
	}
	slices.SortFunc(newRanges, func(a, b macvlanv1.IPRange) int {
		return bytes.Compare(a.RangeStart, b.RangeStart)
	})
	return newRanges
}

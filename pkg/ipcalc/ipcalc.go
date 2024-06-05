package ipcalc

import (
	"bytes"
	"errors"
	"net"
	"slices"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
)

var (
	ErrNoAvailableIP  = errors.New("no available IP address")
	ErrNoAvailableMac = errors.New("no available MAC address")
)

// IPIncrease increases the provided IP address.
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

// GetDefaultGateway returns **16 bytes** IP address by CIDR.
//
// Example:
//
//	CIDR `192.168.1.0/24` -> return `192.168.1.1`.
func GetDefaultGateway(CIDR string) (net.IP, error) {
	ip, ipnet, err := net.ParseCIDR(CIDR)
	if err != nil {
		return nil, err
	}

	ip = ip.Mask(ipnet.Mask)
	IPIncrease(ip)
	return ip.To16(), nil
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

// GetAvailableIP gets a **16bytes length** IP address by CIDR and IPRange.
// ErrNoAvailableIP error will be returned if no IP address resource available.
func GetAvailableIP(
	cidr string, ipRanges []macvlanv1.IPRange, usedIPs []macvlanv1.IPRange,
) (net.IP, error) {
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	// Iterate to get an available IP address from defined IPRanges.
	if len(ipRanges) > 0 {
		for _, r := range ipRanges {
			start := slices.Clone(r.RangeStart.To16())
			end := r.RangeEnd.To16()
			// Skip the already used range to increase performance.
			if len(usedIPs) != 0 {
				for _, u := range usedIPs {
					if u.RangeStart.Equal(start) {
						start = slices.Clone(u.RangeEnd.To16())
					}
				}
			}
			if len(start) == 0 || len(end) == 0 {
				continue
			}

			for ip := start; bytes.Compare(start, end) <= 0; IPIncrease(ip) {
				ip = ip.To16() // Ensure IP in 16 bytes
				// Remove network address and broadcast address.
				if len(ip) == 0 || !IsAvailableIP(ip, network) {
					continue
				}
				if !IPInRanges(ip, usedIPs) {
					return ip, nil
				}
			}
		}
		return nil, ErrNoAvailableIP
	}

	// No custom IPRange defined, iterate to get available IP address.
	start := net.IP(bytes.Clone(ip.Mask(network.Mask)))
	IPIncrease(start)
	// Skip the already used range to increase performance.
	if len(usedIPs) != 0 {
		for _, u := range usedIPs {
			if u.RangeStart.Equal(start) {
				start = slices.Clone(u.RangeEnd.To16())
			}
		}
	}
	for ip = start; network.Contains(ip); IPIncrease(ip) {
		ip = ip.To16() // Ensure IP in 16 bytes
		// Remove network address and broadcast address.
		if len(ip) == 0 || !IsAvailableIP(ip, network) {
			continue
		}
		if !IPInRanges(ip, usedIPs) {
			return ip, nil
		}
	}

	return nil, ErrNoAvailableIP
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

// MaskXOR returns the XOR-ed network mask.
//
// Example:
//
//	input '255.255.240.0' -> return '0.0.15.255'
func MaskXOR(mask net.IPMask) net.IPMask {
	if len(mask) == 0 {
		return nil
	}
	mask = slices.Clone(mask)
	for i := 0; i < len(mask); i++ {
		mask[i] ^= 255
	}
	return mask
}

// IsBroadCast checks if the IP address is the broadcast address of the network.
func IsBroadCast(ip net.IP, network *net.IPNet) bool {
	if len(ip) == 0 || network == nil || len(network.IP) == 0 || len(network.Mask) == 0 {
		return false
	}
	mask := MaskXOR(network.Mask)
	if network.IP.To4() != nil {
		// IPv4
		a := slices.Clone(network.IP.To4())
		for i := 0; i < len(a) && i < len(mask); i++ {
			a[i] += mask[i]
		}
		return ip.Equal(a)
	} else if network.IP.To16() != nil {
		// IPv6
		a := slices.Clone(network.IP.To16())
		for i := 0; i < len(a) && i < len(mask); i++ {
			a[i] += mask[i]
		}
		return ip.Equal(a)
	}

	return false
}

// IsNetwork checks if the IP address is the network address itself.
func IsNetwork(ip net.IP, network *net.IPNet) bool {
	if len(ip) == 0 || network == nil || len(network.IP) == 0 || len(network.Mask) == 0 {
		return false
	}

	return ip.Equal(network.IP)
}

func IPInNetwork(ip net.IP, network *net.IPNet) bool {
	if len(ip) == 0 || network == nil || len(network.IP) == 0 || len(network.Mask) == 0 {
		return false
	}
	networkAddr := ip.Mask(network.Mask)
	return networkAddr.Equal(network.IP)
}

// IsAvailableIP returns true if the provided IP address is not a broadcast
// and not a network address.
func IsAvailableIP(ip net.IP, network *net.IPNet) bool {
	if len(ip) == 0 || network == nil || len(network.IP) == 0 || len(network.Mask) == 0 {
		return false
	}
	// IP address should inside the provided network.
	if !IPInNetwork(ip, network) {
		return false
	}

	return !(IsBroadCast(ip, network) || IsNetwork(ip, network))
}

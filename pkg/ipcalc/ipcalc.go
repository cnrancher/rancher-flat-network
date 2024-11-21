package ipcalc

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"slices"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
)

var (
	ErrNoAvailableIP    = errors.New("no available IP address")
	ErrNoAvailableMac   = errors.New("no available MAC address")
	ErrNetworkConflict  = errors.New("network CIDR conflict")
	ErrIPRangesConflict = errors.New("ip ranges conflict")
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
func IPInRanges(ip net.IP, ipRanges []flv1.IPRange) bool {
	if len(ipRanges) == 0 {
		return false
	}
	a := ip.To16() // Ensure 16bytes length.
	if a == nil {
		return false
	}

	for _, ipRange := range ipRanges {
		if ipRange.From == nil || ipRange.To == nil {
			continue
		}
		start := ipRange.From.To16()
		end := ipRange.To.To16()
		if bytes.Compare(a, start) >= 0 && bytes.Compare(a, end) <= 0 {
			return true
		}
	}
	return false
}

// GetAvailableIP gets a **16bytes length** IP address by CIDR and IPRange.
// ErrNoAvailableIP error will be returned if no IP address resource available.
func GetAvailableIP(
	cidr string, ipRanges []flv1.IPRange, usedIPs []flv1.IPRange,
) (net.IP, error) {
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	// Iterate to get an available IP address from defined IPRanges.
	if len(ipRanges) > 0 {
		for _, r := range ipRanges {
			start := slices.Clone(r.From.To16())
			end := r.To.To16()
			// Skip the already used range to increase performance.
			if len(usedIPs) != 0 {
				for _, u := range usedIPs {
					if u.From.Equal(start) {
						start = slices.Clone(u.To.To16())
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
			if u.From.Equal(start) {
				start = slices.Clone(u.To.To16())
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
func AddIPToRange(ip net.IP, ipRanges []flv1.IPRange) []flv1.IPRange {
	if ip == nil {
		return ipRanges
	}
	length := len(ipRanges)
	if length == 0 {
		ipRanges = []flv1.IPRange{}
	}
	if IPInRanges(ip, ipRanges) {
		// Skip if ip already in ranges.
		return ipRanges
	}
	deleteIndex := -1
	for i := range ipRanges {
		var s1 net.IP = bytes.Clone(ipRanges[i].From)
		var s2 net.IP = bytes.Clone(ipRanges[i].To)
		IPDecrease(s1)
		IPIncrease(s2)
		if ip.Equal(s1) {
			ipRanges[i].From = s1
			return ipRanges
		}
		if ip.Equal(s2) {
			if i < length-1 {
				// Connect two ranges
				next := ipRanges[i+1]
				var nextFrom net.IP = bytes.Clone(next.From)
				IPDecrease(nextFrom)
				if s2.Equal(nextFrom) {
					ipRanges[i+1].From = ipRanges[i].From
					deleteIndex = i
					break
				}
			}
			ipRanges[i].To = s2
			return ipRanges
		}
	}
	if deleteIndex >= 0 {
		ipRanges = append(ipRanges[:deleteIndex], ipRanges[deleteIndex+1:]...)
		return ipRanges
	}
	ipRanges = append(ipRanges, flv1.IPRange{
		From: bytes.Clone(ip),
		To:   bytes.Clone(ip),
	})
	slices.SortFunc(ipRanges, func(a, b flv1.IPRange) int {
		return bytes.Compare(a.From, b.From)
	})
	return ipRanges
}

// RemoveIPFromRange removes an IP address from IPRange.
func RemoveIPFromRange(ip net.IP, ipRanges []flv1.IPRange) []flv1.IPRange {
	ip = ip.To16() // ensure 16 bytes length.
	if ip == nil {
		return ipRanges
	}
	if !IPInRanges(ip, ipRanges) {
		// Skip if ip not in ranges.
		return ipRanges
	}
	newRanges := []flv1.IPRange{}
	for _, r := range ipRanges {
		start := r.From.To16()
		end := r.To.To16()
		a := bytes.Compare(start, ip)
		b := bytes.Compare(end, ip)
		switch {
		case a < 0 && b > 0:
			var s1 net.IP = bytes.Clone(ip)
			var s2 net.IP = bytes.Clone(ip)
			IPDecrease(s1)
			IPIncrease(s2)
			newRanges = append(newRanges, flv1.IPRange{
				From: start,
				To:   s1,
			})
			newRanges = append(newRanges, flv1.IPRange{
				From: s2,
				To:   end,
			})
		case a == 0 && b > 0:
			var s1 net.IP = bytes.Clone(ip)
			IPIncrease(s1)
			newRanges = append(newRanges, flv1.IPRange{
				From: s1,
				To:   end,
			})
		case a < 0 && b == 0:
			var s2 net.IP = bytes.Clone(ip)
			IPDecrease(s2)
			newRanges = append(newRanges, flv1.IPRange{
				From: start,
				To:   s2,
			})
		case a == 0 && b == 0:
		default:
			newRanges = append(newRanges, flv1.IPRange{
				From: start,
				To:   end,
			})
		}
	}
	slices.SortFunc(newRanges, func(a, b flv1.IPRange) int {
		return bytes.Compare(a.From, b.From)
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

func CheckNetworkConflict(cidr1, cidr2 string) error {
	ip1, n1, err := net.ParseCIDR(cidr1)
	if err != nil {
		return fmt.Errorf("failed to parse CIDR [%v]: %w",
			cidr1, err)
	}
	ip2, n2, err := net.ParseCIDR(cidr2)
	if err != nil {
		return fmt.Errorf("failed to parse CIDR [%v]: %w",
			cidr2, err)
	}
	if ip1.Equal(ip2) {
		return fmt.Errorf("network [%v] already used: %w",
			cidr1, ErrNetworkConflict)
	}
	if n1.Contains(ip2) {
		return fmt.Errorf("network [%v] contains CIDR [%v]: %w",
			cidr1, cidr2, ErrNetworkConflict)
	}
	if n2.Contains(ip1) {
		return fmt.Errorf("network [%v] contains CIDR [%v]: %w",
			cidr2, cidr1, ErrNetworkConflict)
	}

	return nil
}

func CheckIPRangesConflict(ranges1, ranges2 []flv1.IPRange) error {
	if len(ranges1) == 0 || len(ranges2) == 0 {
		return nil
	}

	for _, r1 := range ranges1 {
		for _, r2 := range ranges2 {
			if err := ipRangeConflict(r1, r2); err != nil {
				return fmt.Errorf("range [%v] conflict with [%v]: %w",
					r1.String(), r2.String(), err)
			}
		}
	}

	return nil
}

func ipRangeConflict(r1, r2 flv1.IPRange) error {
	a1 := r1.From.To16()
	a2 := r1.To.To16()
	b1 := r2.From.To16()
	b2 := r2.To.To16()
	if a1 == nil || a2 == nil {
		return fmt.Errorf("invalid IP Range provided: %v", utils.Print(r1))
	}
	if b1 == nil || b2 == nil {
		return fmt.Errorf("invalid IP Range provided: %v", utils.Print(r2))
	}
	ranges := []flv1.IPRange{r2}
	if IPInRanges(a1, ranges) {
		return ErrIPRangesConflict
	}
	if IPInRanges(a2, ranges) {
		return ErrIPRangesConflict
	}
	return nil
}

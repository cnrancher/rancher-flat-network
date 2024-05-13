package ipcalc

import (
	"bytes"
	"net"
	"sort"
)

// http://play.golang.org/p/m8TNTtygK0
func incip(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

type sortedIPList []net.IP

func (p sortedIPList) Len() int {
	return len(p)
}

func (p sortedIPList) Less(i, j int) bool {
	return bytes.Compare(p[i], p[j]) == -1
}

func (p sortedIPList) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

func complementip(a, b []net.IP) []net.IP {
	c := []net.IP{}

	if len(a) == 0 {
		return c
	}

	am := make(map[string]bool, len(a))
	for _, v := range a {
		am[string(v)] = true
	}

	for _, v := range b {
		if am[string(v)] {
			delete(am, string(v))
		}
	}

	for k := range am {
		c = append(c, net.IP(k))
	}

	sort.Sort(sortedIPList(c))
	return c
}

func intersectionip(slice1, slice2 []net.IP) []net.IP {
	diff := []net.IP{}
	m1 := map[string]bool{}
	m2 := map[string]bool{}

	for _, v := range slice1 {
		m1[v.String()] = true
	}

	for _, v := range slice2 {
		m2[v.String()] = true
	}

	for _, v := range slice1 {
		if !m2[v.String()] {
			delete(m1, v.String())
		}
	}

	for k := range m1 {
		diff = append(diff, net.ParseIP(k))
	}
	sort.Sort(sortedIPList(diff))
	return diff
}

// CIDRtoHosts parse cidr string to ip list
func CIDRtoHosts(cidr string) ([]net.IP, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	var ips []net.IP
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); incip(ip) {
		if len(ip) == 4 {
			// remove network address and broadcast address
			if ip[3] == 0x00 || ip[3] == 0xff {
				continue
			}
		}
		a := net.IPv4(ip[0], ip[1], ip[2], ip[3])
		ips = append(ips, a)
	}
	return ips, nil
}

// RemoveUsedHosts make a complement set of hosts and used
func RemoveUsedHosts(hosts, used []net.IP) []net.IP {
	return complementip(hosts, used)
}

// GetUsableHosts make a intersection set of hosts and usable
func GetUsableHosts(hosts, usable []net.IP) []net.IP {
	return intersectionip(hosts, usable)
}

// CalcDefaultGateway returns x.x.x.1 for x.x.x.x/yy
func CalcDefaultGateway(cidr string) (net.IP, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	ip = ip.Mask(ipnet.Mask)
	incip(ip)
	return ip, nil
}

// IsInRangeBound judge a ip is in range boundray between lower and upper
func IsInRangeBound(trial net.IP, lower net.IP, upper net.IP) bool {
	if bytes.Compare(trial, lower) >= 0 && bytes.Compare(trial, upper) <= 0 {
		return true
	}
	return false
}

// ParseIPRange parse ip list from start to end
func ParseIPRange(start, end net.IP) []net.IP {
	ips := []net.IP{}
	if start == nil || end == nil {
		return ips
	}
	for ip := start; IsInRangeBound(ip, start, end); incip(ip) {
		a := net.ParseIP(ip.To4().String())
		ips = append(ips, a)
	}
	return ips
}

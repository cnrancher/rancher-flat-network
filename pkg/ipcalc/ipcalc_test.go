package ipcalc

import (
	"bytes"
	"net"
	"testing"
)

func testEq(a, b []net.IP) bool {

	// If one is nil, the other must also be nil.
	if (a == nil) != (b == nil) {
		return false
	}

	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if bytes.Compare(a[i], b[i]) != 0 {
			return false
		}
	}

	return true
}

func Test_incip(t *testing.T) {
	var ip1, ip2 net.IP
	ip1 = net.IPv4(192, 168, 1, 1)
	ip2 = net.IPv4(192, 168, 1, 2)
	incip(ip1)
	if !ip2.Equal(ip1) {
		t.Errorf("ip1 %s , ip2 %s", ip1, ip2)
	}

	ip1 = net.IPv4(192, 168, 1, 255)
	ip2 = net.IPv4(192, 168, 2, 0)

	incip(ip1)
	if !ip2.Equal(ip1) {
		t.Errorf("ip1 %s , ip2 %s", ip1, ip2)
	}
}

func Test_complementip(t *testing.T) {
	var ip1, ip2, diffs, expect []net.IP
	ip1 = []net.IP{
		net.IPv4(192, 168, 1, 1),
	}
	ip2 = []net.IP{
		net.IPv4(192, 168, 1, 1),
	}

	diffs = complementip(ip1, ip2)
	expect = []net.IP{}

	if !testEq(diffs, expect) {
		t.Errorf("ip1 %v , ip2 %v", ip1, ip2)
		t.Errorf("diffs %v , expect %v", diffs, expect)
	}

	ip1 = []net.IP{
		net.IPv4(192, 168, 1, 4),
		net.IPv4(192, 168, 1, 3),
		net.IPv4(192, 168, 1, 2),
		net.IPv4(192, 168, 1, 1),
	}
	ip2 = []net.IP{
		net.IPv4(192, 168, 1, 1),
		net.IPv4(192, 168, 1, 3),
		net.IPv4(192, 168, 1, 5),
	}

	diffs = complementip(ip1, ip2)
	expect = []net.IP{
		net.IPv4(192, 168, 1, 2),
		net.IPv4(192, 168, 1, 4),
	}

	if !testEq(diffs, expect) {
		t.Errorf("ip1 %v , ip2 %v", ip1, ip2)
		t.Errorf("diffs %v , expect %v", diffs, expect)
	}

	ip1 = []net.IP{
		net.IPv4(192, 168, 1, 4),
		net.IPv4(192, 168, 1, 3),
		net.IPv4(192, 168, 1, 2),
		net.IPv4(192, 168, 1, 1),
	}
	ip2 = []net.IP{}

	diffs = complementip(ip1, ip2)
	expect = []net.IP{
		net.IPv4(192, 168, 1, 1),
		net.IPv4(192, 168, 1, 2),
		net.IPv4(192, 168, 1, 3),
		net.IPv4(192, 168, 1, 4),
	}

	if !testEq(diffs, expect) {
		t.Errorf("ip1 %v , ip2 %v", ip1, ip2)
		t.Errorf("diffs %v , expect %v", diffs, expect)
	}

	ip1 = []net.IP{}
	ip2 = []net.IP{}

	diffs = complementip(ip1, ip2)
	expect = []net.IP{}

	if !testEq(diffs, expect) {
		t.Errorf("ip1 %v , ip2 %v", ip1, ip2)
		t.Errorf("diffs %v , expect %v", diffs, expect)
	}
}

func Test_RemoveUsedHosts(t *testing.T) {
	var ip1, ip2, diffs, expect []net.IP
	ip1 = []net.IP{
		net.IPv4(192, 168, 1, 4),
		net.IPv4(192, 168, 1, 3),
		net.IPv4(192, 168, 1, 2),
		net.IPv4(192, 168, 1, 1),
	}
	ip2 = []net.IP{
		net.IPv4(192, 168, 1, 1),
		net.IPv4(192, 168, 1, 3),
		net.IPv4(192, 168, 1, 5),
	}

	diffs = RemoveUsedHosts(ip1, ip2)
	expect = []net.IP{
		net.IPv4(192, 168, 1, 2),
		net.IPv4(192, 168, 1, 4),
	}

	if !testEq(diffs, expect) {
		t.Errorf("ip1 %v , ip2 %v", ip1, ip2)
		t.Errorf("diffs %v , expect %v", diffs, expect)
	}
}

func Test_CIDRtoHosts(t *testing.T) {
	var expect []net.IP
	hosts, err := CIDRtoHosts("192.168.1.100/30")
	if err != nil {
		t.Error(err)
	}
	expect = []net.IP{
		net.IPv4(192, 168, 1, 100),
		net.IPv4(192, 168, 1, 101),
		net.IPv4(192, 168, 1, 102),
		net.IPv4(192, 168, 1, 103),
	}
	if !testEq(hosts, expect) {
		t.Errorf("hosts %v , expect %v", hosts, expect)
	}

	hosts, err = CIDRtoHosts("192.168.1.255/30")
	if err != nil {
		t.Error(err)
	}
	expect = []net.IP{
		net.IPv4(192, 168, 1, 252),
		net.IPv4(192, 168, 1, 253),
		net.IPv4(192, 168, 1, 254),
	}
	if !testEq(hosts, expect) {
		t.Errorf("hosts %v , expect %v", hosts, expect)
	}

	hosts, _ = CIDRtoHosts("")
	if !testEq(hosts, nil) {
		t.Errorf("hosts %v , expect %v", hosts, expect)
	}
}

func Test_GetUseableHosts(t *testing.T) {
	var ip1, ip2, hosts, expect []net.IP
	ip1 = []net.IP{
		net.IPv4(192, 168, 1, 4),
		net.IPv4(192, 168, 1, 3),
		net.IPv4(192, 168, 1, 2),
		net.IPv4(192, 168, 1, 1),
	}
	ip2 = []net.IP{
		net.IPv4(192, 168, 1, 1),
		net.IPv4(192, 168, 1, 3),
		net.IPv4(192, 168, 1, 5),
	}

	hosts = GetUsableHosts(ip1, ip2)

	expect = []net.IP{
		net.IPv4(192, 168, 1, 1),
		net.IPv4(192, 168, 1, 3),
	}
	if !testEq(hosts, expect) {
		t.Errorf("hosts %v , expect %v", hosts, expect)
	}
}
func Test_CalcDefaultGateway(t *testing.T) {

	ip, _ := CalcDefaultGateway("192.168.1.0/24")
	if ip.String() != "192.168.1.1" {
		t.Errorf("result %v", ip)
	}

	ip, _ = CalcDefaultGateway("")
	if ip != nil {
		t.Errorf("result %v", ip)
	}
}

func Test_ParseIPRange(t *testing.T) {

	hosts := ParseIPRange("192.168.1.10", "192.168.1.15")

	expect := []net.IP{
		net.IPv4(192, 168, 1, 10),
		net.IPv4(192, 168, 1, 11),
		net.IPv4(192, 168, 1, 12),
		net.IPv4(192, 168, 1, 13),
		net.IPv4(192, 168, 1, 14),
		net.IPv4(192, 168, 1, 15),
	}
	if !testEq(hosts, expect) {
		t.Errorf("hosts %v , expect %v", hosts, expect)
	}

	if !testEq(ParseIPRange("", ""), []net.IP{}) {
		t.Errorf("hosts %v , expect %v", hosts, expect)
	}
}

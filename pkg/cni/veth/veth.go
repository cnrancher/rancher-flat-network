package veth

import (
	"fmt"
	"net"

	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

const (
	PodVethIface = "veth0"
)

var (
	ipv4Mask = net.IPv4Mask(255, 255, 255, 255)
	ipv6Mask net.IPMask
)

func init() {
	// PodIP is IPv6, set IPv6 mask
	ipv6Mask = make(net.IPMask, net.IPv6len)
	for i := range ipv6Mask {
		ipv6Mask[i] = 0xff
	}
}

// CreatePairForPod creates an veth pair between pod and host to allow host
// has ability to access to pod directly.
func CreatePairForPod(netns ns.NetNS, master string, podIP net.IP) error {
	// veth1 is for host, veth2 is for pod
	veth1, veth2, err := createVeth()
	if err != nil {
		return fmt.Errorf("createVeth: %w", err)
	}

	defer func() {
		// cleanup if error
		if err != nil {
			l, err := netlink.LinkByName(veth1)
			if err != nil {
				logrus.Errorf("failed to cleanup veth %v: %v", veth1, err)
				return
			}
			netlink.LinkDel(l)
		}
	}()

	veth1Link, err := netlink.LinkByName(veth1)
	if err != nil {
		return fmt.Errorf("failed to get veth1 link [%v]: %w",
			veth1, err)
	}
	veth2Link, err := netlink.LinkByName(veth2)
	if err != nil {
		return fmt.Errorf("failed to get veth2 link [%v]: %w",
			veth2, err)
	}
	masterLink, err := netlink.LinkByName(master)
	if err != nil {
		return fmt.Errorf("failed to get master link [%v]: %w",
			master, err)
	}

	// Move veth2 to pod NS
	if err = netlink.LinkSetNsFd(veth2Link, int(netns.Fd())); err != nil {
		return fmt.Errorf("failed to move veth2 %q to pod NetNS: %w",
			veth2, err)
	}

	// Set veth1 UP
	if err = netlink.LinkSetUp(veth1Link); err != nil {
		return fmt.Errorf("failed to set veth1 up: %w", err)
	}
	// Add podIP route to veth1
	r := &netlink.Route{
		LinkIndex: veth1Link.Attrs().Index,
		Dst: &net.IPNet{
			IP:   podIP,
			Mask: ipv4Mask,
		},
	}
	family := netlink.FAMILY_V4
	if podIP.To4() == nil {
		r.Dst.Mask = ipv6Mask
		family = netlink.FAMILY_V6
	}
	if err = netlink.RouteAdd(r); err != nil {
		return fmt.Errorf("failed to add route [%v] for link [%v]: %w",
			podIP.String(), veth1, err)
	}
	logrus.Infof("add route podIP %q for veth1 [%v]",
		podIP.String(), veth1)

	// List master addrs
	masterAddrs, err := netlink.AddrList(masterLink, family)
	if err != nil {
		return fmt.Errorf("failed to list master addrs [%v]: %w",
			master, err)
	}
	if err := netns.Do(func(_ ns.NetNS) error {
		err := ip.RenameLink(veth2, PodVethIface)
		if err != nil {
			return fmt.Errorf("failed to rename veth iface name to %q: %w",
				PodVethIface, err)
		}
		logrus.Debugf("rename link [%v] to [%v] in pod NS", veth2, PodVethIface)

		veth2Link, err := netlink.LinkByName(PodVethIface)
		if err != nil {
			return fmt.Errorf("failed to get veth2 link in pod [%v]: %w",
				PodVethIface, err)
		}
		if err = netlink.LinkSetUp(veth2Link); err != nil {
			return fmt.Errorf("failed to set veth2 up: %w", err)
		}

		if len(masterAddrs) == 0 {
			return nil
		}
		// Add host IP addrs to pod veth2
		for _, a := range masterAddrs {
			a.IPNet.Mask = ipv4Mask
			if family == netlink.FAMILY_V6 {
				a.IPNet.Mask = ipv6Mask
			}
			if err = netlink.RouteAdd(&netlink.Route{
				LinkIndex: veth2Link.Attrs().Index,
				Dst:       a.IPNet,
				Src:       podIP,
			}); err != nil {
				return fmt.Errorf("failed to add route [%v] for link [%v]: %w",
					a.IPNet.String(), PodVethIface, err)
			}
			logrus.Infof("add route host IP %q for veth1 [%v]",
				a.IP.String(), veth1)
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

// createVeth creates a random veth iface and return its name
func createVeth() (string, string, error) {
	n1, err := ip.RandomVethName()
	if err != nil {
		return "", "", fmt.Errorf("ip.RandomVethName: %w", err)
	}
	n2, err := ip.RandomVethName()
	if err != nil {
		return "", "", fmt.Errorf("ip.RandomVethName: %w", err)
	}
	v := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: n1,
		},
		PeerName: n2,
	}
	if err := netlink.LinkAdd(v); err != nil {
		return "", "", fmt.Errorf("failed to create veth: LinkAdd: %w",
			err)
	}
	logrus.Debugf("created veth iface [%v], [%v]", n1, n2)
	return n1, n2, nil
}

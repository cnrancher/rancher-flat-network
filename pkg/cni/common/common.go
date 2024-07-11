package common

import (
	"fmt"
	"net"
	"slices"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

const (
	PodIfaceEth0 = "eth0"
)

func AddFlatNetworkRouteToHost(
	podNS ns.NetNS, flatNetworkIP net.IP, master string,
) error {
	if podNS == nil || len(flatNetworkIP) == 0 || flatNetworkIP.To16() == nil || master == "" {
		return nil
	}

	var family int
	var mask []byte
	if flatNetworkIP.To4() == nil {
		family = netlink.FAMILY_V6
		mask = []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	} else {
		family = netlink.FAMILY_V4
		mask = net.IPv4Mask(0xff, 0xff, 0xff, 0xff)
	}

	podIP, err := getPodNativeIP(podNS)
	if err != nil {
		err = fmt.Errorf("getPodNativeIP: failed to get Pod eth0 IP: %w", err)
		logrus.Error(err)
		return err
	}
	if len(podIP) == 0 {
		logrus.Warnf("skip add flatNetwork route to host: podIP not found")
		return nil
	}
	logrus.Debugf("get Pod eth0 IP address %q", podIP)
	route, err := getHostRouteByIP(podIP)
	if err != nil {
		err = fmt.Errorf("failed to get route by pod IP %q on host: %w",
			podIP.String(), err)
		logrus.Error(err)
		return err
	}
	if route == nil {
		logrus.Warnf("skip add flatNetwork route to host: podIP original route not found")
		return nil
	}
	logrus.Debugf("get IP address %q default route link %v", podIP, route.LinkIndex)

	r := &netlink.Route{
		LinkIndex: route.LinkIndex,
		Dst: &net.IPNet{
			IP:   flatNetworkIP,
			Mask: mask,
		},
		Family: family,
		Src:    nil,
	}
	// Add route src addr if the master card have address
	addrs, err := listIfaceAddr(master, family)
	if err != nil {
		err = fmt.Errorf("failed to get iface %q addresses: %w", master, err)
		logrus.Error(err)
		return err
	}
	if len(addrs) != 0 {
		for _, a := range addrs {
			if a.IP.IsLinkLocalUnicast() {
				continue
			}
			r.Src = addrs[0].IP
			break
		}
	}
	if err := netlink.RouteAdd(r); err != nil {
		err = fmt.Errorf("failed to add pod flatNetwork IP %q route on host: %w",
			flatNetworkIP.String(), err)
		logrus.Error(err)
		return err
	}
	logrus.Infof("create flatNetwork route IP %q to link ID %v on host",
		flatNetworkIP, route.LinkIndex)

	return nil
}

func DelFlatNetworkRouteFromHost(flatNetworkIP net.IP) error {
	if len(flatNetworkIP) == 0 || flatNetworkIP.To16() == nil {
		return nil
	}
	if flatNetworkIP.To4() == nil {
		logrus.Infof("skip del pod FlatNetwork IP route to host: address is IPv6")
		return nil
	}

	route, err := getHostRouteByIP(flatNetworkIP)
	if err != nil {
		err = fmt.Errorf("failed to get route by flatNetwork IP %q on host: %w",
			flatNetworkIP.String(), err)
		logrus.Error(err)
		return err
	}
	if route == nil {
		logrus.Infof("skip del flatNetwork IP %q route from host: already deleted",
			flatNetworkIP.String())
		return nil
	}

	if err := netlink.RouteDel(route); err != nil {
		err = fmt.Errorf("failed to delete flatNetwork IP %q route from host: %w",
			flatNetworkIP.String(), err)
		logrus.Error(err)
		return err
	}
	logrus.Infof("delete flatNetwork route IP %q from link ID %v on host",
		flatNetworkIP, route.LinkIndex)

	return nil
}

// getPodNativeIP returns Native CNI allocated IP on Pod iface eth0
// NOTE: will return nil if no IP allocated on pod eth0 iface.
func getPodNativeIP(podNS ns.NetNS) (net.IP, error) {
	var podNativeIP net.IP
	podNS.Do(func(_ ns.NetNS) error {
		link, err := netlink.LinkByName(PodIfaceEth0)
		if err != nil {
			return fmt.Errorf("failed to get iface %q on pod: %w",
				PodIfaceEth0, err)
		}

		addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
		if err != nil {
			return fmt.Errorf("failed to list IP addr on pod iface %q: %w",
				PodIfaceEth0, err)
		}

		if len(addrs) == 0 {
			return nil
		}
		for _, a := range addrs {
			if a.IP.IsLinkLocalUnicast() {
				// Skip link local unicast addr
				continue
			}
			podNativeIP = slices.Clone(a.IP)
		}
		return nil
	})

	return podNativeIP, nil
}

// getHostRouteByIP executes 'ip route get <IP>' on host network NS.
// NOTE: will return nil if no route found
func getHostRouteByIP(ip net.IP) (*netlink.Route, error) {
	routes, err := netlink.RouteGet(ip)
	if err != nil {
		return nil, fmt.Errorf("netlink.RouteGet failed: %w", err)
	}
	if len(routes) == 0 {
		return nil, nil
	}
	return &routes[0], nil
}

func listIfaceAddr(iface string, family int) ([]netlink.Addr, error) {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return nil, fmt.Errorf("listIfaceAddr: link %q not found: %w",
			iface, err)
	}
	addrs, err := netlink.AddrList(link, family)
	if err != nil {
		return nil, fmt.Errorf("listIfaceAddr: failed to list addr: %w", err)
	}
	return addrs, nil
}

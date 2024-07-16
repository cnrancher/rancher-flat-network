package route

import (
	"fmt"
	"net"
	"strings"

	"github.com/cnrancher/rancher-flat-network/pkg/cni/common"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
)

// AddFlatNetworkRouteToHost adds flatNetworkIP route to native CNI iface on host NS
//
// Example: ip route add <FLAT_NETWORK_IP> dev <NATIVE_CNI_IFACE> src <HOST_IP>
func AddFlatNetworkRouteToHost(
	podNS ns.NetNS, flatNetworkIP net.IP, master string,
) error {
	if podNS == nil || len(flatNetworkIP) == 0 || flatNetworkIP.To16() == nil || master == "" {
		return nil
	}

	var family int
	var mask []byte
	family = nl.GetIPFamily(flatNetworkIP)
	if flatNetworkIP.To4() == nil {
		mask = net.CIDRMask(net.IPv6len*8, net.IPv6len*8)
	} else {
		mask = net.CIDRMask(net.IPv4len*8, net.IPv4len*8)
	}

	podIP, err := common.GetPodNativeIP(podNS, netlink.FAMILY_V4)
	if err != nil {
		err = fmt.Errorf("common.GetPodNativeIP: failed to get Pod eth0 IP: %w", err)
		logrus.Error(err)
		return err
	}
	if len(podIP) == 0 {
		logrus.Warnf("skip add flatNetwork route to host: pod IP not found")
		return nil
	}
	logrus.Debugf("get Pod eth0 IP address %q", podIP)
	route, err := GetRouteByIP(podIP)
	if err != nil {
		err = fmt.Errorf("failed to get route by pod IP %q on host: %w",
			podIP.String(), err)
		logrus.Error(err)
		return err
	}
	if route == nil || route.Dst == nil {
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
	addrs, err := common.AddrListByName(master, family)
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
		if err := netlink.RouteDel(r); err != nil {
			logrus.Errorf("failed to delete existing route %q on host: %v",
				flatNetworkIP.String(), err)
		}
		return err
	}
	logrus.Infof("create flatNetwork route [%v dev %v src %v] on host NS",
		flatNetworkIP, r.LinkIndex, r.Src)

	return nil
}

// DelFlatNetworkRouteFromHost remove flatNetworkIP route from native CNI iface on host NS
//
// Example: ip route del <FLAT_NETWORK_IP>
func DelFlatNetworkRouteFromHost(flatNetworkIP net.IP) error {
	if len(flatNetworkIP) == 0 || flatNetworkIP.To16() == nil {
		return nil
	}

	route, err := GetRouteByIP(flatNetworkIP)
	if err != nil {
		err = fmt.Errorf("failed to get route by flatNetwork IP %q on host: %w",
			flatNetworkIP.String(), err)
		logrus.Error(err)
		return err
	}
	if route == nil || route.Dst == nil {
		logrus.Infof("skip del flatNetwork IP %q route from host: already deleted",
			flatNetworkIP.String())
		return nil
	}

	r := &netlink.Route{
		LinkIndex: route.LinkIndex,
		Dst:       route.Dst,
	}
	if err := netlink.RouteDel(r); err != nil {
		if strings.Contains(err.Error(), "no such process") {
			logrus.Infof("DelFlatNetworkRouteFromHost: skip delete: %v: %v",
				utils.Print(r), err)
			return nil
		}
		err = fmt.Errorf("failed to delete flatNetwork IP %q route from host: %w",
			flatNetworkIP.String(), err)
		logrus.Error(err)
		return err
	}
	logrus.Infof("delete flatNetwork route IP %q from link ID %v on host",
		flatNetworkIP, route.LinkIndex)

	return nil
}

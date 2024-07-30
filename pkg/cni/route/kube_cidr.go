package route

import (
	"fmt"
	"net"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
)

func AddPodKubeCIDRRoutes(podNS ns.NetNS, cidr string) error {
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("failed to parse CIDR %q: %w", cidr, err)
	}
	err = podNS.Do(func(_ ns.NetNS) error {
		defaultLinkSet, err := GetDefaultLinkIDSet()
		if err != nil {
			return fmt.Errorf("failed to get pod default link id: %w", err)
		}

		podDefaultRoutes, err := GetDefaultRoutes()
		if err != nil {
			return fmt.Errorf("failed to get pod default routes: %w", err)
		}
		if len(podDefaultRoutes) == 0 {
			return nil
		}
		var podDefaultGatewayV4 net.IP
		var podDefaultGatewayV6 net.IP
		for _, r := range podDefaultRoutes {
			switch r.Family {
			case netlink.FAMILY_V4:
				podDefaultGatewayV4 = r.Gw
			default:
				podDefaultGatewayV6 = r.Gw
			}
		}

		for id := range defaultLinkSet {
			r := netlink.Route{
				LinkIndex: id,
				Dst:       network,
				Family:    netlink.FAMILY_V4,
				Gw:        podDefaultGatewayV4,
			}
			if ip.To16() != nil && len(ip.To4()) == 0 {
				r.Family = netlink.FAMILY_V6
				r.Gw = podDefaultGatewayV6
			}
			return EnsureRouteExists(&r)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("addPodKubeCIDRRoutes: %w", err)
	}
	return nil
}

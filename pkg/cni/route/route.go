package route

import (
	"fmt"
	"net"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
)

// GetDefaultRoute gets default route
// NOTE: will return nil if no route found
func GetDefaultRoutes() ([]netlink.Route, error) {
	rs, err := netlink.RouteList(nil, netlink.FAMILY_ALL)
	if err != nil {
		return nil, fmt.Errorf("getDefaultRoute: failed to list route: %w", err)
	}
	if len(rs) == 0 {
		return nil, nil
	}
	var results []netlink.Route
	for _, r := range rs {
		if r.Dst == nil {
			results = append(results, r)
		}
	}
	return results, nil
}

func GetDefaultLinkIDSet() (map[int]bool, error) {
	defaultRoutes, err := GetDefaultRoutes()
	if err != nil {
		return nil, fmt.Errorf("getDefaultLinkIDSet: %w", err)
	}

	defaultLinkID := map[int]bool{} // map[linkID]true
	if len(defaultRoutes) != 0 {
		for _, r := range defaultRoutes {
			defaultLinkID[r.LinkIndex] = true
		}
	}
	return defaultLinkID, nil
}

// GetRouteByIP executes 'ip route get <IP>' on host network NS.
// NOTE: will return nil if no route found
func GetRouteByIP(ip net.IP) (*netlink.Route, error) {
	routes, err := netlink.RouteGet(ip)
	if err != nil {
		return nil, fmt.Errorf("netlink.RouteGet failed: %w", err)
	}
	if len(routes) == 0 {
		return nil, nil
	}
	return &routes[0], nil
}

// CheckRouteExists checks whether the route exists by route dst
func CheckRouteExists(
	route *netlink.Route,
) (bool, error) {
	if route == nil {
		return false, nil
	}
	rs, err := netlink.RouteList(nil, netlink.FAMILY_ALL)
	if err != nil {
		return false, fmt.Errorf("checkRouteExists: failed to list route: %w", err)
	}
	if len(rs) == 0 {
		return false, nil
	}
	for _, r := range rs {
		// TODO: Change here if needed
		// Simple logic to avoid conflict when add routes
		if r.Dst.String() != route.Dst.String() {
			continue
		}
		if r.Family != route.Family {
			continue
		}
		if r.Src != nil && route.Src != nil {
			if !r.Src.Equal(route.Src) {
				continue
			}
		}
		if r.Via != nil && route.Via != nil {
			if !r.Via.Equal(route.Via) {
				continue
			}
		}
		if r.Gw != nil && route.Gw != nil {
			if !r.Gw.Equal(route.Gw) {
				continue
			}
		}
		logrus.Debugf("route already exists on pod: [%v]",
			utils.Print(r))
		return true, nil
	}
	return false, nil
}

// EnsureRouteExists adds route if not exists
func EnsureRouteExists(
	route *netlink.Route,
) error {
	if route == nil {
		return nil
	}
	if ok, err := CheckRouteExists(route); err != nil {
		return fmt.Errorf("ensureRouteExists: checkRouteExists: %w", err)
	} else if ok {
		// skip if route already exists
		return nil
	}
	if err := netlink.RouteAdd(route); err != nil {
		return fmt.Errorf("ensureRouteExists: failed to add route dst %q: %w",
			route.Dst.String(), err)
	}
	logrus.Infof("add route dst %q on link id %v",
		route.Dst.String(), route.LinkIndex)
	return nil
}

// AddPodFlatNetworkCustomRoutes adds user defined custom routes and
// host IP routes to pod NS
func AddPodFlatNetworkCustomRoutes(podNS ns.NetNS, customRoutes []flv1.Route) error {
	if podNS == nil || len(customRoutes) == 0 {
		return nil
	}
	err := podNS.Do(func(_ ns.NetNS) error {
		for _, r := range customRoutes {
			link, err := netlink.LinkByName(r.Dev)
			if err != nil {
				return fmt.Errorf("failed to get link %q in pod: %w",
					r.Dev, err)
			}
			ip, network, err := net.ParseCIDR(r.Dst)
			if err != nil {
				return fmt.Errorf("failed to parse CIDR %q: %w", r.Dst, err)
			}

			route := &netlink.Route{
				LinkIndex: link.Attrs().Index,
				Src:       r.Src,
				Via:       nil,
				Dst:       network,
				Priority:  r.Priority,
				Family:    nl.GetIPFamily(ip),
			}
			if r.Via != nil {
				route.Via = &netlink.Via{
					AddrFamily: nl.GetIPFamily(ip),
					Addr:       r.Via,
				}
			}

			switch network.IP.String() {
			case "0.0.0.0", "::0":
				route.Dst = nil
			}
			logrus.Debugf("add custom route: %v", utils.Print(route))
			if err := EnsureRouteExists(route); err != nil {
				return fmt.Errorf("failed to add pod custom route %q: %w",
					r.Dst, err)
			}
		}
		return nil
	})
	if err != nil {
		logrus.Error(err)
		return fmt.Errorf("addPodFlatNetworkCustomRoutes: %w", err)
	}
	return nil
}

func UpdatePodDefaultGateway(
	podNS ns.NetNS, ifName string, flatNetworkIP net.IP, gateway net.IP,
) error {
	err := podNS.Do(func(_ ns.NetNS) error {
		link, err := netlink.LinkByName(ifName)
		if err != nil {
			return fmt.Errorf("failed to get iface %q: %w", ifName, err)
		}

		routes, err := netlink.RouteList(nil, netlink.FAMILY_ALL)
		if err != nil {
			return fmt.Errorf("failed to list route: %w", err)
		}
		for _, r := range routes {
			if r.Dst != nil {
				// Only replace default routes...
				continue
			}

			replaced := r
			// change dev to flatNetwork interface
			replaced.LinkIndex = link.Attrs().Index
			replaced.Src = flatNetworkIP
			replaced.Gw = nil
			// FIXME: gateway may encounter error if not reachable
			_ = gateway
			logrus.Debugf("request to replace default route %v", utils.Print(replaced))
			if err := netlink.RouteReplace(&replaced); err != nil {
				return fmt.Errorf("failed to replace default route: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		logrus.Error(err)
		return fmt.Errorf("updatePodDefaultGateway: %w", err)
	}
	return nil
}

package route

import (
	"fmt"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
)

func CheckRouteExists(netns ns.NetNS, link netlink.Link, route *netlink.Route) (bool, error) {
	if netns == nil {
		return routeExists(link, route)
	}
	var exists bool
	if err := netns.Do(func(_ ns.NetNS) error {
		var err error
		exists, err = routeExists(link, route)
		return err
	}); err != nil {
		return exists, err
	}

	return false, nil
}

func routeExists(link netlink.Link, route *netlink.Route) (bool, error) {
	// Check route exists in root NS
	rs, err := netlink.RouteList(link, netlink.FAMILY_ALL)
	if err != nil {
		return false, fmt.Errorf("failed to list route: %w", err)
	}
	if len(rs) == 0 {
		return false, nil
	}
	for _, r := range rs {
		if r.Equal(*route) {
			return true, nil
		}
	}
	return false, nil
}

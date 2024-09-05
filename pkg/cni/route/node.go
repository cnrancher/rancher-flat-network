package route

import (
	"fmt"
	"net"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/cni/common"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
)

// getHostCIDRCustomRoutes for adding host iface IP addr routes to pod
func getHostCIDRCustomRoutes(linkID int, gwV4, gwV6 net.IP) ([]flv1.Route, error) {
	link, err := netlink.LinkByIndex(linkID)
	if err != nil {
		return nil, fmt.Errorf("getHostCIDRCustomRoutes: %w", err)
	}
	addrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		return nil, fmt.Errorf("getHostCIDRCustomRoutes: %w", err)
	}
	if len(addrs) == 0 {
		return nil, nil
	}
	routes := []flv1.Route{}
	for _, a := range addrs {
		if a.IP.IsLinkLocalUnicast() {
			continue
		}
		r := flv1.Route{
			Dev: common.PodIfaceEth0,
			Dst: a.IPNet.String(),
			Via: nil,
		}
		switch nl.GetIPFamily(a.IP) {
		case netlink.FAMILY_V4:
			r.Via = gwV4
		default:
			r.Via = gwV6
		}
		routes = append(routes, r)
	}
	logrus.Debugf("getHostCIDRCustomRoutes: %v", utils.Print(routes))
	return routes, nil
}

func AddPodNodeCIDRRoutes(podNS ns.NetNS) error {
	// Add host iface IP addr routes and user custom routes to Pod
	customRoutes := []flv1.Route{}
	defaultLinkSet, err := GetDefaultLinkIDSet()
	if err != nil {
		return fmt.Errorf("failed to get pod default link id: %w", err)
	}

	var podDefaultGatewayV4 net.IP
	var podDefaultGatewayV6 net.IP
	if err := podNS.Do(func(_ ns.NetNS) error {
		podDefaultRoutes, err := GetDefaultRoutes()
		if err != nil {
			return fmt.Errorf("failed to get pod default routes: %w", err)
		}
		if len(podDefaultRoutes) == 0 {
			logrus.Warnf("AddPodNodeCIDRRoutes: pod default route not found, skip")
			return nil
		}
		for _, r := range podDefaultRoutes {
			switch r.Family {
			case netlink.FAMILY_V4:
				podDefaultGatewayV4 = r.Gw
			default:
				podDefaultGatewayV6 = r.Gw
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("addPodNodeCIDRRoutes: %w", err)
	}
	if len(defaultLinkSet) == 0 {
		logrus.Warnf("AddPodNodeCIDRRoutes: pod default link not found, skip")
	}
	for id := range defaultLinkSet {
		results, err := getHostCIDRCustomRoutes(id, podDefaultGatewayV4, podDefaultGatewayV6)
		if err != nil {
			return fmt.Errorf("addPodNodeCIDRRoutes: %w", err)
		}
		if len(results) == 0 {
			continue
		}
		customRoutes = append(customRoutes, results...)
	}

	return AddPodFlatNetworkCustomRoutes(podNS, customRoutes)
}

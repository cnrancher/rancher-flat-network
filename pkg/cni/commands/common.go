package commands

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/types"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/utils"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"

	flv1 "github.com/cnrancher/rancher-flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	types100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/types/create"
)

func getVlanIfaceOnHost(
	master string, mtu int, vlanID int,
) (*types100.Interface, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, fmt.Errorf("netlink.LinkList: failed to list links: %w", err)
	}

	ifName := master
	if vlanID != 0 {
		ifName = ifName + "." + fmt.Sprint(vlanID)
	}
	for _, l := range links {
		if l.Attrs().Name == ifName {
			iface := &types100.Interface{}
			iface.Name = ifName
			iface.Mac = l.Attrs().HardwareAddr.String()
			return iface, nil
		}
	}
	return createVLANOnHost(master, mtu, ifName, vlanID)
}

// createVLANOnHost creates a VLAN interface <ifname>.<vlanID> (eth0.1) on root
// network namespace.
func createVLANOnHost(
	master string, MTU int, ifName string, vlanID int,
) (*types100.Interface, error) {
	rootNS, err := netns.Get()
	if err != nil {
		return nil, fmt.Errorf(
			"createVLANOnHost: failed to get root network NS: %w", err)
	}
	defer rootNS.Close()

	m, err := netlink.LinkByName(master)
	if err != nil {
		return nil, fmt.Errorf(
			"createVLANOnHost: failed to lookup master %q: %v", master, err)
	}

	vlan := &netlink.Vlan{
		LinkAttrs: netlink.LinkAttrs{
			MTU:         MTU,
			Name:        ifName,
			ParentIndex: m.Attrs().Index,
			Namespace:   netlink.NsFd(int(rootNS)),
		},
		VlanId: vlanID,
	}
	if err := netlink.LinkAdd(vlan); err != nil {
		return nil, fmt.Errorf(
			"createVLANOnHost: failed to create VLAN on %q: %w", ifName, err)
	}
	if err := netlink.LinkSetUp(vlan); err != nil {
		netlink.LinkDel(vlan)
		return nil, fmt.Errorf(
			"createVLANOnHost: failed to set vlan iface [%v] status UP: %w",
			ifName, err)
	}
	logrus.Infof("create vlan interface [%v] on host", ifName)

	// Re-fetch vlan to get all properties/attributes
	contVlan, err := netlink.LinkByName(ifName)
	if err != nil {
		netlink.LinkDel(vlan)
		return nil, fmt.Errorf(
			"createVLANOnHost: failed to refetch vlan iface [%v]: %w",
			ifName, err)
	}
	iface := &types100.Interface{
		Name: ifName,
		Mac:  contVlan.Attrs().HardwareAddr.String(),
	}
	return iface, nil
}

func addEth0CustomRoutes(netns ns.NetNS, routes []flv1.Route) error {
	if len(routes) == 0 {
		return nil
	}

	err := netns.Do(func(_ ns.NetNS) error {
		rs, err := netlink.RouteList(nil, netlink.FAMILY_V4)
		if err != nil {
			logrus.Warnf("addEth0CustomRoutes: failed to list routes: %v", err)
			return nil
		}
		logrus.Debugf("existing routes: %v", utils.Print(rs))

		if len(rs) == 0 {
			return nil
		}
		originDefault := rs[0]
		for _, v := range routes {
			if v.Iface != "eth0" {
				continue
			}

			eth0Link, _ := netlink.LinkByIndex(originDefault.LinkIndex)
			if v.Dst == "0.0.0.0/0" {
				if err := ip.AddDefaultRoute(v.GW, eth0Link); err != nil {
					return err
				}
				continue
			}

			_, dst, err := net.ParseCIDR(v.Dst)
			if err != nil {
				logrus.Infof("%v", err)
				continue
			}

			err = ip.AddRoute(dst, v.GW, eth0Link)
			if err != nil {
				return fmt.Errorf("%v %s", err, v.GW)
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("addEth0CustomRoutes: failed to add custom route: %w", err)
	}
	return nil
}

func changeDefaultGateway(netns ns.NetNS, serviceCIDR string, gateway net.IP) error {
	logrus.Infof("changeDefaultGateway: %s", gateway)
	err := netns.Do(func(_ ns.NetNS) error {
		// List IPv4 & IPv6 routes
		routes, err := netlink.RouteList(nil, netlink.FAMILY_ALL)
		if err != nil {
			logrus.Errorf("failed to list routes: %v", err)
			return fmt.Errorf("failed to list routes in pod NS: %w", err)
		}

		var originDefault netlink.Route
		for _, r := range routes {
			if r.Dst == nil || len(r.Dst.IP) == 0 {
				originDefault = r
				// 1. delete default gateway
				if err := netlink.RouteDel(&r); err != nil {
					return fmt.Errorf("failed to delete pod default gw: %w", err)
				}
				logrus.Infof("delete old default route GW [%v] in pod NS", r.Gw)

				// 2. add eth1 gateway
				eth1Route := r
				// FIXME: should check link 'eth1' ID instead of simply +1.
				eth1Route.LinkIndex = eth1Route.LinkIndex + 1
				eth1Route.Gw = gateway
				if err := netlink.RouteAdd(&eth1Route); err != nil {
					return fmt.Errorf("failed to add default route: %w", err)
				}
				logrus.Infof("add default route GW [%v] in pod NS", eth1Route.Gw)
			}
		}

		// 3. add serviceCIDR route
		clusterRoute := originDefault
		_, dst, err := net.ParseCIDR(serviceCIDR)
		if err != nil {
			return fmt.Errorf("failed to parse CIDR %q: %w",
				serviceCIDR, err)
		}
		clusterRoute.Dst = dst
		err = netlink.RouteAdd(&clusterRoute)
		if err != nil {
			return fmt.Errorf("failed to add serviceCIDR route: %w", err)
		}
		logrus.Infof("add serviceCIDR [%v] route [%v]", clusterRoute.Dst, clusterRoute.Dst)
		return nil
	})
	if err != nil {
		return fmt.Errorf("changeDefaultGateway: %w", err)
	}
	return nil
}

func loadCNINetConf(bytes []byte) (*types.NetConf, error) {
	n := &types.NetConf{}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %w", err)
	}
	return n, nil
}

func setPromiscOn(iface string) error {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return fmt.Errorf("failed to search iface %q: %w", iface, err)
	}

	if link.Attrs().Promisc == 1 {
		return nil
	}
	if err = netlink.SetPromiscOn(link); err != nil {
		return fmt.Errorf("netlink.SetPromiscOn failed on iface %q: %w", iface, err)
	}
	logrus.Infof("set promisc on master link [%v]", iface)
	return nil
}

func mergeIPAMConfig(
	netConf *types.NetConf, flatNetworkIP *flv1.FlatNetworkIP, subnet *flv1.FlatNetworkSubnet,
) ([]byte, error) {
	address := flatNetworkIP.Status.Addr
	_, n, err := net.ParseCIDR(subnet.Spec.CIDR)
	if err != nil {
		return nil, fmt.Errorf("failed to parse subnet CIDR: %w", err)
	}
	ones, _ := n.Mask.Size()
	routes, gateway := subnet.Spec.Routes, subnet.Spec.Gateway
	enable6to4 := flatNetworkIP.Annotations[flv1.AnnotationsIPv6to4] != ""
	netConf.IPAM.Addresses = []types.Address{
		{
			Address: fmt.Sprintf("%v/%v", address.String(), ones),
			Gateway: gateway,
		},
	}

	// add 6to4 address for IPv6
	// https://en.wikipedia.org/wiki/6to4
	if enable6to4 {
		_, n, err := net.ParseCIDR(subnet.Spec.CIDR)
		if err != nil {
			return nil, fmt.Errorf("failed to parse subnet CIDR [%v]: %w",
				subnet.Spec.CIDR, err)
		}
		size, _ := n.Mask.Size()
		if ip6to4CIDR := get6to4CIDR(address, size); len(ip6to4CIDR) != 0 {
			netConf.IPAM.Addresses = append(netConf.IPAM.Addresses, types.Address{
				Address: ip6to4CIDR,
			})
		}
	}

	if len(routes) != 0 {
		rs := []*cnitypes.Route{}
		for _, v := range routes {
			if v.Iface != "" && v.Iface == "eth0" {
				continue
			}

			_, n, err := net.ParseCIDR(v.Dst)
			if err != nil {
				return nil, fmt.Errorf("failed to parse route destination CIDR [%v]: %w",
					v.Dst, err)
			}
			rs = append(rs, &cnitypes.Route{
				Dst: *n,
				GW:  v.GW,
			})
		}
		netConf.IPAM.Routes = rs
	}
	logrus.Debugf("merged IPAM addresses: %v", utils.Print(netConf.IPAM))
	return json.Marshal(netConf)
}

func get6to4CIDR(ip net.IP, size int) string {
	if ip = ip.To4(); ip == nil {
		return ""
	}
	if size <= 0 || size > 32 {
		return ""
	}
	sixtofourSize := 48 - (32 - size)
	tmp := []byte{}
	for _, v := range ip.To4() {
		tmp = append(tmp, v)
	}
	if len(tmp) != 4 {
		return ""
	}
	return fmt.Sprintf("2002:%02x:%02x:0:0:0:0:0/%d",
		tmp[0]+tmp[1], tmp[2]+tmp[3], sixtofourSize)
}

// parsePrevResult parses a prevResult in a NetConf structure and sets
// the NetConf's PrevResult member to the parsed Result object.
func parsePrevResult(conf *types.NetConf) error {
	if conf.RawPrevResult == nil {
		return nil
	}

	// Prior to 1.0.0, Result types may not marshal a CNIVersion. Since the
	// result version must match the config version, if the Result's version
	// is empty, inject the config version.
	if ver, ok := conf.RawPrevResult["CNIVersion"]; !ok || ver == "" {
		conf.RawPrevResult["CNIVersion"] = conf.CNIVersion
	}

	resultBytes, err := json.Marshal(conf.RawPrevResult)
	if err != nil {
		return fmt.Errorf("could not serialize prevResult: %w", err)
	}

	conf.RawPrevResult = nil
	conf.PrevResult, err = create.Create(conf.CNIVersion, resultBytes)
	if err != nil {
		return fmt.Errorf("could not parse prevResult: %w", err)
	}

	return nil
}

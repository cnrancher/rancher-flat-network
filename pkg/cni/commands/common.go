package commands

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/macvlan"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/types"
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
		return nil, fmt.Errorf("failed to get links %v", err)
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

func createVLANOnHost(
	master string, MTU int, ifName string, vlanID int,
) (*types100.Interface, error) {
	rootNS, err := netns.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to get root network NS: %w", err)
	}
	defer rootNS.Close()

	m, err := netlink.LinkByName(master)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup master %q: %v", master, err)
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
		return nil, fmt.Errorf("failed to create VLAN on %q: %w", ifName, err)
	}

	if err := netlink.LinkSetUp(vlan); err != nil {
		netlink.LinkDel(vlan)
		return nil, fmt.Errorf("failed to setup vlan on %q: %w", ifName, err)
	}

	// Re-fetch vlan to get all properties/attributes
	contVlan, err := netlink.LinkByName(ifName)
	if err != nil {
		netlink.LinkDel(vlan)
		return nil, fmt.Errorf("failed to refetch vlan on %q: %w", ifName, err)
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

	logrus.Infof("addEth0CustomRoutes===")
	err := netns.Do(func(_ ns.NetNS) error {
		rs, err := netlink.RouteList(nil, netlink.FAMILY_V4)
		if err != nil {
			logrus.Debugf("%v", err)
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
		return fmt.Errorf("addEth0CustomRoutes=== error %v", err)
	}
	return nil
}

func changeDefaultGateway(netns ns.NetNS, serviceCidr string, gateway net.IP) error {
	logrus.Infof("changeDefaultGateway: %s", gateway)
	err := netns.Do(func(_ ns.NetNS) error {
		routes, err := netlink.RouteList(nil, netlink.FAMILY_V4)
		if err != nil {
			logrus.Debugf("%v", err)
			return nil
		}

		originDefault := routes[0]
		// 1. delete default gateway
		err = netlink.RouteDel(&originDefault)
		if err != nil {
			return fmt.Errorf("RouteDel: %v", err)
		}

		// 2. add eth1 gateway
		eth1Route := originDefault
		eth1Route.LinkIndex = eth1Route.LinkIndex + 1
		eth1Route.Gw = gateway
		err = netlink.RouteAdd(&eth1Route)
		if err != nil {
			return fmt.Errorf("RouteAdd: %v", err)
		}
		// 3. add serviceCidr route
		clusterRoute := originDefault
		_, dst, err := net.ParseCIDR(serviceCidr)
		if err != nil {
			return fmt.Errorf("ParseCIDR: %v", err)
		}
		clusterRoute.Dst = dst
		err = netlink.RouteAdd(&clusterRoute)
		if err != nil {
			return fmt.Errorf("RouteAdd: %v", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("changeDefaultGateway: %v", err)
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

	if link.Attrs().Promisc != 1 {
		err = netlink.SetPromiscOn(link)
		if err != nil {
			return fmt.Errorf("netlink.SetPromiscOn failed on iface %q: %w", iface, err)
		}
	}
	return nil
}

func mergeIPAMConfig(
	netConf *types.NetConf, flatNetworkIP *flv1.FlatNetworkIP, subnet *flv1.FlatNetworkSubnet,
) ([]byte, error) {
	address := flatNetworkIP.Status.Addr
	routes, gateway := subnet.Spec.Routes, subnet.Spec.Gateway
	enableIPv6 := flatNetworkIP.Annotations[flv1.AnnotationsIPv6to4] != ""
	netConf.FlatNetworkConfig.IPAM.Addresses = []types.Address{
		{
			Address: address,
			Gateway: gateway,
		},
	}

	// add 6to4 address for IPv6
	// https://en.wikipedia.org/wiki/6to4
	if enableIPv6 {
		_, n, err := net.ParseCIDR(subnet.Spec.CIDR)
		if err != nil {
			return nil, fmt.Errorf("failed to parse subnet CIDR [%v]: %w",
				subnet.Spec.CIDR, err)
		}
		size, _ := n.Mask.Size()
		if ip6to4CIDR := get6to4CIDR(address, size); len(ip6to4CIDR) != 0 {
			netConf.FlatNetworkConfig.IPAM.Addresses = append(netConf.FlatNetworkConfig.IPAM.Addresses, types.Address{
				Address: ip6to4CIDR,
			})
		}
	}

	if len(routes) != 0 {
		rs := []*cnitypes.Route{}
		for _, v := range routes {
			if v.Iface != "" && v.Iface != "eth1" {
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
		netConf.FlatNetworkConfig.IPAM.Routes = rs
	}
	return json.Marshal(netConf)
}

func get6to4CIDR(ip net.IP, size int) net.IP {
	if ip = ip.To4(); ip == nil {
		return nil
	}
	if size <= 0 || size > 32 {
		return nil
	}
	sixtofourSize := 48 - (32 - size)
	tmp := []byte{}
	for _, v := range ip.To4() {
		tmp = append(tmp, v)
	}
	if len(tmp) != 4 {
		return nil
	}
	return net.ParseIP(fmt.Sprintf("2002:%02x:%02x:0:0:0:0:0/%d",
		tmp[0]+tmp[1], tmp[2]+tmp[3], sixtofourSize))
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

func validateCniContainerInterface(intf types100.Interface, modeExpected string) error {
	var link netlink.Link
	var err error

	if intf.Name == "" {
		return fmt.Errorf("container interface name missing in prevResult: %v", intf.Name)
	}
	link, err = netlink.LinkByName(intf.Name)
	if err != nil {
		return fmt.Errorf("container Interface name in prevResult: %s not found", intf.Name)
	}
	if intf.Sandbox == "" {
		return fmt.Errorf("error: Container interface %s should not be in host namespace", link.Attrs().Name)
	}

	macv, isMacvlan := link.(*netlink.Macvlan)
	if !isMacvlan {
		return fmt.Errorf("error: Container interface %s not of type macvlan", link.Attrs().Name)
	}

	mode, err := macvlan.ModeFromString(modeExpected)
	if err != nil {
		return err
	}
	if macv.Mode != mode {
		currString, err := macvlan.ModeToString(macv.Mode)
		if err != nil {
			return err
		}
		confString, err := macvlan.ModeToString(mode)
		if err != nil {
			return err
		}
		return fmt.Errorf("container macvlan mode %s does not match expected value: %s", currString, confString)
	}

	if intf.Mac != "" {
		if intf.Mac != link.Attrs().HardwareAddr.String() {
			return fmt.Errorf("interface %s Mac %s doesn't match container Mac: %s", intf.Name, intf.Mac, link.Attrs().HardwareAddr)
		}
	}

	return nil
}

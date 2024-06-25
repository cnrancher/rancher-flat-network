package commands

import (
	"context"
	"fmt"
	"net"

	flv1 "github.com/cnrancher/rancher-flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/kubeclient"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/macvlan"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/types"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/utils"
	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	types100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	"github.com/j-keck/arping"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
)

const (
	arpNotifyPolicy = "arp_notify"
	arpingPolicy    = "arping"
)

func Add(args *skel.CmdArgs) error {
	logrus.Debugf("cmdAdd args: %v", utils.Print(args))

	n, err := utils.LoadCNINetConf(args.StdinData)
	if err != nil {
		return err
	}
	k8sArgs := &types.K8sArgs{}
	if err := cnitypes.LoadArgs(args.Args, k8sArgs); err != nil {
		return fmt.Errorf("failed to load k8s args: %w", err)
	}

	client, err := kubeclient.GetK8sClient(args.Path)
	if err != nil {
		return fmt.Errorf("failed to get kube client: %w", err)
	}

	podName := string(k8sArgs.K8S_POD_NAME)
	podNamespace := string(k8sArgs.K8S_POD_NAMESPACE)
	_, err = client.GetPod(context.TODO(), podNamespace, podName)
	if err != nil {
		return fmt.Errorf("failed to get pod [%v/%v]: %w",
			podNamespace, podName, err)
	}

	// The pod may just created and the IP is not allocated by flat-network-operator.
	var flatNetworkIP *flv1.FlatNetworkIP
	if err := retry.OnError(retry.DefaultBackoff, errors.IsNotFound, func() error {
		flatNetworkIP, err = client.GetFlatNetworkIP(context.TODO(), podNamespace, podName)
		if err != nil {
			logrus.Warnf("failed to get FlatNetworkIP [%v/%v]: %v",
				podNamespace, podName, err)
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to get FlatNetworkIP [%v/%v]: %w",
			podNamespace, podName, err)
	}

	subnet, err := client.GetFlatNetworkSubnet(context.TODO(), flatNetworkIP.Spec.Subnet)
	if err != nil {
		return fmt.Errorf("failed to get FlatNetworkSubnet: %w", err)
	}

	master := subnet.Spec.Master
	hostIfname := subnet.ObjectMeta.Name
	vlanID := subnet.Spec.VLAN

	podDefaultGatewayEnabled := subnet.Spec.PodDefaultGateway.Enable
	serviceCIDR := subnet.Spec.PodDefaultGateway.ServiceCIDR
	gateway := subnet.Spec.Gateway

	if err := utils.SetPromiscOn(master); err != nil {
		return fmt.Errorf("failed to set promisc on %v: %w", master, err)
	}
	vlanIface, err := getVlanIfaceOnHost(master, 0, vlanID)
	if err != nil {
		return fmt.Errorf("failed to create host vlan interface on %v %s.%d: %w",
			hostIfname, master, vlanID, err)
	}

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %w", args.Netns, err)
	}
	defer netns.Close()

	macvlanInterface, err := macvlan.Create(&macvlan.Options{
		Mode:   subnet.Spec.Mode,
		Master: vlanIface.Name,
		MTU:    n.FlatNetworkConfig.MTU,
		IfName: args.IfName,
		NetNS:  netns,
		MAC:    flatNetworkIP.Status.MAC,
	})
	if err != nil {
		return err
	}
	logrus.Infof("create macvlan iface for pod [%v:%v]: %v",
		podNamespace, podName, utils.Print(macvlanInterface))

	flatNetworkIP.Status.MAC, _ = net.ParseMAC(macvlanInterface.Mac)
	_, err = client.UpdateFlatNetworkIP(context.TODO(), podNamespace, flatNetworkIP)
	if err != nil {
		logrus.Errorf("failed to update IP: IPAM [%v]: %v",
			n.FlatNetworkConfig.IPAM.Type, err)
		err2 := netns.Do(func(_ ns.NetNS) error {
			return ip.DelLinkByName(macvlanInterface.Name)
			// return nil
		})
		if err2 != nil {
			logrus.Errorf("failed to updateMacvlanIP DelLinkByName: %v \n", err2)
		}
	}

	// TODO:
	// Delete link if err to avoid link leak in this ns
	defer func() {
		if err != nil {
			err2 := netns.Do(func(_ ns.NetNS) error {
				return ip.DelLinkByName(args.IfName)
			})
			if err2 != nil {
				logrus.Infof("ipam.ExecDel: %v %v\n", n.FlatNetworkConfig.IPAM.Type, err2)
			}
		}
	}()

	// run the IPAM plugin and get back the config to apply
	ipamConf, err := utils.MergeIPAMConfig(n, flatNetworkIP, subnet)
	if err != nil {
		return fmt.Errorf("failed to merge IPAM config on netConf [%v]: %w",
			utils.Print(n), err)
	}
	r, err := ipam.ExecAdd(n.FlatNetworkConfig.IPAM.Type, []byte(ipamConf))
	if err != nil {
		return fmt.Errorf("failed to execute ipam add, type: [%v] conf [%v]: %w",
			n.FlatNetworkConfig.IPAM.Type, string(ipamConf), err)
	}

	// TODO:
	// Invoke ipam del if err to avoid ip leak
	defer func() {
		if err != nil {
			ipam.ExecDel(n.FlatNetworkConfig.IPAM.Type, []byte(ipamConf))
		}
	}()

	// Convert whatever the IPAM result was into the types100 Result type
	result, err := types100.NewResultFromResult(r)
	if err != nil {
		return fmt.Errorf("failed to convert IPAM result from [%v]: %w",
			utils.Print(r), err)
	}

	if len(result.IPs) == 0 {
		return fmt.Errorf("IPAM plugin returned missing IP config")
	}
	result.Interfaces = []*types100.Interface{
		macvlanInterface,
	}
	for _, ipc := range result.IPs {
		// All addresses apply to the container macvlan interface
		ipc.Interface = types100.Int(0)
	}
	err = netns.Do(func(_ ns.NetNS) error {
		if n.FlatNetworkConfig.RuntimeConfig.ARPPolicy == arpNotifyPolicy {
			logrus.Debugf("setting up sysctl arp_notify: %s", args.IfName)
			_, _ = sysctl.Sysctl(fmt.Sprintf("net/ipv4/conf/%s/arp_notify", args.IfName), "1")
		}

		if n.FlatNetworkConfig.RuntimeConfig.ProxyARP {
			logrus.Debugf("setting up sysctl proxy_arp: %s", args.IfName)
			_, _ = sysctl.Sysctl(fmt.Sprintf("net/ipv4/conf/%s/proxy_arp", args.IfName), "1")
		}

		if err := ipam.ConfigureIface(args.IfName, result); err != nil {
			return fmt.Errorf("configure ip failed, error: %v, interface: %s, result: %+v",
				err, args.IfName, result)
		}

		if n.FlatNetworkConfig.RuntimeConfig.ARPPolicy == arpingPolicy {
			logrus.Debugf("sending arping request: %s", args.IfName)
			contVeth, err := net.InterfaceByName(args.IfName)
			if err != nil {
				return fmt.Errorf("failed to look up %q: %v", args.IfName, err)
			}
			for _, ipc := range result.IPs {
				if ipc.Address.IP.To4() != nil {
					_ = arping.GratuitousArpOverIface(ipc.Address.IP, *contVeth)
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("netns do failed, error: %v", err)
	}
	result.DNS = n.DNS
	err = addEth0CustomRoutes(netns, subnet.Spec.Routes)
	if err != nil {
		return fmt.Errorf("failed to add eth0 custom routes: %w", err)
	}

	// Skip change gw if using single nic macvlan
	if podDefaultGatewayEnabled && args.IfName == "eth1" {
		err = changeDefaultGateway(netns, serviceCIDR, gateway)
		if err != nil {
			return fmt.Errorf("failed to change default gateway: %w", err)
		}
	}

	if err := cnitypes.PrintResult(result, n.CNIVersion); err != nil {
		return fmt.Errorf("failed to print result: %w", err)
	}

	return nil
}

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

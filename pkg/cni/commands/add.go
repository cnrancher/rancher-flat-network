package commands

import (
	"context"
	"fmt"
	"net"

	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/ipvlan"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/kubeclient"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/logger"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/macvlan"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/types"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/utils"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	"github.com/j-keck/arping"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"

	flv1 "github.com/cnrancher/rancher-flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	types100 "github.com/containernetworking/cni/pkg/types/100"
)

const (
	arpNotifyPolicy = "arp_notify"
	arpingPolicy    = "arping"
)

func Add(args *skel.CmdArgs) error {
	if err := logger.Setup(); err != nil {
		return err
	}
	logrus.Debugf("cmdAdd args: %v", utils.Print(args))

	n, err := loadCNINetConf(args.StdinData)
	if err != nil {
		return err
	}
	logrus.Debugf("cniNetConf: %v", utils.Print(n))
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
	// The pod may just created and the IP is not allocated by operator.
	// Retry to wait a few seconds to let Operator allocate IP for pod.
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

	// Set host master card to promiscuous mode.
	// TODO: Add custom option for master iface promiscuous mode.
	if err := setPromiscOn(subnet.Spec.Master); err != nil {
		return fmt.Errorf("failed to set promisc on %v: %w", subnet.Spec.Master, err)
	}

	// Create/Get vlan interface on host network namespace.
	// If the vlan ID is not 0, it will create a vlan iface [master].[vlanID]
	// (eth0.1 for example) to transmit data in VLAN.
	vlanIface, err := getVlanIfaceOnHost(subnet.Spec.Master, 0, subnet.Spec.VLAN)
	if err != nil {
		return fmt.Errorf("failed to create host vlan of subnet [%v] on iface [%s.%d]: %w",
			subnet.Name, subnet.Spec.Master, subnet.Spec.VLAN, err)
	}
	logrus.Infof("host vlan interface: %v", utils.Print(vlanIface))

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %w", args.Netns, err)
	}
	defer netns.Close()

	// Start flatnetwork iface creation
	var iface *types100.Interface
	switch subnet.Spec.FlatMode {
	case FlatModeMacvlan:
		// Create Macvlan network interface in container network namespace.
		iface, err = macvlan.Create(&macvlan.Options{
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
	case FlatModeIPvlan:
		// Create IPvlan network interface in container network namespace.
		iface, err = ipvlan.Create(&ipvlan.Options{
			Mode:   subnet.Spec.Mode,
			Master: vlanIface.Name,
			MTU:    n.FlatNetworkConfig.MTU,
			IfName: args.IfName,
			NetNS:  netns,
			MAC:    flatNetworkIP.Status.MAC,
		})
	default:
		return fmt.Errorf("unsupported flat mode [%v], only [%v, %v] supported",
			subnet.Spec.FlatMode, FlatModeMacvlan, FlatModeIPvlan)
	}

	logrus.Infof("create flat network [%v] iface [%v] for pod [%v:%v]: %v",
		subnet.Spec.FlatMode, iface.Name, podNamespace, podName, utils.Print(iface))

	// TODO: Update FlatNetworkIP status MAC address if needed
	// flatNetworkIP.Status.MAC, _ = net.ParseMAC(iface.Mac)
	// _, err = client.UpdateFlatNetworkIP(context.TODO(), podNamespace, flatNetworkIP)
	// if err != nil {
	// 	logrus.Errorf("failed to update flatNetworkIP: IPAM [%v]: %v",
	// 		n.IPAM.Type, err)
	// 	err2 := netns.Do(func(_ ns.NetNS) error {
	// 		return ip.DelLinkByName(iface.Name)
	// 	})
	// 	if err2 != nil {
	// 		logrus.Errorf("failed to uodate FlatNetworkIP DelLinkByName: %v \n", err2)
	// 	}
	// 	return err
	// }
	// logrus.Infof("update flatNetwork IP status MAC [%v]",
	// 	flatNetworkIP.Status.MAC.String())

	// Delete link if err to avoid link leak in this ns
	defer func() {
		if err != nil {
			err2 := netns.Do(func(_ ns.NetNS) error {
				return ip.DelLinkByName(args.IfName)
			})
			if err2 != nil {
				logrus.Infof("ipam.ExecDel: %v %v\n", n.IPAM.Type, err2)
			}
		}
	}()

	// run the IPAM plugin and get back the config to apply
	ipamConf, err := mergeIPAMConfig(n, flatNetworkIP, subnet)
	if err != nil {
		return fmt.Errorf("failed to merge IPAM config on netConf [%v]: %w",
			utils.Print(n), err)
	}
	logrus.Debugf("merged IPAM config: %v", string(ipamConf))
	r, err := ipam.ExecAdd(n.IPAM.Type, ipamConf)
	if err != nil {
		return fmt.Errorf("failed to execute ipam add, type: [%v] conf [%v]: %w",
			n.IPAM.Type, string(ipamConf), err)
	}

	// Invoke ipam del if err to avoid ip leak
	defer func() {
		if err != nil {
			ipam.ExecDel(n.IPAM.Type, ipamConf)
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
		iface,
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
					err := arping.GratuitousArpOverIface(ipc.Address.IP, *contVeth)
					if err != nil {
						logrus.Errorf("arping.GratuitousArpOverIface failed: %v", err)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("netns do failed, error: %w", err)
	}
	result.DNS = n.DNS
	err = addEth0CustomRoutes(netns, subnet.Spec.Routes)
	if err != nil {
		return fmt.Errorf("failed to add eth0 custom routes: %w", err)
	}

	// Skip change gw if using single nic macvlan
	if subnet.Spec.PodDefaultGateway.Enable && args.IfName == "eth1" {
		err = changeDefaultGateway(netns, subnet.Spec.PodDefaultGateway.ServiceCIDR, subnet.Spec.Gateway)
		if err != nil {
			return fmt.Errorf("failed to change default gateway: %w", err)
		}
	}

	if err := cnitypes.PrintResult(result, n.CNIVersion); err != nil {
		return fmt.Errorf("failed to print result: %w", err)
	}
	logrus.Infof("result: %v", utils.Print(result))

	return nil
}

package commands

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/cnrancher/rancher-flat-network/pkg/cni/common"
	"github.com/cnrancher/rancher-flat-network/pkg/cni/ipvlan"
	"github.com/cnrancher/rancher-flat-network/pkg/cni/kubeclient"
	"github.com/cnrancher/rancher-flat-network/pkg/cni/logger"
	"github.com/cnrancher/rancher-flat-network/pkg/cni/macvlan"
	"github.com/cnrancher/rancher-flat-network/pkg/cni/route"
	"github.com/cnrancher/rancher-flat-network/pkg/cni/types"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	"github.com/j-keck/arping"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	types100 "github.com/containernetworking/cni/pkg/types/100"
)

const (
	arpNotifyPolicy = "arp_notify"
	arpingPolicy    = "arping"
)

var (
	getPodRetry = wait.Backoff{
		Steps:    5,
		Duration: time.Second,
		Factor:   1.0,
		Jitter:   0.1,
	}
	errIPNotAllocated = fmt.Errorf("pod IP not allocated")
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
	if err := retry.OnError(getPodRetry, shouldRetryOnFlatNetworkIP, func() error {
		flatNetworkIP, err = client.GetIP(context.TODO(), podNamespace, podName)
		if err != nil {
			logrus.Warnf("failed to get FlatNetworkIP [%v/%v]: %v",
				podNamespace, podName, err)
			return err
		}
		if len(flatNetworkIP.Status.Addr) == 0 {
			logrus.Infof("FlatNetworkIP [%v/%v] address not allocated by operator, will retry...",
				podNamespace, podName)
			return errIPNotAllocated
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to get FlatNetworkIP [%v/%v]: %w",
			podNamespace, podName, err)
	}

	if flatNetworkIP == nil || len(flatNetworkIP.Status.Addr) == 0 {
		return fmt.Errorf("flatNetwork IP address not allocated")
	}
	logrus.Infof("flatNetworkIP [%v/%v] allocated address [%v]",
		flatNetworkIP.Namespace, flatNetworkIP.Name, flatNetworkIP.Status.Addr.String())

	subnet, err := client.GetSubnet(context.TODO(), flatNetworkIP.Spec.Subnet)
	if err != nil {
		return fmt.Errorf("failed to get FlatNetworkSubnet: %w", err)
	}

	/**
	 * FYI: https://github.com/moby/libnetwork/blob/c1865b811b6247cc0a52c4f7a253fc05372b3d89/docs/macvlan.md#macvlan-bridge-mode-example-usage
	 * Any Macvlan container sharing the same subnet can communicate via IP to
	 * any other container in the same subnet without a gateway. It is important
	 * to note, that the parent will go into promiscuous mode when a container
	 * is attached to the parent since each container has a unique MAC address.
	 * Alternatively, Ipvlan which is currently an experimental driver uses the
	 * same MAC address as the parent interface and thus precluding the need for
	 * the parent being promiscuous.
	 */
	switch subnet.Spec.FlatMode {
	case flv1.FlatModeMacvlan:
		if err = common.SetPromiscOn(subnet.Spec.Master); err != nil {
			return fmt.Errorf("failed to set promisc on %v: %w", subnet.Spec.Master, err)
		}
	case flv1.FlatModeIPvlan:
		// IPvlan does not need to set promiscuous mode enabled for master since
		// all sub-interfaces are using the same MAC address.
	}

	// Create/Get vlan interface on host network namespace.
	// If the vlan ID is not 0, it will create a vlan iface [master].[vlanID]
	// (eth0.100 for example) to separate broadcast domain.
	vlanIface, err := common.GetVlanIfaceOnHost(subnet.Spec.Master, 0, subnet.Spec.VLAN)
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
	case flv1.FlatModeMacvlan:
		iface, err = macvlan.Create(&macvlan.Options{
			Mode:   subnet.Spec.Mode,
			Master: vlanIface.Name,
			MTU:    n.FlatNetworkConfig.MTU,
			IfName: args.IfName,
			NetNS:  netns,
			MAC:    flatNetworkIP.Status.MAC,
		})
	case flv1.FlatModeIPvlan:
		iface, err = ipvlan.Create(&ipvlan.Options{
			Mode:   subnet.Spec.Mode,
			Master: vlanIface.Name,
			MTU:    n.FlatNetworkConfig.MTU,
			IfName: args.IfName,
			NetNS:  netns,
			MAC:    flatNetworkIP.Status.MAC,
		})
	default:
		err = fmt.Errorf("invalid flat mode [%v], only [%v, %v] supported",
			subnet.Spec.FlatMode, flv1.FlatModeMacvlan, flv1.FlatModeIPvlan)
	}
	if err != nil {
		return err
	}

	logrus.Infof("create flat network [%v] iface [%v] for pod [%v:%v]: %v",
		subnet.Spec.FlatMode, iface.Name, podNamespace, podName, utils.Print(iface))

	// Update flatNetworkIP status addr
	if err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		flatNetworkIP, err = client.GetIP(context.TODO(), podNamespace, podName)
		if err != nil {
			logrus.Warnf("failed to get FlatNetworkIP [%v/%v]: %v",
				podNamespace, podName, err)
			return err
		}

		flatNetworkIP = flatNetworkIP.DeepCopy()
		flatNetworkIP.Status.MAC = iface.Mac
		flatNetworkIP, err = client.UpdateIPStatus(context.TODO(), podNamespace, flatNetworkIP)
		return err
	}); err != nil {
		logrus.Errorf("failed to update flatNetworkIP: IPAM [%v]: %v",
			n.IPAM.Type, err)
		if err := netns.Do(func(_ ns.NetNS) error {
			return ip.DelLinkByName(iface.Name)
		}); err != nil {
			logrus.Errorf("ip.DelLinkByName failed: %v", err)
		}
		return err
	}
	logrus.Infof("update flatNetwork IP status MAC [%v]",
		flatNetworkIP.Status.MAC)

	// Delete link if err to avoid link leak in this ns
	defer func() {
		if err == nil {
			return
		}

		if err := netns.Do(func(_ ns.NetNS) error {
			return ip.DelLinkByName(args.IfName)
		}); err != nil {
			logrus.Errorf("ipam.ExecDel failed on IPAM type %v in NS [%v]: %v",
				n.IPAM.Type, int(netns.Fd()), err)
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

	// Invoke ipam del if err to avoid ip leak on default NS
	defer func() {
		if err == nil {
			return
		}
		if err := ipam.ExecDel(n.IPAM.Type, ipamConf); err != nil {
			logrus.Errorf("ipam.ExecDel failed on IPAM type %v in default NS: %v",
				n.IPAM.Type, err)
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

	// Add NodeCIDR route in Pod NS
	if subnet.Spec.RouteSettings.AddNodeCIDR {
		err = route.AddPodNodeCIDRRoutes(netns)
		if err != nil {
			return fmt.Errorf("route.AddPodNodeCIDRRoutes: %w", err)
		}
	}

	// Add FlatNetwork IP route to Pod on Host NS
	if subnet.Spec.RouteSettings.AddPodIPToHost {
		err = route.AddFlatNetworkRouteToHost(netns, flatNetworkIP.Status.Addr, vlanIface.Name)
		if err != nil {
			return fmt.Errorf("route.AddFlatNetworkRouteToHost: %w", err)
		}
	}

	// Skip change gw if using single NIC
	if subnet.Spec.RouteSettings.FlatNetworkDefaultGateway && args.IfName != common.PodIfaceEth0 {
		err = route.UpdatePodDefaultGateway(
			netns, args.IfName, flatNetworkIP.Status.Addr, subnet.Spec.Gateway)
		if err != nil {
			return fmt.Errorf("route.UpdatePodDefaultGateway: %w", err)
		}
	}

	// Add other user-defined custom routes
	if err := route.AddPodFlatNetworkCustomRoutes(netns, subnet.Spec.Routes); err != nil {
		return fmt.Errorf("failed to add custom routes: %w", err)
	}

	if err = cnitypes.PrintResult(result, n.CNIVersion); err != nil {
		return fmt.Errorf("failed to print result: %w", err)
	}
	logrus.Infof("result: %v", utils.Print(result))
	logrus.Infof("ADD: Done")

	return nil
}

func shouldRetryOnFlatNetworkIP(err error) bool {
	if apierrors.IsNotFound(err) {
		return true
	}
	if errors.Is(err, errIPNotAllocated) {
		return true
	}

	return false
}

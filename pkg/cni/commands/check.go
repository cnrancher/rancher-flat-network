package commands

import (
	"context"
	"fmt"

	flv1 "github.com/cnrancher/rancher-flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/kubeclient"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/logger"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/cni/types"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/utils"
	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	types100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
)

func Check(args *skel.CmdArgs) error {
	if err := logger.Setup(); err != nil {
		return err
	}
	logrus.Debugf("cmdCheck args: %v", utils.Print(args))

	n, err := loadCNINetConf(args.StdinData)
	if err != nil {
		logrus.Errorf("failed to load CNI config: %v", err)
		return err
	}
	logrus.Debugf("cniNetConf: %v", utils.Print(n))
	k8sArgs := &types.K8sArgs{}
	if err := cnitypes.LoadArgs(args.Args, k8sArgs); err != nil {
		return fmt.Errorf("failed to load k8s args: %w", err)
	}

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		logrus.Errorf("failed to open netns %q: %v", args.Netns, err)
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer netns.Close()

	client, err := kubeclient.GetK8sClient(args.Path)
	if err != nil {
		return fmt.Errorf("failed to get kube client: %w", err)
	}
	podName := string(k8sArgs.K8S_POD_NAME)
	podNamespace := string(k8sArgs.K8S_POD_NAMESPACE)

	// The pod may just created and the IP is not allocated by operator.
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

	// run the IPAM plugin and get back the config to apply
	err = ipam.ExecCheck(n.IPAM.Type, args.StdinData)
	if err != nil {
		return err
	}

	// Parse previous result.
	if n.RawPrevResult == nil {
		logrus.Errorf("RawPrevResult is nil")
		return fmt.Errorf("required prevResult missing")
	}

	if err := parsePrevResult(n); err != nil {
		return err
	}

	result, err := types100.NewResultFromResult(n.PrevResult)
	if err != nil {
		return err
	}
	logrus.Debugf("result: %v", utils.Print(result))

	var contMap types100.Interface
	// Find interfaces for names whe know, macvlan device name inside container
	for _, intf := range result.Interfaces {
		if args.IfName == intf.Name {
			if args.Netns == intf.Sandbox {
				contMap = *intf
				continue
			}
		}
	}

	// The namespace must be the same as what was configured
	if args.Netns != contMap.Sandbox {
		return fmt.Errorf("sandbox in prevResult %s doesn't match configured netns: %s",
			contMap.Sandbox, args.Netns)
	}

	_, err = netlink.LinkByName(subnet.Spec.Master)
	if err != nil {
		return fmt.Errorf("failed to lookup master %q: %v",
			subnet.Spec.Master, err)
	}

	// Check prevResults for ips, routes and dns against values found in the container
	if err := netns.Do(func(_ ns.NetNS) error {
		// Check interface against values found in the container
		err := validateCniContainerInterface(contMap, subnet.Spec.Mode)
		if err != nil {
			logrus.Errorf("validateCniContainerInterface failed: %v", err)
			return err
		}

		err = ip.ValidateExpectedInterfaceIPs(args.IfName, result.IPs)
		if err != nil {
			logrus.Errorf("validateExpectedInterfaceIPs failed: %v", err)
			return err
		}

		err = ip.ValidateExpectedRoute(result.Routes)
		if err != nil {
			logrus.Errorf("validateExpectedRoute failed: %v", err)
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

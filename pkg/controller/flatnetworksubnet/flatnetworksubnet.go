package flatnetworksubnet

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"reflect"
	"slices"
	"time"

	"github.com/cnrancher/rancher-flat-network/pkg/common"
	"github.com/cnrancher/rancher-flat-network/pkg/controller/wrangler"
	"github.com/cnrancher/rancher-flat-network/pkg/ipcalc"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	corecontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/core/v1"
	flcontroller "github.com/cnrancher/rancher-flat-network/pkg/generated/controllers/flatnetwork.pandaria.io/v1"
)

const (
	handlerName       = "rancher-flat-network-subnet"
	handlerRemoveName = "rancher-flat-network-subnet-remove"
)

const (
	subnetPendingPhase = ""
	subnetActivePhase  = "Active"
	subnetFailedPhase  = "Failed"
)

const (
	labelMaster   = "master"
	labelVlan     = "vlan"
	labelMode     = "mode"
	labelFlatMode = "flatMode"
)

type handler struct {
	subnetClient flcontroller.FlatNetworkSubnetClient
	subnetCache  flcontroller.FlatNetworkSubnetCache
	ipClient     flcontroller.FlatNetworkIPClient
	ipCache      flcontroller.FlatNetworkIPCache
	podClient    corecontroller.PodClient

	recorder record.EventRecorder

	subnetEnqueueAfter func(string, string, time.Duration)
	subnetEnqueue      func(string, string)
}

func Register(
	ctx context.Context,
	wctx *wrangler.Context,
) {
	h := &handler{
		subnetClient: wctx.FlatNetwork.FlatNetworkSubnet(),
		subnetCache:  wctx.FlatNetwork.FlatNetworkSubnet().Cache(),
		ipClient:     wctx.FlatNetwork.FlatNetworkIP(),
		ipCache:      wctx.FlatNetwork.FlatNetworkIP().Cache(),
		podClient:    wctx.Core.Pod(),

		recorder: wctx.Recorder,

		subnetEnqueueAfter: wctx.FlatNetwork.FlatNetworkSubnet().EnqueueAfter,
		subnetEnqueue:      wctx.FlatNetwork.FlatNetworkSubnet().Enqueue,
	}

	wctx.FlatNetwork.FlatNetworkSubnet().OnChange(ctx, handlerName, h.handleError(h.handleSubnet))
	wctx.FlatNetwork.FlatNetworkSubnet().OnRemove(ctx, handlerRemoveName, h.handleSubnetRemove)
}

func (h *handler) handleError(
	onChange func(string, *flv1.FlatNetworkSubnet) (*flv1.FlatNetworkSubnet, error),
) func(string, *flv1.FlatNetworkSubnet) (*flv1.FlatNetworkSubnet, error) {
	return func(key string, subnet *flv1.FlatNetworkSubnet) (*flv1.FlatNetworkSubnet, error) {
		var err error
		var message string

		subnet, err = onChange(key, subnet)
		if subnet == nil || subnet.DeletionTimestamp != nil {
			return subnet, err
		}
		if err != nil {
			logrus.WithFields(fieldsSubnet(subnet)).
				Errorf("%v", err)
			message = err.Error()
		}
		if subnet.Name == "" {
			return subnet, err
		}

		if subnet.Status.FailureMessage == message {
			return subnet, err
		}

		h.eventError(subnet, err)
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			subnet, err := h.subnetCache.Get(subnet.Namespace, subnet.Name)
			if err != nil {
				return err
			}
			subnet = subnet.DeepCopy()
			if message != "" {
				subnet.Status.Phase = subnetFailedPhase
			}
			subnet.Status.FailureMessage = message

			_, err = h.subnetClient.UpdateStatus(subnet)
			return err
		})
		if err != nil {
			logrus.WithFields(fieldsSubnet(subnet)).
				Errorf("Error recording subnet [%s] failure message: %v", subnet.Name, err)
		}
		return subnet, err
	}
}

func (h *handler) handleSubnet(
	_ string, subnet *flv1.FlatNetworkSubnet,
) (*flv1.FlatNetworkSubnet, error) {
	if subnet == nil {
		return nil, nil
	}
	if subnet.Name == "" || subnet.DeletionTimestamp != nil {
		return subnet, nil
	}

	switch subnet.Status.Phase {
	case subnetActivePhase:
		return h.onSubnetUpdate(subnet)
	default:
		return h.onSubnetCreate(subnet)
	}
}

func (h *handler) onSubnetCreate(subnet *flv1.FlatNetworkSubnet) (*flv1.FlatNetworkSubnet, error) {
	// Webhook may not working properly when creating subnets parallelly, need to double-check
	// subnet spec and ensure no conflicts with other subnets.
	if err := common.ValidateSubnet(subnet); err != nil {
		return subnet, err
	}
	set := map[string]string{
		labelMaster: subnet.Spec.Master,
		labelVlan:   fmt.Sprintf("%v", subnet.Spec.VLAN),
	}
	subnets, err := h.subnetCache.List(subnet.Namespace, labels.SelectorFromSet(set))
	if err != nil {
		return subnet, fmt.Errorf("failed to list subnet from cache by selector %q: %w", utils.Print(set), err)
	}
	// Ensure only one flatMode on iface
	if err := common.CheckSubnetFlatMode(subnet, subnets); err != nil {
		return subnet, err
	}
	// Ensure no subnet CIDR conflict
	if err := common.CheckSubnetConflict(subnet, subnets); err != nil {
		return subnet, err
	}

	if subnet.Namespace != flv1.SubnetNamespace {
		logrus.WithFields(fieldsSubnet(subnet)).
			Errorf("subnet [%v/%v] namespace should be [%v]", subnet.Namespace, subnet.Name,
				flv1.SubnetNamespace)
		return subnet, fmt.Errorf("invalid subnet namespace %q", subnet.Namespace)
	}
	// Update subnet labels.
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err := h.subnetCache.Get(subnet.Namespace, subnet.Name)
		if err != nil {
			logrus.WithFields(fieldsSubnet(subnet)).
				Errorf("failed to get subnet from cache: %v", err)
			return err
		}
		result = result.DeepCopy()
		if result.Labels == nil {
			result.Labels = make(map[string]string)
		}
		result.Labels["master"] = result.Spec.Master
		result.Labels["vlan"] = fmt.Sprintf("%v", result.Spec.VLAN)
		result.Labels["mode"] = result.Spec.Mode
		result.Labels["flatMode"] = result.Spec.FlatMode
		_, network, err := net.ParseCIDR(result.Spec.CIDR)
		if err != nil {
			return fmt.Errorf("failed to parse CIDR %q: %w",
				result.Spec.CIDR, err)
		}
		result.Spec.CIDR = network.String()
		result, err = h.subnetClient.Update(result)
		if err != nil {
			return err
		}
		logrus.WithFields(fieldsSubnet(subnet)).
			Infof("update subnet label %q: %v",
				subnet.Name, utils.Print(result.Labels))
		subnet = result
		return nil
	})
	if err != nil {
		return subnet, fmt.Errorf("failed to update label and gateway of subnet: %w", err)
	}

	var gatewayIP net.IP
	if subnet.Spec.Gateway == nil {
		gatewayIP, err = ipcalc.GetDefaultGateway(subnet.Spec.CIDR)
		if err != nil {
			return nil, fmt.Errorf("failed to get subnet default gateway IP: %w", err)
		}
	} else {
		gatewayIP = subnet.Spec.Gateway
	}

	// Update the flat-network subnet status.
	subnet = subnet.DeepCopy()
	subnet.Status.Phase = subnetActivePhase
	subnet.Status.UsedIP = ipcalc.AddIPToRange(subnet.Spec.Gateway, subnet.Status.UsedIP)
	subnet.Status.Gateway = gatewayIP
	subnetUpdate, err := h.subnetClient.UpdateStatus(subnet)
	if err != nil {
		return subnet, fmt.Errorf("failed to update status of subnet: %w", err)
	}
	subnet = subnetUpdate

	logrus.WithFields(fieldsSubnet(subnet)).
		Infof("update subnet [%v] status to active", subnet.Name)
	return subnet, nil
}

func (h *handler) onSubnetUpdate(subnet *flv1.FlatNetworkSubnet) (*flv1.FlatNetworkSubnet, error) {
	set := map[string]string{
		labelMaster: subnet.Spec.Master,
		labelVlan:   fmt.Sprintf("%v", subnet.Spec.VLAN),
	}
	subnets, err := h.subnetCache.List(subnet.Namespace, labels.SelectorFromSet(set))
	if err != nil {
		return subnet, fmt.Errorf("failed to list subnet from cache by selector %q: %w", utils.Print(set), err)
	}
	// Ensure only one flatMode on iface
	if err := common.CheckSubnetFlatMode(subnet, subnets); err != nil {
		return subnet, err
	}
	// Ensure no subnet CIDR conflict
	if err := common.CheckSubnetConflict(subnet, subnets); err != nil {
		return subnet, err
	}
	if subnet.Namespace != flv1.SubnetNamespace {
		logrus.WithFields(fieldsSubnet(subnet)).
			Errorf("subnet [%v/%v] namespace should be [%v]", subnet.Namespace, subnet.Name,
				flv1.SubnetNamespace)
		return subnet, fmt.Errorf("invalid subnet namespace %q", subnet.Namespace)
	}

	// Sync this subnet in every 10 minutes.
	defer h.subnetEnqueueAfter(subnet.Namespace, subnet.Name, time.Minute*10)

	// Update subnet labels.
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err := h.subnetCache.Get(subnet.Namespace, subnet.Name)
		if err != nil {
			logrus.WithFields(fieldsSubnet(subnet)).
				Errorf("failed to get subnet from cache: %v", err)
			return err
		}
		result = result.DeepCopy()
		if result.Labels == nil {
			result.Labels = make(map[string]string)
		}
		result.Labels[labelMaster] = result.Spec.Master
		result.Labels[labelVlan] = fmt.Sprintf("%v", result.Spec.VLAN)
		result.Labels[labelMode] = result.Spec.Mode
		result.Labels[labelFlatMode] = result.Spec.FlatMode
		_, network, err := net.ParseCIDR(result.Spec.CIDR)
		if err != nil {
			return fmt.Errorf("failed to parse CIDR %q: %w",
				result.Spec.CIDR, err)
		}
		result.Spec.CIDR = network.String()
		result, err = h.subnetClient.Update(result)
		if err != nil {
			return err
		}
		if reflect.DeepEqual(result.Labels, subnet.Labels) &&
			result.Spec.CIDR == subnet.Spec.CIDR {
			// Skip if already updated
			return nil
		}

		result, err = h.subnetClient.Update(result)
		if err != nil {
			return err
		}
		logrus.WithFields(fieldsSubnet(subnet)).
			Infof("update subnet label %q: %v",
				subnet.Name, utils.Print(result.Labels))
		subnet = result
		return nil
	})
	if err != nil {
		return subnet, fmt.Errorf("failed to update label and gateway of subnet: %w", err)
	}

	// List IPs using this subnet.
	ips, err := h.ipCache.List("", labels.SelectorFromSet(labels.Set{
		"subnet": subnet.Name,
	}))
	if err != nil {
		return subnet, fmt.Errorf("failed to list IP from cache: %w", err)
	}

	// Cleanup the duplicated IPs using this subnet.
	duplicatedIPs := filterDuplicatedIP(ips)
	if len(duplicatedIPs) != 0 {
		return subnet, h.cleanupDuplicatedIPs(subnet, ips)
	}

	return subnet, nil
}

func filterDuplicatedIP(ips []*flv1.FlatNetworkIP) []*flv1.FlatNetworkIP {
	duplicatedIPs := []*flv1.FlatNetworkIP{}
	if len(ips) == 0 {
		return duplicatedIPs
	}
	set := map[string]bool{}
	for _, ip := range ips {
		if ip == nil || len(ip.Status.Addr) == 0 {
			continue
		}
		a := ip.Status.Addr.String()
		if !set[a] {
			set[a] = true
			continue
		}
		duplicatedIPs = append(duplicatedIPs, ip)
	}
	return duplicatedIPs
}

func (h *handler) cleanupDuplicatedIPs(subnet *flv1.FlatNetworkSubnet, ips []*flv1.FlatNetworkIP) error {
	if len(ips) == 0 {
		return nil
	}

	for _, ip := range ips {
		logrus.WithFields(fieldsSubnet(subnet)).
			Warnf("found duplicated pod IP [%v], will delete",
				ip.Status.Addr.String())
		err := h.podClient.Delete(ip.Namespace, ip.Name, &metav1.DeleteOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				return fmt.Errorf("failed to delete pod [%v/%v]: %w",
					ip.Namespace, ip.Name, err)
			}
		}
		logrus.WithFields(fieldsSubnet(subnet)).
			Infof("request to delete pod have duplicated IP [%v/%v]: %v",
				ip.Namespace, ip.Name, ip.Status.Addr.String())

		err = h.ipClient.Delete(ip.Namespace, ip.Name, &metav1.DeleteOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				return fmt.Errorf("failed to delete flatNetworkIP [%v/%v]: %w",
					ip.Namespace, ip.Name, err)
			}
		}
		logrus.WithFields(fieldsSubnet(subnet)).
			Infof("request to delete duplicated IP [%v/%v]: %v",
				ip.Namespace, ip.Name, ip.Status.Addr.String())
	}
	return nil
}

func ip2UsedRanges(ips []*flv1.FlatNetworkIP) []flv1.IPRange {
	var usedIPs []flv1.IPRange
	if len(ips) == 0 {
		return usedIPs
	}
	slices.SortFunc(ips, func(a, b *flv1.FlatNetworkIP) int {
		if a.Status.Addr == nil || b.Status.Addr == nil {
			return -1
		}
		return bytes.Compare(a.Status.Addr, b.Status.Addr)
	})
	for _, ip := range ips {
		usedIPs = ipcalc.AddIPToRange(ip.Status.Addr, usedIPs)
	}
	return usedIPs
}

func (h *handler) eventError(subnet *flv1.FlatNetworkSubnet, err error) {
	if err == nil {
		return
	}
	h.recorder.Event(subnet, corev1.EventTypeWarning, "FlatNetworkSubnetError", err.Error())
}

func fieldsSubnet(subnet *flv1.FlatNetworkSubnet) logrus.Fields {
	if subnet == nil {
		return logrus.Fields{}
	}
	return logrus.Fields{
		"GID":    utils.GID(),
		"Subnet": fmt.Sprintf("%v", subnet.Name),
	}
}

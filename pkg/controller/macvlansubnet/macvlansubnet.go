package macvlansubnet

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"slices"
	"time"

	"github.com/cnrancher/flat-network-operator/pkg/controller/wrangler"
	"github.com/cnrancher/flat-network-operator/pkg/ipcalc"
	"github.com/cnrancher/flat-network-operator/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	macvlancontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/macvlan.cluster.cattle.io/v1"
)

const (
	handlerName       = "flatnetwork-macvlansubnet"
	handlerRemoveName = "flatnetwork-macvlansubnet-remove"
)

const (
	macvlanSubnetPendingPhase = ""
	macvlanSubnetActivePhase  = "Active"
	macvlanSubnetFailedPhase  = "Failed"
)

type handler struct {
	macvlanSubnetClient macvlancontroller.MacvlanSubnetClient
	macvlanSubnetCache  macvlancontroller.MacvlanSubnetCache
	macvlanIPCache      macvlancontroller.MacvlanIPCache

	recorder record.EventRecorder

	macvlansubnetEnqueueAfter func(string, string, time.Duration)
	macvlansubnetEnqueue      func(string, string)
}

func Register(
	ctx context.Context,
	wctx *wrangler.Context,
) {
	h := &handler{
		macvlanSubnetClient: wctx.Macvlan.MacvlanSubnet(),
		macvlanSubnetCache:  wctx.Macvlan.MacvlanSubnet().Cache(),
		macvlanIPCache:      wctx.Macvlan.MacvlanIP().Cache(),

		macvlansubnetEnqueueAfter: wctx.Macvlan.MacvlanSubnet().EnqueueAfter,
		macvlansubnetEnqueue:      wctx.Macvlan.MacvlanSubnet().Enqueue,
	}

	wctx.Macvlan.MacvlanSubnet().OnChange(ctx, handlerName, h.handleError(h.handleMacvlanSubnet))
	wctx.Macvlan.MacvlanSubnet().OnRemove(ctx, handlerRemoveName, h.handleMacvlanSubnetRemove)
}

func (h *handler) handleError(
	onChange func(string, *macvlanv1.MacvlanSubnet) (*macvlanv1.MacvlanSubnet, error),
) func(string, *macvlanv1.MacvlanSubnet) (*macvlanv1.MacvlanSubnet, error) {
	return func(key string, subnet *macvlanv1.MacvlanSubnet) (*macvlanv1.MacvlanSubnet, error) {
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

		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			subnet, err := h.macvlanSubnetCache.Get(subnet.Namespace, subnet.Name)
			if err != nil {
				return err
			}
			subnet = subnet.DeepCopy()
			if message != "" {
				subnet.Status.Phase = macvlanSubnetFailedPhase
			}
			subnet.Status.FailureMessage = message

			_, err = h.macvlanSubnetClient.UpdateStatus(subnet)
			return err
		})
		if err != nil {
			logrus.WithFields(fieldsSubnet(subnet)).
				Errorf("Error recording macvlan subnet [%s] failure message: %v", subnet.Name, err)
		}
		return subnet, err
	}
}

func (h *handler) handleMacvlanSubnet(
	_ string, subnet *macvlanv1.MacvlanSubnet,
) (*macvlanv1.MacvlanSubnet, error) {
	if subnet == nil {
		return nil, nil
	}
	if subnet.Name == "" || subnet.DeletionTimestamp != nil {
		return subnet, nil
	}
	result, err := h.macvlanSubnetCache.Get(subnet.Namespace, subnet.Name)
	if err != nil {
		logrus.WithFields(fieldsSubnet(subnet)).
			Errorf("failed to get subnet from cache: %v", err)
		return subnet, err
	}
	subnet = result

	switch subnet.Status.Phase {
	case macvlanSubnetActivePhase:
		return h.onMacvlanSubnetUpdate(subnet)
	default:
		return h.onMacvlanSubnetCreate(subnet)
	}
}

func (h *handler) onMacvlanSubnetCreate(subnet *macvlanv1.MacvlanSubnet) (*macvlanv1.MacvlanSubnet, error) {
	// Update macvlan subnet labels.
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err := h.macvlanSubnetCache.Get(macvlanv1.SubnetNamespace, subnet.Name)
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
		result, err = h.macvlanSubnetClient.Update(result)
		if err != nil {
			logrus.WithFields(fieldsSubnet(subnet)).
				Warnf("failed to update subnet %q: %v", subnet.Name, err)
			return err
		}
		logrus.WithFields(fieldsSubnet(subnet)).
			Infof("update macvlan subnet label %q: %v",
				subnet.Name, utils.PrintObject(result.Labels))
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
			return nil, fmt.Errorf("failed to get macvlan subnet default gateway IP: %w", err)
		}
	} else {
		gatewayIP = subnet.Spec.Gateway
	}

	// Update the flat-network subnet status.
	subnet = subnet.DeepCopy()
	subnet.Status.Phase = macvlanSubnetActivePhase
	subnet.Status.UsedIP = ipcalc.AddIPToRange(subnet.Spec.Gateway, subnet.Status.UsedIP)
	subnet.Status.Gateway = gatewayIP
	subnetUpdate, err := h.macvlanSubnetClient.UpdateStatus(subnet)
	if err != nil {
		return subnet, fmt.Errorf("failed to update status of subnet: %w", err)
	}
	subnet = subnetUpdate

	logrus.WithFields(fieldsSubnet(subnet)).
		Infof("update subnet [%v] status to active", subnet.Name)
	return subnet, nil
}

func (h *handler) onMacvlanSubnetUpdate(subnet *macvlanv1.MacvlanSubnet) (*macvlanv1.MacvlanSubnet, error) {
	// List macvlanIPs using this subnet.
	ips, err := h.macvlanIPCache.List("", labels.SelectorFromSet(labels.Set{
		"subnet": subnet.Name,
	}))
	if err != nil {
		return subnet, fmt.Errorf("failed to list macvlanIP from cache: %w", err)
	}

	// Sync this subnet in every 10 minutes.
	defer h.macvlansubnetEnqueueAfter(subnet.Namespace, subnet.Name, time.Minute*10)

	var gatewayIP net.IP = subnet.Status.Gateway
	if gatewayIP == nil {
		if subnet.Spec.Gateway == nil {
			gatewayIP, err = ipcalc.GetDefaultGateway(subnet.Spec.CIDR)
			if err != nil {
				return nil, fmt.Errorf("failed to get gateway IP from subnet: %w", err)
			}
		} else {
			gatewayIP = subnet.Spec.Gateway
		}
	}

	usedIPs := ip2UsedRanges(ips)
	usedIPs = ipcalc.AddIPToRange(gatewayIP, usedIPs)
	if equality.Semantic.DeepEqual(usedIPs, subnet.Status.UsedIP) &&
		subnet.Status.UsedIPCount == len(ips) &&
		gatewayIP.Equal(subnet.Status.Gateway) {
		logrus.WithFields(fieldsSubnet(subnet)).
			Debugf("subnet [%v] usedIP status already update", subnet.Name)
		return subnet, nil
	}
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		result, err := h.macvlanSubnetCache.Get(subnet.Namespace, subnet.Name)
		if err != nil {
			return fmt.Errorf("failed to get subnet from cache: %w", err)
		}
		result = result.DeepCopy()
		if equality.Semantic.DeepEqual(usedIPs, result.Status.UsedIP) && result.Status.UsedIPCount == len(ips) {
			return nil
		}
		result.Status.UsedIP = usedIPs
		result.Status.UsedIPCount = len(ips) + 1 // PodIPs & Gateway IP
		result.Status.Gateway = gatewayIP
		result, err = h.macvlanSubnetClient.UpdateStatus(result)
		if err != nil {
			return fmt.Errorf("failed to update subnet status: %w", err)
		}
		subnet = result
		return nil
	})
	if err != nil {
		return subnet, fmt.Errorf("failed to update subnet status: %w", err)
	}
	logrus.WithFields(fieldsSubnet(subnet)).
		Infof("update subnet [%v] usedIPCount [%v]",
			subnet.Name, subnet.Status.UsedIPCount)
	return subnet, nil
}

func ip2UsedRanges(ips []*macvlanv1.MacvlanIP) []macvlanv1.IPRange {
	var usedIPs []macvlanv1.IPRange
	if len(ips) == 0 {
		return usedIPs
	}
	slices.SortFunc(ips, func(a, b *macvlanv1.MacvlanIP) int {
		if a.Status.IP == nil || b.Status.IP == nil {
			return -1
		}
		return bytes.Compare(a.Status.IP, b.Status.IP)
	})
	for _, ip := range ips {
		usedIPs = ipcalc.AddIPToRange(ip.Status.IP, usedIPs)
	}
	return usedIPs
}

func (h *handler) eventError(pod *corev1.Pod, err error) {
	h.recorder.Event(pod, corev1.EventTypeWarning, "FlatNetworkSubnetError", err.Error())
}

func fieldsSubnet(subnet *macvlanv1.MacvlanSubnet) logrus.Fields {
	if subnet == nil {
		return logrus.Fields{}
	}
	return logrus.Fields{
		"GID":    utils.GetGID(),
		"SUBNET": fmt.Sprintf("%v", subnet.Name),
	}
}

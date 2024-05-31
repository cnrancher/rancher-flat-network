package macvlansubnet

import (
	"context"
	"fmt"
	"time"

	"github.com/cnrancher/flat-network-operator/pkg/controller/wrangler"
	"github.com/cnrancher/flat-network-operator/pkg/ipcalc"
	"github.com/cnrancher/flat-network-operator/pkg/utils"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/util/retry"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	corecontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/core/v1"
	macvlancontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/macvlan.cluster.cattle.io/v1"
)

const (
	controllerName       = "macvlansubnet"
	controllerRemoveName = "macvlansubnet-remove"
)

const (
	macvlanSubnetPendingPhase = ""
	macvlanSubnetActivePhase  = "Active"
	macvlanSubnetFailedPhase  = "Failed"

	subnetMacvlanIPCountAnnotation = "macvlanipCount"
	subnetGatewayCacheValue        = "subnet gateway ip"
)

type handler struct {
	macvlanSubnetClient macvlancontroller.MacvlanSubnetClient
	macvlanSubnetCache  macvlancontroller.MacvlanSubnetCache
	podCache            corecontroller.PodCache

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
		podCache:            wctx.Core.Pod().Cache(),

		macvlansubnetEnqueueAfter: wctx.Macvlan.MacvlanSubnet().EnqueueAfter,
		macvlansubnetEnqueue:      wctx.Macvlan.MacvlanSubnet().Enqueue,
	}

	wctx.Macvlan.MacvlanSubnet().OnChange(ctx, controllerName, h.handleSubnetError(h.handleSubnetChanged))
	wctx.Macvlan.MacvlanSubnet().OnRemove(ctx, controllerName, h.handleSubnetRemoved)
}

func (h *handler) handleSubnetError(
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
			subnet, err := h.macvlanSubnetClient.Get(macvlanv1.MacvlanSubnetNamespace, subnet.Name, metav1.GetOptions{})
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

func (h *handler) handleSubnetChanged(
	_ string, subnet *macvlanv1.MacvlanSubnet,
) (*macvlanv1.MacvlanSubnet, error) {
	if subnet == nil {
		return nil, nil
	}
	if subnet.Name == "" || subnet.DeletionTimestamp != nil {
		return subnet, nil
	}

	switch subnet.Status.Phase {
	case macvlanSubnetActivePhase:
		return h.updateMacvlanSubnet(subnet)
	default:
		return h.createMacvlanSubnet(subnet)
	}
}

func (h *handler) createMacvlanSubnet(subnet *macvlanv1.MacvlanSubnet) (*macvlanv1.MacvlanSubnet, error) {
	logrus.WithFields(fieldsSubnet(subnet)).
		Infof("create macvlan subnet [%v]", subnet.Name)

	// Update macvlan subnet labels and set status phase to pending.
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err := h.macvlanSubnetCache.Get(macvlanv1.MacvlanSubnetNamespace, subnet.Name)
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
		if result.Spec.Gateway == nil {
			gatewayIP, err := ipcalc.GetDefaultGateway(result.Spec.CIDR)
			if err != nil {
				return fmt.Errorf("failed to get macvlan subnet default gateway IP: %w", err)
			}
			result.Spec.Gateway = gatewayIP
		}
		result, err = h.macvlanSubnetClient.Update(result)
		if err != nil {
			logrus.WithFields(fieldsSubnet(subnet)).
				Warnf("Failed to update subnet %q: %v", subnet.Name, err)
			return err
		}
		logrus.WithFields(fieldsSubnet(subnet)).
			Infof("updated macvlan subnet label %q: %v",
				subnet.Name, utils.PrintObject(result.Labels))
		subnet = result
		return nil
	})
	if err != nil {
		return subnet, fmt.Errorf("failed to update label and gateway of subnet: %w", err)
	}

	// Add the gateway ip to syncmap.
	if subnet.Spec.Gateway == nil {
		return subnet, fmt.Errorf("subnet %q gateway should not empty", subnet.Name)
	}
	// TODO: Gateway IP conflict check step is not needed.
	// // Gateway IP conflic check.
	// if ipcalc.IPInRanges(subnet.Spec.Gateway, subnet.Spec.Ranges) {
	// 	return subnet, fmt.Errorf("subnet gateway IP conflict")
	// }

	// Update the macvlan subnet status phase to active.
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err := h.macvlanSubnetClient.Get(macvlanv1.MacvlanSubnetNamespace, subnet.Name, metav1.GetOptions{})
		if err != nil {
			logrus.WithFields(fieldsSubnet(subnet)).
				Warnf("Failed to get latest version of subnet: %v", err)
			return err
		}
		result = result.DeepCopy()
		result.Status.Phase = macvlanSubnetActivePhase
		result.Status.UsedIP = ipcalc.AddIPToRange(result.Spec.Gateway, result.Status.UsedIP)
		result, err = h.macvlanSubnetClient.UpdateStatus(result)
		if err != nil {
			return err
		}
		subnet = result
		return nil
	})
	if err != nil {
		return subnet, fmt.Errorf("failed to update status of subnet: %w", err)
	}
	return subnet, nil
}

func (h *handler) updateMacvlanSubnet(subnet *macvlanv1.MacvlanSubnet) (*macvlanv1.MacvlanSubnet, error) {
	// Update macvlanip count of the subnet by updating the subnet label.
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err := h.macvlanSubnetCache.Get(macvlanv1.MacvlanSubnetNamespace, subnet.Name)
		if err != nil {
			logrus.WithFields(fieldsSubnet(subnet)).
				Errorf("failed to get subnet from cache: %v", err)
			return err
		}
		result = result.DeepCopy()
		if result.Annotations == nil {
			result.Annotations = make(map[string]string)
		}

		// Get the pod count.
		pods, err := h.podCache.List("", labels.SelectorFromSet(map[string]string{
			"subnet": result.Name,
		}))
		if err != nil {
			logrus.WithFields(fieldsSubnet(subnet)).
				Debugf("Failed to get pod list of subnet %q: %v", result.Name, err)
			return err
		}
		count := fmt.Sprintf("%v", len(pods))
		if result.Annotations[subnetMacvlanIPCountAnnotation] == count {
			return nil
		}
		result.Annotations[subnetMacvlanIPCountAnnotation] = count
		result, err = h.macvlanSubnetClient.Update(result)
		if err != nil {
			return err
		}
		logrus.WithFields(fieldsSubnet(subnet)).
			Infof("update subnet annotation [%v: %v]",
				subnetMacvlanIPCountAnnotation, count)
		subnet = result
		return nil
	})
	if err != nil {
		return subnet, fmt.Errorf("updateSubnet: failed to update ip count of subnet %q: %v", subnet.Name, err)
	}

	// FIXME: this step consumes performance, consider to remove it.
	// Sync the subnet every 10 secs.
	h.macvlansubnetEnqueueAfter(subnet.Namespace, subnet.Name, time.Second*10)
	return subnet, nil
}

func fieldsSubnet(subnet *macvlanv1.MacvlanSubnet) logrus.Fields {
	if subnet == nil {
		return logrus.Fields{}
	}
	return logrus.Fields{
		"SUBNET": fmt.Sprintf("%v", subnet.Name),
	}
}

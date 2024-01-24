package controller

import (
	"fmt"
	"strings"
	"time"

	macvlanv1 "github.com/cnrancher/macvlan-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	"github.com/cnrancher/macvlan-operator/pkg/ipcalc"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

const (
	macvlanSubnetInitPhase    = ""
	macvlanSubnetPendingPhase = "Pending"
	macvlanSubnetActivePhase  = "Active"
	macvlanSubnetFailedPhase  = "Failed"

	subnetMacvlanIPCountAnnotation = "macvlanipCount"
	subnetGatewayCacheValue        = "subnet gateway ip"
)

func (h *Handler) handleMacvlanSubnetError(
	onChange func(string, *macvlanv1.MacvlanSubnet) (*macvlanv1.MacvlanSubnet, error),
) func(string, *macvlanv1.MacvlanSubnet) (*macvlanv1.MacvlanSubnet, error) {
	return func(key string, subnet *macvlanv1.MacvlanSubnet) (*macvlanv1.MacvlanSubnet, error) {
		var err error
		var message string

		subnet, err = onChange(key, subnet)
		if subnet == nil {
			// Macvlan subnet resource is likely deleting.
			return subnet, err
		}
		if err != nil {
			logrus.Warnf("%v", err)
			message = err.Error()
		}
		if subnet.Name == "" {
			return subnet, err
		}

		if subnet.Status.FailureMessage == message {
			// Avoid trigger the rate limit.
			if message != "" {
				time.Sleep(time.Second * 5)
			}
			return subnet, err
		}

		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			subnet, err := h.macvlanSubnets.Get(macvlanv1.MacvlanSubnetNamespace, subnet.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			subnet = subnet.DeepCopy()
			if message != "" {
				// can assume an update is failing
				subnet.Status.Phase = macvlanSubnetFailedPhase
			}
			subnet.Status.FailureMessage = message

			_, err = h.macvlanSubnets.UpdateStatus(subnet)
			return err
		})
		if err != nil {
			logrus.Errorf("Error recording macvlan subnet config [%s] failure message: %v", subnet.Name, err)
		}
		return subnet, err
	}
}

func (h *Handler) onMacvlanSubnetRemove(_ string, subnet *macvlanv1.MacvlanSubnet) (*macvlanv1.MacvlanSubnet, error) {
	return subnet, nil
}

func (h *Handler) onMacvlanSubnetChanged(
	_ string,
	subnet *macvlanv1.MacvlanSubnet,
) (*macvlanv1.MacvlanSubnet, error) {
	if subnet == nil {
		return nil, nil
	}
	if subnet.Name == "" || subnet.DeletionTimestamp != nil {
		return subnet, nil
	}

	switch subnet.Status.Phase {
	case macvlanSubnetInitPhase,
		macvlanSubnetPendingPhase,
		macvlanSubnetFailedPhase:
		return h.createSubnet(subnet)
	case macvlanSubnetActivePhase:
		return h.updateSubnet(subnet)
	}

	return nil, nil
}

func (h *Handler) createSubnet(subnet *macvlanv1.MacvlanSubnet) (*macvlanv1.MacvlanSubnet, error) {
	// Update macvlan subnet labels and set status phase to pending.
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err := h.macvlanSubnets.Get(macvlanv1.MacvlanSubnetNamespace, subnet.Name, metav1.GetOptions{})
		if err != nil {
			logrus.Debugf("Failed to get latest version of subnet: %v", err)
			return err
		}
		result = result.DeepCopy()
		if result.Labels == nil {
			result.Labels = make(map[string]string)
		}
		result.Labels["master"] = result.Spec.Master
		result.Labels["vlan"] = fmt.Sprintf("%v", result.Spec.VLAN)
		result.Labels["mode"] = result.Spec.Mode
		if result.Spec.Gateway == "" {
			gatewayIP, err := ipcalc.CalcDefaultGateway(result.Spec.CIDR)
			if err != nil {
				return fmt.Errorf("failed to get macvlan subnet default gateway IP: %w", err)
			}
			result.Spec.Gateway = gatewayIP.String()
		}
		result.Status.Phase = macvlanSubnetPendingPhase

		result, err = h.macvlanSubnets.Update(result)
		if err != nil {
			logrus.Warnf("Failed to update subnet %q: %v", subnet.Name, err)
			return err
		}
		logrus.Infof("Update the label of subnet %q", subnet.Name)
		subnet = result
		return nil
	})
	if err != nil {
		return subnet, fmt.Errorf("createSubnet: failed to update label and gateway of subnet: %w", err)
	}

	// Add the gateway ip to syncmap.
	if subnet.Spec.Gateway == "" {
		return subnet, fmt.Errorf("createSubnet: subnet %q gateway should not empty", subnet.Name)
	}
	key := fmt.Sprintf("%s:%s", subnet.Spec.Gateway, subnet.Name)
	owner, ok := h.inUsedIPs.Load(key)
	if ok && owner != subnetGatewayCacheValue {
		// Delete the pod who used this gateway ip
		temp := strings.SplitN(owner.(string), ":", 2) // TODO: remove temp
		pod, err := h.pods.Get(temp[0], temp[1], metav1.GetOptions{})

		// Do not delete the pod directly, return the subnet gateway IP conflict error.
		if err == nil && pod != nil && pod.DeletionTimestamp == nil && pod.Annotations[macvlanv1.AnnotationSubnet] == subnet.Name {
			err := fmt.Errorf("gateway IP %v is used by pod [%v/%v]", subnet.Spec.Gateway, pod.Namespace, pod.Name)
			logrus.Errorf("Failed to create subnet %q: %v", subnet.Name, err)
			return subnet, err
		}
	}
	h.inUsedIPs.Store(key, subnetGatewayCacheValue)
	logrus.Infof("createSubnet: Add gateway %s to syncmap cache", key)

	// Update the macvlan subnet status phase to active.
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err := h.macvlanSubnets.Get(macvlanv1.MacvlanSubnetNamespace, subnet.Name, metav1.GetOptions{})
		if err != nil {
			logrus.Warnf("Failed to get latest version of subnet: %v", err)
			return err
		}
		result.Status.Phase = macvlanSubnetActivePhase
		result, err = h.macvlanSubnets.UpdateStatus(result)
		if err != nil {
			return err
		}
		subnet = result
		return nil
	})
	if err != nil {
		return subnet, fmt.Errorf("createSubnet: failed to update status of subnet: %w", err)
	}
	return subnet, nil
}

func (h *Handler) updateSubnet(subnet *macvlanv1.MacvlanSubnet) (*macvlanv1.MacvlanSubnet, error) {
	// Update macvlanip count of the subnet by updating the subnet label.
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err := h.macvlanSubnets.Get(macvlanv1.MacvlanSubnetNamespace, subnet.Name, metav1.GetOptions{})
		if err != nil {
			logrus.Debugf("Failed to get latest version of subnet: %v", err)
			return err
		}
		result = result.DeepCopy()
		if result.Annotations == nil {
			result.Annotations = make(map[string]string)
		}

		// Get the pod count.
		pods, err := h.pods.List("", metav1.ListOptions{
			LabelSelector: fmt.Sprintf("subnet=%s", result.Name),
		})
		if err != nil {
			logrus.Debugf("Failed to get pod list of subnet %q: %v", result.Name, err)
			return err
		}
		count := fmt.Sprintf("%v", len(pods.Items))
		if result.Annotations[subnetMacvlanIPCountAnnotation] == count {
			return nil
		}
		result.Annotations[subnetMacvlanIPCountAnnotation] = count

		result, err = h.macvlanSubnets.Update(result)
		if err != nil {
			logrus.Warnf("Failed to update subnet ip count: %v", err)
			return err
		}
		subnet = result
		return nil
	})
	if err != nil {
		return subnet, fmt.Errorf("updateSubnet: failed to update ip count of subnet %q: %v", subnet.Name, err)
	}

	// Sync the subnet every 10 secs.
	h.subnetEnqueueAfter(subnet.Namespace, subnet.Name, time.Second*10)
	return subnet, nil
}

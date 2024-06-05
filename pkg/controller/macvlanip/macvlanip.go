package macvlanip

import (
	"context"
	"fmt"
	"net"
	"slices"
	"sync"
	"time"

	"github.com/cnrancher/flat-network-operator/pkg/controller/wrangler"
	"github.com/cnrancher/flat-network-operator/pkg/ipcalc"
	"github.com/cnrancher/flat-network-operator/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	corecontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/core/v1"
	macvlancontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/macvlan.cluster.cattle.io/v1"
)

const (
	handlerName       = "flatnetwork-macvlanip"
	handlerRemoveName = "flatnetwork-macvlanip-remove"
)

const (
	macvlanIPInitPhase    = ""
	macvlanIPPendingPhase = "Pending"
	macvlanIPActivePhase  = "Active"
	macvlanIPFailedPhase  = "Failed"
)

type handler struct {
	macvlanIPClient     macvlancontroller.MacvlanIPClient
	macvlanIPCache      macvlancontroller.MacvlanIPCache
	macvlanSubnetClient macvlancontroller.MacvlanSubnetClient
	macvlanSubnetCache  macvlancontroller.MacvlanSubnetCache
	podClient           corecontroller.PodClient
	podCache            corecontroller.PodCache

	macvlanipEnqueueAfter func(string, string, time.Duration)
	macvlanipEnqueue      func(string, string)

	podEnqueueAfter func(string, string, time.Duration)
	podEnqueue      func(string, string)

	recorder record.EventRecorder

	// allocateMutex is the mutex for allocating IP & MAC address from subnet.
	// FIXME: Use leader election lock instead of memory mutex!
	allocateMutex sync.Mutex
}

func Register(
	ctx context.Context,
	wctx *wrangler.Context,
) {
	h := &handler{
		macvlanIPClient:     wctx.Macvlan.MacvlanIP(),
		macvlanIPCache:      wctx.Macvlan.MacvlanIP().Cache(),
		macvlanSubnetClient: wctx.Macvlan.MacvlanSubnet(),
		macvlanSubnetCache:  wctx.Macvlan.MacvlanSubnet().Cache(),
		podClient:           wctx.Core.Pod(),
		podCache:            wctx.Core.Pod().Cache(),
		recorder:            wctx.Recorder,

		macvlanipEnqueueAfter: wctx.Macvlan.MacvlanIP().EnqueueAfter,
		macvlanipEnqueue:      wctx.Macvlan.MacvlanSubnet().Enqueue,

		podEnqueueAfter: wctx.Core.Pod().EnqueueAfter,
		podEnqueue:      wctx.Core.Pod().Enqueue,
	}

	wctx.Macvlan.MacvlanIP().OnChange(ctx, handlerName, h.handleError(h.handleMacvlanIP))
	wctx.Macvlan.MacvlanIP().OnRemove(ctx, handlerRemoveName, h.handleMacvlanIPRemove)
}

func (h *handler) handleError(
	onChange func(string, *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error),
) func(string, *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	return func(key string, ip *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
		var message string
		var err error
		ip, err = onChange(key, ip)
		if err != nil {
			logrus.WithFields(fieldsIP(ip)).
				Error(err)
			message = err.Error()
		}
		if ip == nil {
			return ip, err
		}
		if ip.Name == "" {
			return ip, err
		}

		if ip.Status.FailureMessage == message {
			return ip, err
		}

		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			ip, err := h.macvlanIPCache.Get(ip.Namespace, ip.Name)
			if err != nil {
				return err
			}
			ip = ip.DeepCopy()
			if message != "" {
				ip.Status.Phase = macvlanIPFailedPhase
			}
			ip.Status.FailureMessage = message

			_, err = h.macvlanIPClient.UpdateStatus(ip)
			return err
		})
		if err != nil {
			logrus.Errorf("error recording macvlan IP [%s] failure message: %v", ip.Name, err)
			return ip, err
		}
		return ip, nil
	}
}

func (h *handler) handleMacvlanIP(s string, ip *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	if ip == nil || ip.Name == "" || ip.DeletionTimestamp != nil {
		return ip, nil
	}
	result, err := h.macvlanIPCache.Get(ip.Namespace, ip.Name)
	if err != nil {
		return ip, fmt.Errorf("failed to get macvlanIP from cache: %v", err)
	}
	ip = result

	switch ip.Status.Phase {
	case macvlanIPActivePhase:
		return h.onMacvlanIPUpdate(ip)
	default:
		return h.onMacvlanIPCreate(ip)
	}
}

func (h *handler) onMacvlanIPCreate(ip *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	// Ensure the pod exists.
	pod, err := h.podCache.Get(ip.Namespace, ip.Name)
	if err != nil {
		return ip, fmt.Errorf("onMacvlanIPCreate: failed to get pod [%v/%v]: %w",
			ip.Namespace, ip.Name, err)
	}
	if pod.UID != types.UID(ip.Spec.PodID) {
		logrus.WithFields(fieldsIP(ip)).
			Warnf("ip.PodID [%v] is not same with pod.metadata.uid [%v]",
				ip.Spec.PodID, pod.UID)
	}

	// FIXME: Use leader election lock instead of memory mutex!
	h.allocateMutex.Lock()
	defer h.allocateMutex.Unlock()

	// Ensure the macvlan subnet resource exists.
	subnet, err := h.macvlanSubnetCache.Get(macvlanv1.SubnetNamespace, ip.Spec.Subnet)
	if err != nil {
		return ip, fmt.Errorf("onMacvlanIPCreate: failed to get subnet [%v] of ip [%v/%v]: %w",
			ip.Spec.Subnet, ip.Namespace, ip.Name, err)
	}

	allocatedIP, err := allocateIP(ip, subnet)
	if err != nil {
		logrus.WithFields(fieldsIP(ip)).
			Errorf("failed to allocate IP address: %v", err)
		h.eventError(pod, err)
		return ip, err
	}
	allocatedMAC, err := allocateMAC(ip, subnet)
	if err != nil {
		logrus.WithFields(fieldsIP(ip)).
			Errorf("failed to allocate MAC address: %v", err)
		return ip, err
	}

	// Update subnet status.
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		result, err := h.macvlanSubnetCache.Get(macvlanv1.SubnetNamespace, ip.Spec.Subnet)
		if err != nil {
			logrus.WithFields(fieldsIP(ip)).
				Errorf("failed to get subnet from cache: %v", err)
			return err
		}
		result = result.DeepCopy()
		result.Status.UsedIP = ipcalc.AddIPToRange(allocatedIP, result.Status.UsedIP)
		result.Status.UsedIPCount++
		if allocatedMAC != nil {
			result.Status.UsedMac = append(result.Status.UsedMac, allocatedMAC)
		}
		result, err = h.macvlanSubnetClient.UpdateStatus(result)
		if err != nil {
			return err
		}
		subnet = result
		return err
	})
	if err != nil {
		return ip, fmt.Errorf("failed to update subnet [%v] status: %w",
			subnet.Name, err)
	}

	// Update macvlanIP status to active.
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		result, err := h.macvlanIPCache.Get(ip.Namespace, ip.Name)
		if err != nil {
			logrus.Errorf("failed to get macvlanIP from cache: %v", err)
			return err
		}

		result = result.DeepCopy()
		result.Status.IP = allocatedIP
		result.Status.MAC = allocatedMAC
		result.Status.Phase = macvlanIPActivePhase
		result, err = h.macvlanIPClient.UpdateStatus(result)
		if err != nil {
			return err
		}

		ip = result
		return nil
	})
	if err != nil {
		// Fallback subnet status.
		subnet = subnet.DeepCopy()
		subnet.Status.UsedIP = ipcalc.RemoveIPFromRange(allocatedIP, subnet.Status.UsedIP)
		subnet.Status.UsedIPCount--
		if len(allocatedMAC) != 0 && len(subnet.Status.UsedMac) != 0 {
			subnet.Status.UsedMac = slices.DeleteFunc(subnet.Status.UsedMac, func(a net.HardwareAddr) bool {
				return a.String() == allocatedIP.String()
			})
		}
		subnet, err = h.macvlanSubnetClient.UpdateStatus(subnet)
		if err != nil {
			logrus.Warnf("failed to update subnet [%v] status: %v",
				subnet.Name, err)
		}
		return ip, fmt.Errorf("failed to update IP [%v/%v] addr status: %w",
			ip.Namespace, ip.Name, err)
	}

	macString := ip.Status.MAC.String()
	if macString == "" {
		macString = "auto"
	}
	logrus.WithFields(fieldsIP(ip)).
		Infof("allocated IP subnet [%v] MAC [%v] address [%v]",
			ip.Spec.Subnet, macString, ip.Status.IP.String())

	return ip, nil
}

func (h *handler) onMacvlanIPUpdate(ip *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	// Ensure the macvlan subnet resource exists.
	subnet, err := h.macvlanSubnetCache.Get(macvlanv1.SubnetNamespace, ip.Spec.Subnet)
	if err != nil {
		return ip, fmt.Errorf("onMacvlanIPUpdate: failed to get subnet [%v] of ip [%v/%v]: %w",
			ip.Spec.Subnet, ip.Namespace, ip.Name, err)
	}

	if alreadyAllocateIP(ip, subnet) && alreadyAllocatedMAC(ip) {
		logrus.WithFields(fieldsIP(ip)).
			Debugf("macvlanIP already updated")
		return ip, nil
	}

	logrus.WithFields(fieldsIP(ip)).
		Infof("macvlanIP changes detected, will re-allocate IP & MAC addr")
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		result, err := h.macvlanIPCache.Get(ip.Namespace, ip.Name)
		if err != nil {
			return fmt.Errorf("failed to get macvlanIP from cache: %w", err)
		}

		result = result.DeepCopy()
		result.Status.Phase = macvlanIPPendingPhase
		result, err = h.macvlanIPClient.UpdateStatus(result)
		if err != nil {
			return err
		}
		ip = result
		return nil
	})
	if err != nil {
		logrus.WithFields(fieldsIP(ip)).
			Errorf("failed to update macvlanIP status: %v", err)
		return ip, err
	}

	h.macvlanipEnqueue(ip.Namespace, ip.Name)
	h.podEnqueueAfter(ip.Namespace, ip.Name, time.Second*5)
	return ip, nil
}

func (h *handler) eventError(pod *corev1.Pod, err error) {
	h.recorder.Event(pod, corev1.EventTypeWarning, "FlatNetworkIPError", err.Error())
}

func fieldsIP(ip *macvlanv1.MacvlanIP) logrus.Fields {
	if ip == nil {
		return logrus.Fields{}
	}
	return logrus.Fields{
		"GID": utils.GetGID(),
		"IP":  fmt.Sprintf("%v/%v", ip.Namespace, ip.Name),
	}
}

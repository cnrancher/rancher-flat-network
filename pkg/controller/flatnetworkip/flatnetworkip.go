package flatnetworkip

import (
	"context"
	"fmt"
	"net"
	"slices"
	"sync"
	"time"

	"github.com/cnrancher/rancher-flat-network-operator/pkg/controller/wrangler"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/ipcalc"
	"github.com/cnrancher/rancher-flat-network-operator/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

	flv1 "github.com/cnrancher/rancher-flat-network-operator/pkg/apis/flatnetwork.pandaria.io/v1"
	corecontroller "github.com/cnrancher/rancher-flat-network-operator/pkg/generated/controllers/core/v1"
	flcontroller "github.com/cnrancher/rancher-flat-network-operator/pkg/generated/controllers/flatnetwork.pandaria.io/v1"
)

const (
	handlerName       = "rancher-flat-network-ip"
	handlerRemoveName = "rancher-flat-network-ip-remove"
)

const (
	flatNetworkIPInitPhase    = ""
	flatNetworkIPPendingPhase = "Pending"
	flatNetworkIPActivePhase  = "Active"
	flatNetworkIPFailedPhase  = "Failed"
)

type handler struct {
	ipClient     flcontroller.FlatNetworkIPClient
	ipCache      flcontroller.FlatNetworkIPCache
	subnetClient flcontroller.FlatNetworkSubnetClient
	subnetCache  flcontroller.FlatNetworkSubnetCache
	podClient    corecontroller.PodClient
	podCache     corecontroller.PodCache

	ipEnqueueAfter func(string, string, time.Duration)
	ipEnqueue      func(string, string)

	subnetEnqueueAfter func(string, string, time.Duration)
	subnetEnqueue      func(string, string)

	podEnqueueAfter func(string, string, time.Duration)
	podEnqueue      func(string, string)

	recorder record.EventRecorder

	// Mutex for allocating IP address.
	allocateIPMutex sync.Mutex
}

func Register(
	ctx context.Context,
	wctx *wrangler.Context,
) {
	h := &handler{
		ipClient:     wctx.FlatNetwork.FlatNetworkIP(),
		ipCache:      wctx.FlatNetwork.FlatNetworkIP().Cache(),
		subnetClient: wctx.FlatNetwork.FlatNetworkSubnet(),
		subnetCache:  wctx.FlatNetwork.FlatNetworkSubnet().Cache(),
		podClient:    wctx.Core.Pod(),
		podCache:     wctx.Core.Pod().Cache(),
		recorder:     wctx.Recorder,

		ipEnqueueAfter: wctx.FlatNetwork.FlatNetworkIP().EnqueueAfter,
		ipEnqueue:      wctx.FlatNetwork.FlatNetworkSubnet().Enqueue,

		subnetEnqueueAfter: wctx.FlatNetwork.FlatNetworkSubnet().EnqueueAfter,
		subnetEnqueue:      wctx.FlatNetwork.FlatNetworkSubnet().Enqueue,

		podEnqueueAfter: wctx.Core.Pod().EnqueueAfter,
		podEnqueue:      wctx.Core.Pod().Enqueue,
	}

	wctx.FlatNetwork.FlatNetworkIP().OnChange(ctx, handlerName, h.handleError(h.handleIP))
	wctx.FlatNetwork.FlatNetworkIP().OnRemove(ctx, handlerRemoveName, h.handleIPRemove)
}

func (h *handler) handleError(
	onChange func(string, *flv1.FlatNetworkIP) (*flv1.FlatNetworkIP, error),
) func(string, *flv1.FlatNetworkIP) (*flv1.FlatNetworkIP, error) {
	return func(key string, ip *flv1.FlatNetworkIP) (*flv1.FlatNetworkIP, error) {
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
		h.eventError(ip, err)

		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			ip, err := h.ipCache.Get(ip.Namespace, ip.Name)
			if err != nil {
				return err
			}
			ip = ip.DeepCopy()
			if message != "" {
				ip.Status.Phase = flatNetworkIPFailedPhase
			}
			ip.Status.FailureMessage = message

			_, err = h.ipClient.UpdateStatus(ip)
			return err
		})
		if err != nil {
			logrus.Errorf("error recording flat-network IP [%s] failure message: %v", ip.Name, err)
			return ip, err
		}
		return ip, nil
	}
}

func (h *handler) handleIP(_ string, ip *flv1.FlatNetworkIP) (*flv1.FlatNetworkIP, error) {
	if ip == nil || ip.Name == "" || ip.DeletionTimestamp != nil {
		return ip, nil
	}
	result, err := h.ipCache.Get(ip.Namespace, ip.Name)
	if err != nil {
		return ip, fmt.Errorf("failed to get IP from cache: %v", err)
	}
	ip = result

	switch ip.Status.Phase {
	case flatNetworkIPActivePhase:
		return h.onIPUpdate(ip)
	default:
		return h.onIPCreate(ip)
	}
}

func (h *handler) onIPCreate(ip *flv1.FlatNetworkIP) (*flv1.FlatNetworkIP, error) {
	// Ensure the pod exists.
	pod, err := h.podCache.Get(ip.Namespace, ip.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ip, h.ipClient.Delete(ip.Namespace, ip.Name, &metav1.DeleteOptions{})
		}
		return ip, fmt.Errorf("onIPCreate: failed to get pod [%v/%v]: %w",
			ip.Namespace, ip.Name, err)
	}
	if pod.UID != types.UID(ip.Spec.PodID) {
		logrus.WithFields(fieldsIP(ip)).
			Warnf("ip.PodID [%v] is not same with pod.metadata.uid [%v]",
				ip.Spec.PodID, pod.UID)
	}

	h.allocateIPMutex.Lock()
	defer h.allocateIPMutex.Unlock()

	// Ensure the flat-network subnet resource exists.
	subnet, err := h.subnetCache.Get(flv1.SubnetNamespace, ip.Spec.Subnet)
	if err != nil {
		return ip, fmt.Errorf("onIPCreate: failed to get subnet [%v] of ip [%v/%v]: %w",
			ip.Spec.Subnet, ip.Namespace, ip.Name, err)
	}

	allocatedIP, err := allocateIP(ip, subnet)
	if err != nil {
		logrus.WithFields(fieldsIP(ip)).
			Errorf("failed to allocate IP address: %v", err)
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
		result, err := h.subnetCache.Get(flv1.SubnetNamespace, ip.Spec.Subnet)
		if err != nil {
			logrus.WithFields(fieldsIP(ip)).
				Errorf("failed to get subnet from cache: %v", err)
			return err
		}
		result = result.DeepCopy()
		result.Status.UsedIP = ipcalc.AddIPToRange(allocatedIP, result.Status.UsedIP)
		result.Status.UsedIPCount++
		if allocatedMAC != nil {
			result.Status.UsedMAC = append(result.Status.UsedMAC, allocatedMAC)
		}
		result, err = h.subnetClient.UpdateStatus(result)
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

	// Update IP status to active.
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		result, err := h.ipCache.Get(ip.Namespace, ip.Name)
		if err != nil {
			logrus.Errorf("failed to get IP from cache: %v", err)
			return err
		}

		result = result.DeepCopy()
		result.Status.Addr = allocatedIP
		result.Status.MAC = allocatedMAC
		result.Status.Phase = flatNetworkIPActivePhase
		result, err = h.ipClient.UpdateStatus(result)
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
		if len(allocatedMAC) != 0 && len(subnet.Status.UsedMAC) != 0 {
			subnet.Status.UsedMAC = slices.DeleteFunc(subnet.Status.UsedMAC, func(a net.HardwareAddr) bool {
				return a.String() == allocatedIP.String()
			})
		}
		subnet, err = h.subnetClient.UpdateStatus(subnet)
		if err != nil {
			logrus.WithFields(fieldsIP(ip)).
				Warnf("failed to update (fallback) subnet [%v] status: %v",
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
			ip.Spec.Subnet, macString, ip.Status.Addr.String())

	return ip, nil
}

func (h *handler) onIPUpdate(ip *flv1.FlatNetworkIP) (*flv1.FlatNetworkIP, error) {
	// Ensure the subnet resource exists.
	subnet, err := h.subnetCache.Get(flv1.SubnetNamespace, ip.Spec.Subnet)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = h.ipClient.Delete(ip.Namespace, ip.Name, &metav1.DeleteOptions{})
			return ip, err
		}
		return ip, fmt.Errorf("onIPUpdate: failed to get subnet [%v] of ip [%v/%v]: %w",
			ip.Spec.Subnet, ip.Namespace, ip.Name, err)
	}

	// Ensure the pod exists and UID matches
	pod, err := h.podCache.Get(ip.Namespace, ip.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = h.ipClient.Delete(ip.Namespace, ip.Name, &metav1.DeleteOptions{})
			return ip, err
		}
		return ip, fmt.Errorf("onIPUpdate: failed to get pod [%v/%v] from cache: %w",
			ip.Namespace, ip.Name, err)
	}
	if pod.UID != types.UID(ip.Spec.PodID) {
		err = h.ipClient.Delete(ip.Namespace, ip.Name, &metav1.DeleteOptions{})
		return ip, err
	}

	if alreadyAllocateIP(ip, subnet) && alreadyAllocatedMAC(ip) {
		logrus.WithFields(fieldsIP(ip)).
			Debugf("IP already updated")
		return ip, nil
	}

	logrus.WithFields(fieldsIP(ip)).
		Infof("IP changes detected, will re-allocate IP & MAC addr")
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		result, err := h.ipCache.Get(ip.Namespace, ip.Name)
		if err != nil {
			return fmt.Errorf("failed to get IP from cache: %w", err)
		}

		result = result.DeepCopy()
		result.Status.Phase = flatNetworkIPPendingPhase
		result, err = h.ipClient.UpdateStatus(result)
		if err != nil {
			return err
		}
		ip = result
		return nil
	})
	if err != nil {
		logrus.WithFields(fieldsIP(ip)).
			Errorf("failed to update IP status: %v", err)
		return ip, err
	}
	return ip, nil
}

func (h *handler) eventError(ip *flv1.FlatNetworkIP, err error) {
	if err == nil {
		return
	}
	h.recorder.Event(ip, corev1.EventTypeWarning, "FlatNetworkIPError", err.Error())
}

func fieldsIP(ip *flv1.FlatNetworkIP) logrus.Fields {
	if ip == nil {
		return logrus.Fields{}
	}
	return logrus.Fields{
		"GID": utils.GID(),
		"IP":  fmt.Sprintf("%v/%v", ip.Namespace, ip.Name),
	}
}

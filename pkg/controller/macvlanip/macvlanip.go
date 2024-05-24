package macvlanip

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/controller/wrangler"
	corecontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/core/v1"
	macvlancontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/macvlan.cluster.cattle.io/v1"
	"github.com/cnrancher/flat-network-operator/pkg/utils"
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

	// IPMutex is the mutex for allocating IP address from subnet.
	IPMutex sync.Mutex
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

		macvlanipEnqueueAfter: wctx.Macvlan.MacvlanIP().EnqueueAfter,
		macvlanipEnqueue:      wctx.Macvlan.MacvlanSubnet().Enqueue,
	}

	wctx.Macvlan.MacvlanIP().OnChange(ctx, handlerName, h.handleMacvlanIPError(h.handleMacvlanIP))
	wctx.Macvlan.MacvlanIP().OnRemove(ctx, handlerRemoveName, h.handleMacvlanIPRemove)
}

func (h *handler) handleMacvlanIPError(
	onChange func(string, *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error),
) func(string, *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	return func(key string, ip *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
		var message string
		var err error
		ip, err = onChange(key, ip)
		if ip == nil {
			return ip, err
		}

		if err != nil {
			logrus.Warnf("%v", err)
			message = err.Error()
		}
		if ip.Name == "" {
			return ip, err
		}

		if ip.Status.FailureMessage == message {
			return ip, err
		}

		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			ip, err := h.macvlanIPClient.Get(ip.Namespace, ip.Name, metav1.GetOptions{})
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
			logrus.Errorf("Error recording macvlan IP config [%s] failure message: %v", ip.Name, err)
			return ip, err
		}
		return ip, nil
	}
}

func (h *handler) handleMacvlanIP(s string, ip *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	if ip == nil || ip.Name == "" || ip.DeletionTimestamp != nil {
		return ip, nil
	}

	switch ip.Status.Phase {
	case macvlanIPActivePhase:
		return h.onMacvlanIPUpdate(ip)
	default:
		return h.onMacvlanIPCreate(ip)
	}
}

func (h *handler) onMacvlanIPCreate(ip *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	// Ensure the macvlan subnet resource exists.
	subnet, err := h.macvlanSubnetCache.Get(macvlanv1.MacvlanSubnetNamespace, ip.Spec.Subnet)
	if err != nil {
		return ip, fmt.Errorf("onMacvlanIPCreate: failed to get subnet [%v] of ip [%v/%v]: %w",
			ip.Spec.Subnet, ip.Namespace, ip.Name, err)
	}
	// Ensure the pod exists.
	pod, err := h.podCache.Get(ip.Namespace, ip.Name)
	if err != nil {
		return ip, fmt.Errorf("onMacvlanIPCreate: failed to get pod [%v/%v]: %w",
			ip.Namespace, ip.Name, err)
	}
	// Allocate IP from subnet.
	switch {
	case ip.Spec.IP == "auto":
		err = h.allocateIPModeAuto(ip, pod, subnet)
	case utils.IsSingleIP(ip.Spec.IP):
		err = h.allocateIPModeSingle(ip, pod, subnet)
	case utils.IsMultipleIP(ip.Spec.IP):
		err = h.allocateIPModeMultiple(ip, pod, subnet)
	default:
		err = fmt.Errorf("invalid IP [%v] in macvlanIP [%v/%v]",
			ip.Spec.IP, ip.Namespace, ip.Name)
	}
	if err != nil {
		logrus.WithFields(fieldsIP(ip)).
			Errorf("failed to allocate IP address: %v", err)
		return ip, err
	}

	// Update macvlanIP status to active.
	ip = ip.DeepCopy()
	ip.Status.Phase = macvlanIPActivePhase
	ip, err = h.macvlanIPClient.UpdateStatus(ip)
	if err != nil {
		err = fmt.Errorf("onMacvlanIPCreate: failed to update macvlanip [%s/%s] status: %w",
			ip.Namespace, ip.Name, err)
		return ip, err
	}
	logrus.Infof("Create macvlanIP [%v/%v] Subnet [%vIf a Pod was deleted with a duplicate IP in badpods purging process, CIDR [%v] MAC [%v]",
		ip.Namespace, ip.Name, ip.Spec.Subnet, ip.Spec.CIDR, ip.Spec.MAC)

	return ip, nil
}

func (h *handler) onMacvlanIPUpdate(ip *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	// Re-add missing records in cache
	// If a Pod was deleted with a duplicate IP in badpods purging process,
	// it may cause the IP record to be lost in the cache
	key := fmt.Sprintf("%s:%s", strings.SplitN(ip.Spec.CIDR, "/", 2)[0], ip.Spec.Subnet)
	_ = key // TODO:
	// if _, ok := h.inUsedIPs.Load(key); !ok {
	// 	// use api client to get the latest resource version
	// 	// pod, _ := c.kubeClientset.CoreV1().Pods(ip.Namespace).Get(context.TODO(), ip.Name, metav1.GetOptions{})
	// 	pod, _ := h.pods.Get(ip.Namespace, ip.Name, metav1.GetOptions{})
	// 	if pod != nil && pod.DeletionTimestamp == nil && pod.Name != "" {
	// 		owner := fmt.Sprintf("%s:%s", pod.Namespace, pod.Name)
	// 		h.inUsedIPs.Store(key, owner)
	// 		logrus.Infof("updateMacvlanIP: re-add key %s value %s to the syncmap", key, owner)
	// 	}
	// }

	// TODO:
	// if oldip.ResourceVersion != ip.ResourceVersion && oldip.Spec.CIDR != ip.Spec.CIDR {
	// 	// remove the old record from cache
	// 	// to address the statfuleset pod case
	// 	oldkey := fmt.Sprintf("%s:%s", strings.SplitN(oldip.Spec.CIDR, "/", 2)[0], oldip.Spec.Subnet)
	// 	c.inUsedIPs.Delete(oldkey)
	// 	log.Infof("onMacvlanIPUpdate: remove key %s from syncmap as macvlanip record %s was updated", oldkey, ip.Name)
	// }

	// IP delayed release, only in auto mode
	if ip.Labels[macvlanv1.LabelMacvlanIPType] != "auto" {
		return ip, nil
	}

	// subnetName := ip.Labels["subnet"]
	// subnet, err := h.macvlanSubnets.Get(macvlanv1.MacvlanSubnetNamespace, subnetName, metav1.GetOptions{})
	// if err != nil {
	// 	logrus.Errorf("onMacvlanIPUpdate: %s subnet %s not exist", ip.Name, subnetName)
	// 	subnet = &macvlanv1.MacvlanSubnet{}
	// }
	// if ip.DeletionTimestamp != nil {
	// 	if ip.Annotations[macvlanv1.AnnotationIPDelayReuse] == "" {
	// 		c.updateDelayReuseTimestamp(ip, subnet.Spec.IPDelayReuse)
	// 		return
	// 	}
	// 	c.calcNeedRemoveDelayReuseMacvlanIP(ip)
	// 	return
	// }

	// if subnet.Spec.IPDelayReuse != 0 && !slices.Contains(ip.ObjectMeta.Finalizers, macvlanv1.FinalizerIPDelayReuse) {
	// 	c.updateIPDelayReuseFinalizer(ip)
	// }

	return ip, nil
}

func fieldsIP(ip *macvlanv1.MacvlanIP) logrus.Fields {
	if ip == nil {
		return logrus.Fields{}
	}
	return logrus.Fields{
		"IP": fmt.Sprintf("%v/%v", ip.Namespace, ip.Name),
	}
}

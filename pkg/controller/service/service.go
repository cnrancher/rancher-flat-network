package service

import (
	"context"
	"strings"
	"time"

	"github.com/cnrancher/flat-network-operator/pkg/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/util/retry"

	macvlanv1 "github.com/cnrancher/flat-network-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	corecontroller "github.com/cnrancher/flat-network-operator/pkg/generated/controllers/core/v1"
)

const (
	controllerName           = "service"
	controllerRemoveName     = "service-remove"
	macvlanServiceNameSuffix = "-macvlan"
)

type handler struct {
	serviceClient corecontroller.ServiceClient
	serviceCache  corecontroller.ServiceCache
	podCache      corecontroller.PodCache

	serviceEnqueueAfter func(string, string, time.Duration)
	serviceEnqueue      func(string, string)
}

func Register(
	ctx context.Context,
	services corecontroller.ServiceController,
	pods corecontroller.PodController,
) {
	h := &handler{
		serviceClient: services,
		serviceCache:  services.Cache(),
		podCache:      pods.Cache(),

		serviceEnqueueAfter: services.EnqueueAfter,
		serviceEnqueue:      services.Enqueue,
	}

	services.OnChange(ctx, controllerName, h.handleServiceError(h.syncService))
	var _ metav1.Object
}

func (h *handler) handleServiceError(
	onChange func(string, *corev1.Service) (*corev1.Service, error),
) func(string, *corev1.Service) (*corev1.Service, error) {
	// TODO: handle service retry
	return onChange
}

func (h *handler) syncService(name string, svc *corev1.Service) (*corev1.Service, error) {
	// macvlan svc creation disabled by annotation
	isMacvlanServiceDisabled := false
	// is this service a macvlan service
	isMacvlanService := false
	// macvlan was not enabled on owner workload
	isMacvlanPodEnabled := false

	// Check if the service owner disabled the macvlan svc creation.
	for _, owner := range svc.OwnerReferences {
		if strings.ToLower(owner.Kind) == "ingress" {
			// Discard ingress service since the ingress svc is handled in ingress.go.
			return svc, nil
		}
	}

	if len(svc.Spec.Selector) != 0 {
		pods, err := h.podCache.List(svc.Namespace, labels.SelectorFromSet(svc.Spec.Selector))
		if err != nil {
			logrus.Errorf("syncService: failed to list pod by selector [%v] on svc [%v/%v]: %v",
				svc.Spec.Selector, svc.Namespace, svc.Name, err)
			return svc, err
		}
		logrus.Debugf("syncService: list [%v] pods on selector [%+v]",
			len(pods), svc.Spec.Selector)
		if len(pods) != 0 {
			// Check if the pod of this svc enabled macvlan.
			for _, pod := range pods {
				if pod == nil {
					continue
				}
				if utils.IsMacvlanPod(pod) {
					logrus.Debugf("syncService: pod [%v/%v] of service [%v] enabled macvlan",
						pod.Namespace, pod.Name, svc.Name)
					isMacvlanPodEnabled = true
					break
				}
			}
			// Check if the pod of this svc disabled macvlan service by annotation.
			for _, pod := range pods {
				if pod == nil {
					continue
				}
				annotations := pod.Annotations
				if annotations != nil && annotations[macvlanv1.AnnotationMacvlanService] == "disabled" {
					logrus.Debugf("syncService: found %q disabled on pod [%v/%v]",
						macvlanv1.AnnotationMacvlanService, pod.Namespace, pod.Name)
					isMacvlanServiceDisabled = true
					break
				}
			}
		}
	}

	// Check if the service is a macvlan svc.
	if strings.HasSuffix(svc.Name, macvlanServiceNameSuffix) {
		_, err := h.serviceCache.Get(svc.Namespace, strings.TrimSuffix(svc.Name, macvlanServiceNameSuffix))
		if err == nil {
			isMacvlanService = true
			logrus.Debugf("syncService: service [%s/%s] is a macvlan svc",
				svc.Namespace, svc.Name)
		}
	}
	// Check if this macvlan svc needs delete.
	if isMacvlanService {
		if isMacvlanServiceDisabled || !isMacvlanPodEnabled {
			// Delete this macvlan svc.
			logrus.Infof("syncService: service [%s/%s] owner disabled macvlan, will deleted",
				svc.Namespace, svc.Name)
			// err := c.kubeClientset.CoreV1().Services(svc.Namespace).
			// 	Delete(context.TODO(), svc.Name, metav1.DeleteOptions{})
			err := h.serviceClient.Delete(svc.Namespace, svc.Name, &metav1.DeleteOptions{})
			if err != nil {
				logrus.Errorf("syncService: failed to delete service [%v/%v]: %v",
					svc.Namespace, svc.Name, err)
				return svc, err
			}
			return svc, nil
		}
	}

	// The macvlan service creation was disabled, return directly.
	// Only sync non-macvlan & non-ingress service.
	if isMacvlanServiceDisabled || !isMacvlanPodEnabled || isMacvlanService {
		return svc, nil
	}

	logrus.Debugf("syncService: processing service [%s/%s]", svc.Namespace, svc.Name)
	// Create if the macvlan service not exists.
	expectedMacvlanSvc := makeMacvlanService(svc)
	existMacvlanSvc, err := h.serviceCache.Get(svc.Name, expectedMacvlanSvc.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			if strings.HasSuffix(svc.Name, macvlanServiceNameSuffix) {
				logrus.Infof("syncService: Skip create [%v/%v] as the origional svc have %q suffix",
					svc.Namespace, expectedMacvlanSvc.Name, macvlanServiceNameSuffix)
				return svc, nil
			}
			// Create the macvlan service.
			logrus.Infof("syncService: Request to create macvlan service [%v/%v]",
				svc.Namespace, expectedMacvlanSvc.Name)
			// _, err := c.kubeClientset.CoreV1().Services(svc.Namespace).
			// 	Create(context.TODO(), expectedMacvlanSvc, metav1.CreateOptions{})
			_, err := h.serviceClient.Create(expectedMacvlanSvc)
			if err != nil {
				logrus.Errorf("syncService: failed to create macvlan service [%v/%v]: %v",
					svc.Namespace, expectedMacvlanSvc.Name, err)
				return svc, err
			}
			return svc, nil
		}
		logrus.Errorf("syncService: failed to get [%v/%v]: %v", svc.Namespace, expectedMacvlanSvc.Name, err)
		return svc, err
	}

	// Skip if the macvlan service is already updated.
	if macvlanServiceUpdated(existMacvlanSvc, expectedMacvlanSvc) {
		logrus.Debugf("syncService: macvlan service [%v/%v] already updated, skip",
			svc.Namespace, expectedMacvlanSvc.Name)
		return svc, nil
	}

	// Update the macvlan service with retry to avoid conflict.
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		logrus.Debugf("Kube apiserver update service [%v/%v] request", svc.Namespace, svc.Name)
		macvlanSvc, err := h.serviceClient.Get(svc.Namespace, expectedMacvlanSvc.Name, metav1.GetOptions{})
		if err != nil {
			logrus.Warnf("syncService: failed to get svc [%v/%v]: %v",
				svc.Namespace, expectedMacvlanSvc.Name, err)
			return err
		}

		macvlanSvc = macvlanSvc.DeepCopy()
		macvlanSvc.Spec = expectedMacvlanSvc.Spec
		macvlanSvc.Annotations = expectedMacvlanSvc.Annotations
		macvlanSvc.OwnerReferences = expectedMacvlanSvc.OwnerReferences
		_, err = h.serviceClient.Update(macvlanSvc)
		if err != nil {
			logrus.Warnf("syncService: service [%v/%v] update error: %v",
				svc.Namespace, macvlanSvc.Name, err)
			return err
		}
		return nil
	}); err != nil {
		logrus.Errorf("syncService: service [%v/%v] update retry error: %v",
			svc.Namespace, expectedMacvlanSvc.Name, err)
		return svc, err
	}

	return svc, nil
}

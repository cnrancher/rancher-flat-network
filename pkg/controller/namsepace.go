package controller

import (
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

func (h *Handler) handleNamespaceError(
	onChange func(string, *corev1.Namespace) (*corev1.Namespace, error),
) func(string, *corev1.Namespace) (*corev1.Namespace, error) {
	return func(key string, namespace *corev1.Namespace) (*corev1.Namespace, error) {
		var err error
		namespace, err = onChange(key, namespace)
		if namespace == nil {
			return namespace, err
		}

		if err != nil {
			// Avoid trigger the rate limit.
			logrus.Warnf("%v", err)
			time.Sleep(time.Second * 1)

			// TODO: handle error event here.
		}
		return namespace, err
	}
}

func (h *Handler) onNamespaceChanged(s string, namespace *corev1.Namespace) (*corev1.Namespace, error) {
	if namespace == nil || namespace.Name == "" || namespace.DeletionTimestamp != nil {
		return namespace, nil
	}

	return namespace, nil
}

func (h *Handler) onNamespaceRemoved(s string, namespace *corev1.Namespace) (*corev1.Namespace, error) {
	if namespace == nil || namespace.Name == "" {
		return namespace, nil
	}

	return namespace, nil
}

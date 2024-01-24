package controller

import (
	"time"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
)

func (h *Handler) handleDeploymentError(
	onChange func(string, *appsv1.Deployment) (*appsv1.Deployment, error),
) func(string, *appsv1.Deployment) (*appsv1.Deployment, error) {
	return func(key string, deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
		var err error
		deployment, err = onChange(key, deployment)
		if deployment == nil {
			return deployment, err
		}

		if err != nil {
			// Avoid trigger the rate limit.
			logrus.Warnf("%v", err)
			time.Sleep(time.Second * 1)

			// TODO: handle error event here.
		}
		return deployment, err
	}
}

func (h *Handler) onDeploymentChanged(s string, deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	if deployment == nil || deployment.Name == "" || deployment.DeletionTimestamp != nil {
		return deployment, nil
	}

	obj, err := h.syncWorkload(deployment)
	return obj.(*appsv1.Deployment), err
}

func (h *Handler) onDeploymentRemoved(s string, deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	if deployment == nil || deployment.Name == "" {
		return deployment, nil
	}

	return deployment, nil
}

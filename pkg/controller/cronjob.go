package controller

import (
	"time"

	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
)

func (h *Handler) handleCronJobError(
	onChange func(string, *batchv1.CronJob) (*batchv1.CronJob, error),
) func(string, *batchv1.CronJob) (*batchv1.CronJob, error) {
	return func(key string, cronjob *batchv1.CronJob) (*batchv1.CronJob, error) {
		var err error
		cronjob, err = onChange(key, cronjob)
		if cronjob == nil {
			return cronjob, err
		}

		if err != nil {
			// Avoid trigger the rate limit.
			logrus.Warnf("%v", err)
			time.Sleep(time.Second * 1)

			// TODO: handle error event here.
		}
		return cronjob, err
	}
}

func (h *Handler) onCronJobChanged(s string, cronjob *batchv1.CronJob) (*batchv1.CronJob, error) {
	if cronjob == nil || cronjob.Name == "" || cronjob.DeletionTimestamp != nil {
		return cronjob, nil
	}

	h.syncWorkload(cronjob)

	return cronjob, nil
}

func (h *Handler) onCronJobRemoved(s string, cronjob *batchv1.CronJob) (*batchv1.CronJob, error) {
	if cronjob == nil || cronjob.Name == "" || cronjob.DeletionTimestamp != nil {
		return cronjob, nil
	}

	return cronjob, nil
}

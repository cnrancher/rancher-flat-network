package controller

import (
	"time"

	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
)

func (h *Handler) handleJobsError(
	onChange func(string, *batchv1.Job) (*batchv1.Job, error),
) func(string, *batchv1.Job) (*batchv1.Job, error) {
	return func(key string, job *batchv1.Job) (*batchv1.Job, error) {
		var err error
		job, err = onChange(key, job)
		if job == nil {
			return job, err
		}

		if err != nil {
			// Avoid trigger the rate limit.
			logrus.Warnf("%v", err)
			time.Sleep(time.Second * 1)

			// TODO: handle error event here.
		}
		return job, err
	}
}

func (h *Handler) onJobsChanged(s string, job *batchv1.Job) (*batchv1.Job, error) {
	if job == nil || job.Name == "" || job.DeletionTimestamp != nil {
		return job, nil
	}

	obj, err := h.syncWorkload(job)
	return obj.(*batchv1.Job), err
}

func (h *Handler) onJobsRemoved(s string, job *batchv1.Job) (*batchv1.Job, error) {
	if job == nil || job.Name == "" {
		return job, nil
	}

	return job, nil
}

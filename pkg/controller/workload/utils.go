package workload

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func deepCopy(w metav1.Object) metav1.Object {
	switch w := w.(type) {
	case *appsv1.Deployment:
		return w.DeepCopy()
	case *appsv1.DaemonSet:
		return w.DeepCopy()
	case *appsv1.StatefulSet:
		return w.DeepCopy()
	case *appsv1.ReplicaSet:
		return w.DeepCopy()
	case *batchv1.CronJob:
		return w.DeepCopy()
	case *batchv1.Job:
		return w.DeepCopy()
	}
	return nil
}

func getTemplateObjectMeta(w any) *metav1.ObjectMeta {
	switch w := w.(type) {
	case *appsv1.Deployment:
		return &w.Spec.Template.ObjectMeta
	case *appsv1.DaemonSet:
		return &w.Spec.Template.ObjectMeta
	case *appsv1.StatefulSet:
		return &w.Spec.Template.ObjectMeta
	case *appsv1.ReplicaSet:
		return &w.Spec.Template.ObjectMeta
	case *batchv1.CronJob:
		return &w.Spec.JobTemplate.ObjectMeta
	case *batchv1.Job:
		return &w.Spec.Template.ObjectMeta
	}
	return nil
}

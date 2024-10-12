package migrate

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	"github.com/cnrancher/rancher-flat-network/pkg/controller/workload"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/sirupsen/logrus"
)

func (m *migrator) migrateWorkload(ctx context.Context, kind string) error {
	var err error
	var listOption = metav1.ListOptions{
		Limit: 100,
	}
	// Workload kind could be deployment, daemonset, statefulset, cronjob, job.
	// replicaset is managed by deployment and is not a workload.
	switch strings.TrimSuffix(strings.ToLower(kind), "s") {
	case "deployment":
		var o *appsv1.DeploymentList
		for o == nil || o.Continue != "" {
			o, err = m.wctx.Apps.Deployment().List("", listOption)
			if err != nil {
				return err
			}
			for _, i := range o.Items {
				if err := m.processPodTemplateAnnotation(ctx, &i); err != nil {
					return err
				}
			}
			listOption.Continue = o.Continue
		}
	case "daemonset":
		var o *appsv1.DaemonSetList
		for o == nil || o.Continue != "" {
			o, err = m.wctx.Apps.DaemonSet().List("", listOption)
			if err != nil {
				return err
			}
			for _, i := range o.Items {
				if err := m.processPodTemplateAnnotation(ctx, &i); err != nil {
					return err
				}
			}
			listOption.Continue = o.Continue
		}
	case "statefulset":
		var o *appsv1.StatefulSetList
		for o == nil || o.Continue != "" {
			o, err = m.wctx.Apps.StatefulSet().List("", listOption)
			if err != nil {
				return err
			}
			for _, i := range o.Items {
				if err := m.processPodTemplateAnnotation(ctx, &i); err != nil {
					return err
				}
			}
			listOption.Continue = o.Continue
		}
	case "cronjob":
		var o *batchv1.CronJobList
		for o == nil || o.Continue != "" {
			o, err = m.wctx.Batch.CronJob().List("", listOption)
			if err != nil {
				return err
			}
			for _, i := range o.Items {
				if err := m.processPodTemplateAnnotation(ctx, &i); err != nil {
					return err
				}
			}
			listOption.Continue = o.Continue
		}
	case "job":
		var o *batchv1.JobList
		for o == nil || o.Continue != "" {
			o, err = m.wctx.Batch.Job().List("", listOption)
			if err != nil {
				return err
			}
			for _, i := range o.Items {
				if err := m.processPodTemplateAnnotation(ctx, &i); err != nil {
					return err
				}
			}
			listOption.Continue = o.Continue
		}
	case "":
	default:
		logrus.Warnf("unrecognized workload kind %q", kind)
	}
	return nil
}

func isMacvlanV1Enabled(a map[string]string) bool {
	if len(a) == 0 {
		return false
	}
	if a[macvlanV1AnnotationIP] == "" || a[macvlanV1AnnotationSubnet] == "" {
		return false
	}
	return true
}

func (m *migrator) getWorkload(o metav1.Object) (metav1.Object, error) {
	switch o := o.(type) {
	case *appsv1.Deployment:
		return m.wctx.Apps.Deployment().Get(o.GetNamespace(), o.GetName(), metav1.GetOptions{})
	case *appsv1.DaemonSet:
		return m.wctx.Apps.DaemonSet().Get(o.GetNamespace(), o.GetName(), metav1.GetOptions{})
	case *appsv1.StatefulSet:
		return m.wctx.Apps.StatefulSet().Get(o.GetNamespace(), o.GetName(), metav1.GetOptions{})
	case *batchv1.CronJob:
		return m.wctx.Batch.CronJob().Get(o.GetNamespace(), o.GetName(), metav1.GetOptions{})
	case *batchv1.Job:
		return m.wctx.Batch.Job().Get(o.GetNamespace(), o.GetName(), metav1.GetOptions{})
	}
	return nil, fmt.Errorf("unrecognized workload type %T", o)
}

func (m *migrator) processPodTemplateAnnotation(
	ctx context.Context, o metav1.Object,
) error {
	var err error
	metadata := workload.GetTemplateObjectMeta(o)
	annotation := metadata.Annotations
	if !isMacvlanV1Enabled(annotation) {
		logrus.Debugf("skip update %T [%v/%v] podTemplate as not using FlatNetwork",
			o, o.GetNamespace(), o.GetName())
		return nil
	}
	au := updateAnnotation(o)
	if reflect.DeepEqual(annotation, au) {
		logrus.Debugf("skip update %T [%v/%v] podTemplate annotation as already updated",
			o, o.GetNamespace(), o.GetName())
		return nil
	}
	logrus.Infof("%T [%v/%v] Annotation before:",
		o, o.GetNamespace(), o.GetName())
	fmt.Println(utils.Print(annotation))
	logrus.Infof("============== After ==============")
	fmt.Println(utils.Print(au))
	if err = utils.PromptUser(ctx, "Continue?", m.autoYes); err != nil {
		return fmt.Errorf("failed to update %T [%v/%v] annotation: %w",
			o, o.GetNamespace(), o.GetName(), err)
	}
	if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		o, err = m.getWorkload(o)
		if err != nil {
			return fmt.Errorf("failed to get workload %T [%v/%v]: %w",
				o, o.GetNamespace(), o.GetName(), err)
		}
		o = workload.DeepCopy(o)
		setTemplateObjectMetaAnnotations(o, au)
		l := removeV1Labels(o)
		logrus.Debugf("update workload labels to %v", utils.Print(l))
		o.SetLabels(l)
		logrus.Infof("applying workload %T [%v/%v] annotations...",
			o, o.GetNamespace(), o.GetName())
		_, err := m.updateWorkload(o)
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to update workload %T [%v/%v]: %w",
			o, o.GetNamespace(), o.GetName(), err)
	}
	time.Sleep(m.interval)
	return nil
}

func setTemplateObjectMetaAnnotations(w metav1.Object, a map[string]string) {
	if w == nil {
		return
	}

	switch w := w.(type) {
	case *appsv1.Deployment:
		w.Spec.Template.ObjectMeta.Annotations = a
	case *appsv1.DaemonSet:
		w.Spec.Template.ObjectMeta.Annotations = a
	case *appsv1.StatefulSet:
		w.Spec.Template.ObjectMeta.Annotations = a
	case *batchv1.CronJob:
		w.Spec.JobTemplate.ObjectMeta.Annotations = a
	case *batchv1.Job:
		w.Spec.Template.ObjectMeta.Annotations = a
	}
}

func (m *migrator) updateWorkload(o metav1.Object) (metav1.Object, error) {
	switch o := o.(type) {
	case *appsv1.Deployment:
		return m.wctx.Apps.Deployment().Update(o)
	case *appsv1.DaemonSet:
		return m.wctx.Apps.DaemonSet().Update(o)
	case *appsv1.StatefulSet:
		return m.wctx.Apps.StatefulSet().Update(o)
	case *batchv1.CronJob:
		return m.wctx.Batch.CronJob().Update(o)
	case *batchv1.Job:
		return m.wctx.Batch.Job().Update(o)
	}
	return nil, fmt.Errorf("unrecognized workload type %T", o)
}

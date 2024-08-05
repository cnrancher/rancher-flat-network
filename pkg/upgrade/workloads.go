package upgrade

import (
	"fmt"
	"maps"
	"reflect"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	flv1 "github.com/cnrancher/rancher-flat-network/pkg/apis/flatnetwork.pandaria.io/v1"
	"github.com/cnrancher/rancher-flat-network/pkg/controller/workload"
	"github.com/sirupsen/logrus"
)

const (
	macvlanV1Prefix            = "macvlan.pandaria.cattle.io/"
	macvlanV1AnnotationIP      = "macvlan.pandaria.cattle.io/ip"
	macvlanV1AnnotationSubnet  = "macvlan.pandaria.cattle.io/subnet"
	macvlanV1NetAttatchDefName = `[{"name":"static-macvlan-cni-attach","interface":"eth1"}]`

	k8sCNINetworksKey     = "k8s.v1.cni.cncf.io/networks"
	rancherFlatNetworkCNI = `[{"name":"rancher-flat-network","interface":"eth1"}]`
)

func (m *migrator) migrateWorkload(kind string) error {
	switch kind {
	case "deployment":
		o, err := m.wctx.Apps.Deployment().List("", metav1.ListOptions{})
		if err != nil {
			return err
		}
		for _, i := range o.Items {
			if err := m.processPodTemplateAnnotation(&i); err != nil {
				return err
			}
		}
	case "daemonset":
		o, err := m.wctx.Apps.DaemonSet().List("", metav1.ListOptions{})
		if err != nil {
			return err
		}
		for _, i := range o.Items {
			if err := m.processPodTemplateAnnotation(&i); err != nil {
				return err
			}
		}
	case "statefulset":
		o, err := m.wctx.Apps.StatefulSet().List("", metav1.ListOptions{})
		if err != nil {
			return err
		}
		for _, i := range o.Items {
			if err := m.processPodTemplateAnnotation(&i); err != nil {
				return err
			}
		}
	case "replicaset":
		o, err := m.wctx.Apps.ReplicaSet().List("", metav1.ListOptions{})
		if err != nil {
			return err
		}
		for _, i := range o.Items {
			if err := m.processPodTemplateAnnotation(&i); err != nil {
				return err
			}
		}
	case "cronjob":
		o, err := m.wctx.Batch.CronJob().List("", metav1.ListOptions{})
		if err != nil {
			return err
		}
		for _, i := range o.Items {
			if err := m.processPodTemplateAnnotation(&i); err != nil {
				return err
			}
		}
	case "job":
		o, err := m.wctx.Batch.Job().List("", metav1.ListOptions{})
		if err != nil {
			return err
		}
		for _, i := range o.Items {
			if err := m.processPodTemplateAnnotation(&i); err != nil {
				return err
			}
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
	case *appsv1.ReplicaSet:
		return m.wctx.Apps.ReplicaSet().Get(o.GetNamespace(), o.GetName(), metav1.GetOptions{})
	case *batchv1.CronJob:
		return m.wctx.Batch.CronJob().Get(o.GetNamespace(), o.GetName(), metav1.GetOptions{})
	case *batchv1.Job:
		return m.wctx.Batch.Job().Get(o.GetNamespace(), o.GetName(), metav1.GetOptions{})
	}
	return nil, fmt.Errorf("unrecognized workload type %T", o)
}

func (m *migrator) processPodTemplateAnnotation(o metav1.Object) error {
	metadata := workload.GetTemplateObjectMeta(o)
	annotation := metadata.Annotations
	if !isMacvlanV1Enabled(annotation) {
		logrus.Debugf("skip update %T [%v/%v] podTemplate as not using FlatNetwork",
			o, o.GetNamespace(), o.GetName())
		return nil
	}
	au := maps.Clone(annotation)
	for k, v := range annotation {
		if strings.Contains(k, macvlanV1Prefix) {
			delete(au, k)
			k := strings.ReplaceAll(k, macvlanV1Prefix, flv1.AnnotationPrefix)
			au[k] = v
			logrus.Infof("update %T [%v/%v] podTemplate annotation: [%v: %v]",
				o, o.GetNamespace(), o.GetName(), k, v)
		}
		if k == k8sCNINetworksKey && v == macvlanV1NetAttatchDefName { // TODO:
			au[k] = rancherFlatNetworkCNI
			logrus.Infof("update %T [%v/%v] podTemplate annotation: [%v: %v]",
				o, o.GetNamespace(), o.GetName(), k, rancherFlatNetworkCNI)
		}
	}
	if reflect.DeepEqual(annotation, au) {
		logrus.Debugf("skip update %T [%v/%v] podTemplate annotation as already updated",
			o, o.GetNamespace(), o.GetName())
		return nil
	}
	var err error
	if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		o, err = m.getWorkload(o)
		if err != nil {
			return fmt.Errorf("failed to get workload %T [%v/%v]: %w",
				o, o.GetNamespace(), o.GetName(), err)
		}
		o = workload.DeepCopy(o)
		setTemplateObjectMetaAnnotations(o, au)
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
	time.Sleep(time.Millisecond * 500)
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
	case *appsv1.ReplicaSet:
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
	case *appsv1.ReplicaSet:
		return m.wctx.Apps.ReplicaSet().Update(o)
	case *batchv1.CronJob:
		return m.wctx.Batch.CronJob().Update(o)
	case *batchv1.Job:
		return m.wctx.Batch.Job().Update(o)
	}
	return nil, fmt.Errorf("unrecognized workload type %T", o)
}

package controller

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	macvlanv1 "github.com/cnrancher/macvlan-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
)

func (c *Handler) checkFixedIPsFromWorkloadInformer(ip, subnet string) error {
	key := fmt.Sprintf("%s:%s", ip, subnet)
	if obj, ok := c.inUsedFixedIPs.Load(key); ok {
		var usedInfo string
		switch w := obj.(type) {
		case *appsv1.Deployment:
			newobj, _ := c.deployments.Get(w.Namespace, w.Name, metav1.GetOptions{})
			if newobj != nil && newobj.DeletionTimestamp == nil {
				annotationIP := newobj.Spec.Template.Annotations[macvlanv1.AnnotationIP]
				if annotationIP != "" && annotationIP != "auto" {
					usedInfo = newobj.Namespace + ":" + newobj.Name
				}
			}
		case *appsv1.StatefulSet:
			newobj, _ := c.statefulsets.Get(w.Namespace, w.Name, metav1.GetOptions{})
			if newobj != nil && newobj.DeletionTimestamp == nil {
				annotationIP := newobj.Spec.Template.Annotations[macvlanv1.AnnotationIP]
				if annotationIP != "" && annotationIP != "auto" {
					usedInfo = newobj.Namespace + ":" + newobj.Name
				}
			}
		case *batchv1.CronJob:
			newobj, _ := c.cronjobs.Get(w.Namespace, w.Name, metav1.GetOptions{})
			if newobj != nil && newobj.DeletionTimestamp == nil {
				annotationIP := newobj.Spec.JobTemplate.Spec.Template.Annotations[macvlanv1.AnnotationIP]
				if annotationIP != "" && annotationIP != "auto" {
					usedInfo = newobj.Namespace + ":" + newobj.Name
				}
			}
		case *batchv1.Job:
			newobj, _ := c.jobs.Get(w.Namespace, w.Name, metav1.GetOptions{})
			if newobj != nil && newobj.DeletionTimestamp == nil {
				annotationIP := newobj.Spec.Template.Annotations[macvlanv1.AnnotationIP]
				if annotationIP != "" && annotationIP != "auto" {
					usedInfo = newobj.Namespace + ":" + newobj.Name
				}
			}
		}
		if usedInfo != "" {
			return fmt.Errorf("checkFixedIPsFromWorkloadInformer: %s is used by %v", ip, usedInfo)
		}
		logrus.Infof("checkFixedIPsFromWorkloadInformer: delete key %s from inUsedFixedIPs as the workload has no fixed IPs", key)
		c.inUsedFixedIPs.Delete(key)
	}

	return nil
}

func (c *Handler) syncFixedIPsCache() {
	// list fixed IPs from deployment/statefulset/cronjob/job
	dps, err := c.deployments.List("", metav1.ListOptions{
		LabelSelector: labels.Everything().String(),
	})
	if err != nil {
		logrus.Warnf("syncFixedIPsCache: failed to list deployments: %v", err)
	}
	if dps == nil || dps.Items == nil {
		return
	}
	for _, dp := range dps.Items {
		annotationIP := dp.Spec.Template.Annotations[macvlanv1.AnnotationIP]
		annotationSubnet := dp.Spec.Template.Annotations[macvlanv1.AnnotationSubnet]
		if annotationIP != "" && annotationIP != "auto" {
			for _, ip := range strings.Split(annotationIP, "-") {
				c.inUsedFixedIPs.Store(fmt.Sprintf("%s:%s", ip, annotationSubnet), dp)
			}
		}
	}
	sfs, err := c.statefulsets.List("", metav1.ListOptions{})
	if err != nil {
		logrus.Warnf("syncFixedIPsCache: failed to list statefulsets: %v", err)
	}
	if sfs == nil || sfs.Items == nil {
		return
	}
	for _, sf := range sfs.Items {
		annotationIP := sf.Spec.Template.Annotations[macvlanv1.AnnotationIP]
		annotationSubnet := sf.Spec.Template.Annotations[macvlanv1.AnnotationSubnet]
		if annotationIP != "" && annotationIP != "auto" {
			for _, ip := range strings.Split(annotationIP, "-") {
				c.inUsedFixedIPs.Store(fmt.Sprintf("%s:%s", ip, annotationSubnet), sf)
			}
		}
	}
	cronjobs, err := c.cronjobs.List("", metav1.ListOptions{})
	if err != nil {
		logrus.Warnf("syncFixedIPsCache: failed to list cronjobs: %v", err)
	}
	if cronjobs == nil || cronjobs.Items == nil {
		return
	}
	for _, cronjob := range cronjobs.Items {
		annotationIP := cronjob.Spec.JobTemplate.Spec.Template.Annotations[macvlanv1.AnnotationIP]
		annotationSubnet := cronjob.Spec.JobTemplate.Spec.Template.Annotations[macvlanv1.AnnotationSubnet]
		if annotationIP != "" && annotationIP != "auto" {
			for _, ip := range strings.Split(annotationIP, "-") {
				c.inUsedFixedIPs.Store(fmt.Sprintf("%s:%s", ip, annotationSubnet), cronjob)
			}
		}
	}
	jobs, err := c.jobs.List("", metav1.ListOptions{})
	if err != nil {
		logrus.Warnf("syncFixedIPsCache: failed to list jobs: %v", err)
	}
	if jobs == nil || jobs.Items == nil {
		return
	}
	for _, job := range jobs.Items {
		annotationIP := job.Spec.Template.Annotations[macvlanv1.AnnotationIP]
		annotationSubnet := job.Spec.Template.Annotations[macvlanv1.AnnotationSubnet]
		if annotationIP != "" && annotationIP != "auto" {
			for _, ip := range strings.Split(annotationIP, "-") {
				c.inUsedFixedIPs.Store(fmt.Sprintf("%s:%s", ip, annotationSubnet), job)
			}
		}
	}
}

func (c *Handler) syncWorkload(obj interface{}) {
	c.syncFixedIPsCache()
	var err error
	switch workload := obj.(type) {
	case *appsv1.Deployment:
		if update, iptype, subnet := needUpdateWorkloadMacvlanLabel(workload.Spec.Template.ObjectMeta, workload.ObjectMeta); update {
			w := workload.DeepCopy()
			if w.Labels == nil {
				w.Labels = map[string]string{}
			}
			w.Labels[macvlanv1.LabelMacvlanIPType] = iptype
			w.Labels[macvlanv1.LabelSubnet] = subnet
			_, err = c.deployments.Update(w)
		}
	case *appsv1.DaemonSet:
		if update, iptype, subnet := needUpdateWorkloadMacvlanLabel(workload.Spec.Template.ObjectMeta, workload.ObjectMeta); update {
			w := workload.DeepCopy()
			if w.Labels == nil {
				w.Labels = map[string]string{}
			}
			w.Labels[macvlanv1.LabelMacvlanIPType] = iptype
			w.Labels[macvlanv1.LabelSubnet] = subnet
			_, err = c.daemonsets.Update(w)
		}
	case *appsv1.StatefulSet:
		if update, iptype, subnet := needUpdateWorkloadMacvlanLabel(workload.Spec.Template.ObjectMeta, workload.ObjectMeta); update {
			w := workload.DeepCopy()
			if w.Labels == nil {
				w.Labels = map[string]string{}
			}
			w.Labels[macvlanv1.LabelMacvlanIPType] = iptype
			w.Labels[macvlanv1.LabelSubnet] = subnet
			_, err = c.statefulsets.Update(w)
		}
	case *batchv1.CronJob:
		if update, iptype, subnet := needUpdateWorkloadMacvlanLabel(workload.Spec.JobTemplate.Spec.Template.ObjectMeta, workload.ObjectMeta); update {
			w := workload.DeepCopy()
			if w.Labels == nil {
				w.Labels = map[string]string{}
			}
			w.Labels[macvlanv1.LabelMacvlanIPType] = iptype
			w.Labels[macvlanv1.LabelSubnet] = subnet
			_, err = c.cronjobs.Update(w)
		}
	case *batchv1.Job:
		if update, iptype, subnet := needUpdateWorkloadMacvlanLabel(workload.Spec.Template.ObjectMeta, workload.ObjectMeta); update {
			w := workload.DeepCopy()
			if w.Labels == nil {
				w.Labels = map[string]string{}
			}
			w.Labels[macvlanv1.LabelMacvlanIPType] = iptype
			w.Labels[macvlanv1.LabelSubnet] = subnet
			_, err = c.jobs.Update(w)
		}
	}
	if err != nil {
		logrus.Warnf("syncWorkload: failed to update workload label, %v", err)
	}
}

func needUpdateWorkloadMacvlanLabel(podMeta, workloadMeta metav1.ObjectMeta) (bool, string, string) {
	update := false
	iptype := workloadMeta.Labels[macvlanv1.LabelMacvlanIPType]
	subnet := podMeta.Annotations[macvlanv1.AnnotationSubnet]

	if podMeta.Annotations[macvlanv1.AnnotationSubnet] != workloadMeta.Labels[macvlanv1.LabelSubnet] {
		update = true
		subnet = podMeta.Annotations[macvlanv1.AnnotationSubnet]
	}

	if podMeta.Annotations[macvlanv1.AnnotationIP] == "auto" {
		if workloadMeta.Labels[macvlanv1.LabelMacvlanIPType] != "auto" {
			update = true
			iptype = "auto"
		}
	}

	if podMeta.Annotations[macvlanv1.AnnotationIP] != "auto" &&
		podMeta.Annotations[macvlanv1.AnnotationIP] != "" &&
		workloadMeta.Labels[macvlanv1.LabelMacvlanIPType] != "specific" {
		update = true
		iptype = "specific"
	}
	if update {
		logrus.Debugf("needUpdateWorkloadMacvlanLabel: %s %s", workloadMeta.Name, workloadMeta.Namespace)
	}
	return update, iptype, subnet
}

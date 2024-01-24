package controller

import (
	"github.com/sirupsen/logrus"

	macvlanv1 "github.com/cnrancher/macvlan-operator/pkg/apis/macvlan.cluster.cattle.io/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
)

func tmp_ips(s string, ip *macvlanv1.MacvlanIP) (*macvlanv1.MacvlanIP, error) {
	logrus.Infof("sync ip: %v, s: %v", ip.Name, s)

	return ip, nil
}

func tmp_subnets(s string, subnet *macvlanv1.MacvlanSubnet) (*macvlanv1.MacvlanSubnet, error) {
	logrus.Infof("sync subnet: %v", subnet.Name)
	return subnet, nil
}

func tmp_deployments(s string, dep *appsv1.Deployment) (*appsv1.Deployment, error) {
	logrus.Infof("sync Deployment: %v", dep.Name)
	return dep, nil
}

func tmp_statefulsets(s string, ss *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	logrus.Infof("sync statefulset: %v", ss.Name)
	return ss, nil
}

func tmp_cronJobs(s string, cj *batchv1.CronJob) (*batchv1.CronJob, error) {
	logrus.Infof("sync CronJob: %v", cj.Name)
	return cj, nil
}

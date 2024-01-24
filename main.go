//go:generate go run pkg/codegen/cleanup/main.go
//go:generate go run pkg/codegen/main.go

package main

import (
	"flag"
	"time"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/cnrancher/macvlan-operator/pkg/controller"
	"github.com/cnrancher/macvlan-operator/pkg/generated/controllers/apps"
	"github.com/cnrancher/macvlan-operator/pkg/generated/controllers/batch"
	"github.com/cnrancher/macvlan-operator/pkg/generated/controllers/core"
	macvlanv1 "github.com/cnrancher/macvlan-operator/pkg/generated/controllers/macvlan.cluster.cattle.io"
	"github.com/cnrancher/macvlan-operator/pkg/utils"
	"github.com/rancher/wrangler/pkg/kubeconfig"
	"github.com/rancher/wrangler/pkg/signals"
	"github.com/rancher/wrangler/pkg/start"
	"github.com/sirupsen/logrus"
)

var (
	masterURL         string
	kubeconfigFile    string
	port              int
	address           string
	cert              string
	key               string
	isDeployAdmission bool
	qps               string
	burst             string
	queueRate         string
	queueSize         string
	worker            int
	version           bool
	debug             bool

	// purgeMacvlanIPInterval int
	// purgePodInterval       int
)

func init() {
	logrus.SetFormatter(&nested.Formatter{
		HideKeys: true,
		// TimestampFormat: "2006-01-02 15:04:05",
		TimestampFormat: time.StampMilli,
		FieldsOrder:     []string{"cluster", "phase"},
	})
}

func main() {
	flag.StringVar(&kubeconfigFile, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")

	// admission webhook
	flag.IntVar(&port, "port", 443, "The port on which to serve.")
	flag.StringVar(&address, "bind-address", "0.0.0.0", "The IP address on which to listen for the --port port.")
	flag.StringVar(&cert, "tls-cert-file", "/etc/webhook/certs/tls.crt", "File containing the default x509 Certificate for HTTPS.")
	flag.StringVar(&key, "tls-private-key-file", "/etc/webhook/certs/tls.key", "File containing the default x509 private key matching --tls-cert-file.")

	flag.BoolVar(&debug, "debug", false, "Enable log debug level.")
	flag.StringVar(&qps, "qps", "250", "QPS indicates the maximum QPS to the master from this client.")
	flag.StringVar(&burst, "burst", "500", "Maximum burst for throttle.")
	flag.StringVar(&queueRate, "queue-rate", "50", "Rate for work queue.")
	flag.StringVar(&queueSize, "queue-size", "500", "Size for work queue.")
	flag.IntVar(&worker, "worker", 5, "Worker number for controller.")
	// flag.IntVar(&purgeMacvlanIPInterval, "purge-badmacvlanip-interval", 3600, "Interval seconds of purging invalid macvlanips.")
	// flag.IntVar(&purgePodInterval, "purge-badpod-interval", 60, "Interval seconds of purging bad pods.")
	flag.Parse()

	if debug {
		logrus.SetLevel(logrus.DebugLevel)
		logrus.Debugf("debug output enabled")
	}
	if version {
		if utils.GitCommit != "" {
			logrus.Infof("macvlan-operator %v - %v", utils.Version, utils.GitCommit)
		} else {
			logrus.Infof("macvlan-operator %v", utils.Version)
		}
		return
	}

	// controller.PurgeMacvlanIPInterval = purgeMacvlanIPInterval
	// controller.PurgePodInterval = purgePodInterval

	// Set up signals so we handle the first shutdown signal gracefully
	ctx := signals.SetupSignalContext()

	// This will load the kubeconfig file in a style the same as kubectl
	cfg, err := kubeconfig.GetNonInteractiveClientConfig(kubeconfigFile).ClientConfig()
	if err != nil {
		logrus.Fatalf("Error building kubeconfig: %v", err)
	}

	// Generated controllers.
	macvlan := macvlanv1.NewFactoryFromConfigOrDie(cfg)
	core := core.NewFactoryFromConfigOrDie(cfg)
	apps := apps.NewFactoryFromConfigOrDie(cfg)
	batch := batch.NewFactoryFromConfigOrDie(cfg)
	opts := &controller.RegisterOpts{
		MacvlanIPs:     macvlan.Macvlan().V1().MacvlanIP(),
		MacvlanSubnets: macvlan.Macvlan().V1().MacvlanSubnet(),
		Pods:           core.Core().V1().Pod(),
		Services:       core.Core().V1().Service(),
		Namespaces:     core.Core().V1().Namespace(),
		Deployments:    apps.Apps().V1().Deployment(),
		Daemonsets:     apps.Apps().V1().DaemonSet(),
		Replicasets:    apps.Apps().V1().ReplicaSet(),
		Statefulsets:   apps.Apps().V1().StatefulSet(),
		Cronjobs:       batch.Batch().V1().CronJob(),
		Jobs:           batch.Batch().V1().Job(),
	}

	// The typical pattern is to build all your controller/clients then just pass to each handler
	// the bare minimum of what they need.  This will eventually help with writing tests.  So
	// don't pass in something like kubeClient, apps, or sample
	controller.Register(ctx, opts)

	// Start all controllers.
	if err := start.All(ctx, 4, macvlan, core, apps, batch); err != nil {
		logrus.Fatalf("Error starting macvlan operator: %v", err)
	}

	<-ctx.Done()
	logrus.Infof("macvlan operator stopped gracefully")
}

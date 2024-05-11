//go:generate go run pkg/codegen/cleanup/main.go
//go:generate go run pkg/codegen/main.go

package main

import (
	"flag"
	"time"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/cnrancher/flat-network-operator/pkg/controller/ingress"
	"github.com/cnrancher/flat-network-operator/pkg/controller/macvlanip"
	"github.com/cnrancher/flat-network-operator/pkg/controller/macvlansubnet"
	"github.com/cnrancher/flat-network-operator/pkg/controller/service"
	"github.com/cnrancher/flat-network-operator/pkg/controller/workload"
	"github.com/cnrancher/flat-network-operator/pkg/controller/wrangler"
	"github.com/cnrancher/flat-network-operator/pkg/utils"
	"github.com/rancher/wrangler/v2/pkg/kubeconfig"
	"github.com/rancher/wrangler/v2/pkg/signals"
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
	worker            int
	version           bool
	debug             bool
)

func init() {
	logrus.SetFormatter(&nested.Formatter{
		HideKeys:        true,
		TimestampFormat: time.DateTime,
		FieldsOrder:     []string{"namespace", "name"},
	})
}

func main() {
	flag.StringVar(&kubeconfigFile, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.IntVar(&port, "port", 443, "The port on which to serve.")
	flag.StringVar(&address, "bind-address", "0.0.0.0", "The IP address on which to listen for the --port port.")
	flag.StringVar(&cert, "tls-cert-file", "/etc/webhook/certs/tls.crt", "File containing the default x509 Certificate for HTTPS.")
	flag.StringVar(&key, "tls-private-key-file", "/etc/webhook/certs/tls.key", "File containing the default x509 private key matching --tls-cert-file.")
	flag.BoolVar(&debug, "debug", false, "Enable log debug level.")
	flag.BoolVar(&version, "v", false, "Show version.")
	flag.IntVar(&worker, "worker", 5, "Worker number (1-50).")
	flag.Parse()

	if worker > 50 || worker < 1 {
		logrus.Warnf("invalid worker num: %v, set to default: 5", worker)
		worker = 5
	}
	if debug {
		logrus.SetLevel(logrus.DebugLevel)
		logrus.Debugf("debug output enabled")
	}
	if version {
		if utils.GitCommit != "" {
			logrus.Infof("flat-network-operator %v - %v", utils.Version, utils.GitCommit)
		} else {
			logrus.Infof("flat-network-operator %v", utils.Version)
		}
		return
	}

	ctx := signals.SetupSignalContext()
	cfg, err := kubeconfig.GetNonInteractiveClientConfig(kubeconfigFile).ClientConfig()
	if err != nil {
		logrus.Fatalf("Error building kubeconfig: %v", err)
	}

	wctx, err := wrangler.NewContext(cfg)
	if err != nil {
		logrus.Fatalf("Error build wrangler context: %v", err)
	}

	// Register handlers
	macvlanip.Register(ctx, wctx.Macvlan.MacvlanIP())
	macvlansubnet.Register(ctx, wctx.Macvlan.MacvlanSubnet(), wctx.Core.Pod().Cache())
	service.Register(ctx, wctx.Core.Service(), wctx.Core.Pod())
	ingress.Register(ctx, wctx.Networking.Ingress(), wctx.Core.Service())
	workload.Register(ctx,
		wctx.Apps.Deployment(), wctx.Apps.DaemonSet(), wctx.Apps.ReplicaSet(), wctx.Apps.StatefulSet())

	if err := wctx.Start(ctx, worker); err != nil {
		logrus.Fatalf("Failed to start context: %v", err)
	}

	<-ctx.Done()
	logrus.Infof("flat-network-operator stopped gracefully")
}

//go:generate go run pkg/codegen/cleanup/main.go
//go:generate go run pkg/codegen/main.go

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/cnrancher/rancher-flat-network/pkg/admission"
	"github.com/cnrancher/rancher-flat-network/pkg/controller/endpoints"
	"github.com/cnrancher/rancher-flat-network/pkg/controller/endpointslice"
	"github.com/cnrancher/rancher-flat-network/pkg/controller/flatnetworkip"
	"github.com/cnrancher/rancher-flat-network/pkg/controller/flatnetworksubnet"
	"github.com/cnrancher/rancher-flat-network/pkg/controller/ingress"
	"github.com/cnrancher/rancher-flat-network/pkg/controller/namespace"
	"github.com/cnrancher/rancher-flat-network/pkg/controller/pod"
	"github.com/cnrancher/rancher-flat-network/pkg/controller/service"
	"github.com/cnrancher/rancher-flat-network/pkg/controller/workload"
	"github.com/cnrancher/rancher-flat-network/pkg/controller/wrangler"
	"github.com/cnrancher/rancher-flat-network/pkg/logserver"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/rancher/wrangler/v3/pkg/kubeconfig"
	"github.com/rancher/wrangler/v3/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/writer"
)

var (
	masterURL      string
	kubeConfigFile string
	port           int
	address        string
	cert           string
	key            string
	worker         int
	webhookServer  bool
	version        bool
	versionString  string
	debug          bool
)

func init() {
	logrus.SetFormatter(&nested.Formatter{
		HideKeys:        false,
		TimestampFormat: time.DateTime,
		// TimestampFormat: time.RFC3339Nano,
		FieldsOrder: []string{"GID", "POD", "SVC", "IP", "SUBNET"},
	})
	logrus.SetOutput(io.Discard)
	logrus.AddHook(&writer.Hook{
		// Send logs with level higher than warning to stderr.
		Writer: os.Stderr,
		LogLevels: []logrus.Level{
			logrus.PanicLevel,
			logrus.FatalLevel,
			logrus.ErrorLevel,
			logrus.WarnLevel,
		},
	})
	logrus.AddHook(&writer.Hook{
		// Send info, debug and trace logs to stdout.
		Writer: os.Stdout,
		LogLevels: []logrus.Level{
			logrus.TraceLevel,
			logrus.InfoLevel,
			logrus.DebugLevel,
		},
	})

	if utils.GitCommit != "" {
		versionString = fmt.Sprintf("%v - %v", utils.Version, utils.GitCommit)
	} else {
		versionString = utils.Version
	}
}

func main() {
	flag.StringVar(&kubeConfigFile, "kubeconfig", "", "Kube-config file (optional)")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server (optional)")
	flag.IntVar(&port, "port", 443, "Webhook server port")
	flag.StringVar(&address, "bind-address", "0.0.0.0", "Webhook server bind address")
	flag.StringVar(&cert, "tls-cert-file", "/etc/webhook/certs/tls.crt", "Webhook server TLS x509 certificate")
	flag.StringVar(&key, "tls-private-key-file", "/etc/webhook/certs/tls.key", "Webhook server TLS x509 private key")
	flag.BoolVar(&webhookServer, "webhook-server", true, "Enable the Admission Webhook server")
	flag.BoolVar(&debug, "debug", false, "Enable debug log output")
	flag.BoolVar(&version, "v", false, "Output version")
	flag.IntVar(&worker, "worker", 5, "Worker number (1-50)")
	flag.Parse()

	if worker > 50 || worker < 1 {
		logrus.Warnf("invalid worker num: %v, should be 1-50, set to default: 5", worker)
		worker = 5
	}
	if debug || os.Getenv("CATTLE_DEV_MODE") != "" {
		logrus.SetLevel(logrus.DebugLevel)
		logrus.Debugf("debug output enabled")
	}
	if version {
		logrus.Infof("rancher-flat-network-operator %v", versionString)
		return
	}

	logrus.Infof("starting rancher-flat-network operator %v", versionString)
	ctx := signals.SetupSignalContext()
	logserver.StartServerWithDefaults(ctx)
	cfg, err := kubeconfig.GetNonInteractiveClientConfig(kubeConfigFile).ClientConfig()
	if err != nil {
		logrus.Fatalf("Error building kubeconfig: %v", err)
	}

	wctx := wrangler.NewContextOrDie(cfg)
	wctx.WaitForCacheSyncOrDie(ctx)

	// Register handlers
	flatnetworkip.Register(ctx, wctx)
	flatnetworksubnet.Register(ctx, wctx)
	service.Register(ctx, wctx)
	pod.Register(ctx, wctx)
	ingress.Register(ctx, wctx)
	workload.Register(ctx, wctx)
	endpoints.Register(ctx, wctx)
	endpointslice.Register(ctx, wctx)
	namespace.Register(ctx, wctx)

	wctx.OnLeader(func(ctx context.Context) error {
		logrus.Infof("pod [%v] is leader, starting handlers", utils.Hostname())

		// Start controller when this pod becomes leader.
		return wctx.StartHandler(ctx, worker)
	})
	wctx.Run(ctx)

	if webhookServer {
		webhook := admission.NewAdmissionWebhookServer(address, port, cert, key, wctx)
		if err := webhook.Run(ctx); err != nil {
			logrus.Fatalf("failed to start webhook: %v", err)
		}
	}

	select {}
}

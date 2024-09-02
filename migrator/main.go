package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/cnrancher/rancher-flat-network/pkg/upgrade"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/rancher/wrangler/v3/pkg/kubeconfig"
	"github.com/rancher/wrangler/v3/pkg/signals"
	"github.com/sirupsen/logrus"
)

var (
	kubeConfigFile string
	workloadKinds  string
	interval       time.Duration
	listLimit      int64
	autoYes        bool
	version        bool
	debug          bool

	versionString string
)

func init() {
	if utils.GitCommit != "" {
		versionString = fmt.Sprintf("%v - %v", utils.Version, utils.GitCommit)
	} else {
		versionString = utils.Version
	}
}

func main() {
	flag.StringVar(&kubeConfigFile, "kubeconfig", "", "Kube-config file (optional)")
	flag.StringVar(&workloadKinds, "workload",
		"deployment,daemonset,statefulset,cronjob,job", "Workload kinds, separated by comma")
	flag.DurationVar(&interval, "interval", time.Millisecond*500, "The interval between each Kube API requests")
	flag.Int64Var(&listLimit, "list-limit", 100, "Limit for each Kube API list request")
	flag.BoolVar(&autoYes, "yes", false, "Auto yes when migrating resources (default false)")
	flag.BoolVar(&version, "v", false, "Output version")
	flag.BoolVar(&debug, "debug", false, "Show debug output")
	flag.Parse()

	if debug || os.Getenv("CATTLE_DEV_MODE") != "" {
		logrus.SetLevel(logrus.DebugLevel)
		logrus.Debugf("debug output enabled")
	}
	if version {
		logrus.Infof("rancher-flat-network resource migrator %v", versionString)
		return
	}

	ctx := signals.SetupSignalContext()
	cfg, err := kubeconfig.GetNonInteractiveClientConfig(kubeConfigFile).ClientConfig()
	if err != nil {
		logrus.Fatalf("Error building kubeconfig: %v", err)
	}

	m := upgrade.NewResourceMigrator(&upgrade.MigratorOpts{
		Config:        cfg,
		WorkloadKinds: workloadKinds,
		Interval:      interval,
		ListLimit:     listLimit,
		AutoYes:       autoYes,
	})
	if err := m.Run(ctx); err != nil {
		logrus.Fatal(err)
	}
	logrus.Infof("finished migrating resources from 'macvlan.cluster.cattle.io' to 'flatnetwork.pandaria.io'")
}

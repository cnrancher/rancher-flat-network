package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cnrancher/rancher-flat-network/pkg/upgrade"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/rancher/wrangler/v3/pkg/kubeconfig"
	"github.com/rancher/wrangler/v3/pkg/signals"
	"github.com/sirupsen/logrus"
)

var (
	kubeConfigFile string
	workloadKinds  string
	versionString  string
	version        bool
	debug          bool
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
		"deployment,daemonset,statefulset,replicaset,cronjob,job", "Workload kinds, separated by comma")
	flag.BoolVar(&version, "v", false, "Output version")
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

	m := upgrade.NewResourceMigrator(cfg, workloadKinds)
	if err := m.Run(ctx); err != nil {
		logrus.Fatal(err)
	}
	logrus.Infof("finished migrating resources from 'macvlan.cluster.cattle.io' to 'flatnetwork.pandaria.io'")
}

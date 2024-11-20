package commands

import (
	"fmt"

	"github.com/cnrancher/rancher-flat-network/pkg/migrate"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/rancher/wrangler/v3/pkg/kubeconfig"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type migrateCmd struct {
	*baseCmd
	workloadKinds  string
	migrateService bool
}

func newMigrateCmd() *migrateCmd {
	cc := &migrateCmd{}
	cc.baseCmd = newBaseCmd(&cobra.Command{
		Use:     "migrate",
		Short:   "Migrate Rancher Macvlan (V1) CRD & Workloads & Services to FlatNetwork V2",
		Long:    "",
		Example: "rancher-flat-network-migrator migrate",
		PreRun: func(cmd *cobra.Command, args []string) {
			utils.SetupLogrus(cc.debug)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cc.run(); err != nil {
				return err
			}
			return nil
		},
	})

	flags := cc.baseCmd.cmd.Flags()
	flags.StringVarP(&cc.workloadKinds, "workload", "", "deployment,daemonset,statefulset,cronjob,job",
		"workload kinds to migrate, separated by comma")
	flags.BoolVarP(&cc.migrateService, "migrate-service", "", true, "migrate macvlan v1 services to v2")

	return cc
}

func (cc *migrateCmd) getCommand() *cobra.Command {
	return cc.cmd
}

func (cc *migrateCmd) run() error {
	cfg, err := kubeconfig.GetNonInteractiveClientConfigWithContext(cc.configFile, cc.context).ClientConfig()
	if err != nil {
		logrus.Fatalf("Error building kubeconfig: %v", err)
	}

	m := migrate.NewResourceMigrator(&migrate.MigratorOpts{
		Config:          cfg,
		WorkloadKinds:   cc.workloadKinds,
		MigrateServices: cc.migrateService,
		Interval:        cc.baseCmd.interval,
		ListLimit:       cc.baseCmd.listLimit,
		AutoYes:         cc.baseCmd.autoYes,
	})
	if err := m.Run(signalContext); err != nil {
		return fmt.Errorf("failed to migrate resource: %w", err)
	}
	logrus.Infof("Finished migrating resources from 'macvlan.cluster.cattle.io' to 'flatnetwork.pandaria.io'")
	logrus.Infof("Done")
	return nil
}

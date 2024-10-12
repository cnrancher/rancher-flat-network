package commands

import (
	"github.com/cnrancher/rancher-flat-network/pkg/migrate"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/rancher/wrangler/v3/pkg/kubeconfig"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type cleanCmd struct {
	*baseCmd
}

func newCleanCmd() *cleanCmd {
	cc := &cleanCmd{}
	cc.baseCmd = newBaseCmd(&cobra.Command{
		Use:     "clean",
		Short:   "Cleanup Macvlan (V1) CRD resources",
		Long:    "",
		Example: "rancher-flat-network-migrator clean",
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
	_ = flags

	return cc
}

func (cc *cleanCmd) getCommand() *cobra.Command {
	return cc.cmd
}

func (cc *cleanCmd) run() error {
	cfg, err := kubeconfig.GetNonInteractiveClientConfigWithContext(cc.configFile, cc.context).ClientConfig()
	if err != nil {
		logrus.Fatalf("Error building kubeconfig: %v", err)
	}

	m := migrate.NewResourceMigrator(&migrate.MigratorOpts{
		Config:    cfg,
		Interval:  cc.baseCmd.interval,
		ListLimit: cc.baseCmd.listLimit,
		AutoYes:   cc.autoYes,
	})

	if err := m.Clean(signalContext); err != nil {
		return err
	}
	logrus.Infof("Done")
	return nil
}

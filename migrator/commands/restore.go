package commands

import (
	"fmt"

	"github.com/cnrancher/rancher-flat-network/pkg/migrate"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/rancher/wrangler/v3/pkg/kubeconfig"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type restoreCmd struct {
	*baseCmd
	filePath string
}

func newRestoreCmd() *restoreCmd {
	cc := &restoreCmd{}
	cc.baseCmd = newBaseCmd(&cobra.Command{
		Use:     "restore",
		Short:   "Restore backuped V1 & V2 subnet CRD resources",
		Long:    "",
		Example: "rancher-flat-network-migrator restore",
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
	flags.StringVarP(&cc.filePath, "file", "f", "flat-network-backup-output.yaml", "backup file to restore")

	return cc
}

func (cc *restoreCmd) getCommand() *cobra.Command {
	return cc.cmd
}

func (cc *restoreCmd) run() error {
	if cc.filePath == "" {
		return fmt.Errorf("restore YAML file not specified")
	}
	cfg, err := kubeconfig.GetNonInteractiveClientConfigWithContext(cc.configFile, cc.context).ClientConfig()
	if err != nil {
		logrus.Fatalf("Error building kubeconfig: %v", err)
	}

	m := migrate.NewResourceMigrator(&migrate.MigratorOpts{
		Config:    cfg,
		Interval:  cc.baseCmd.interval,
		ListLimit: cc.baseCmd.listLimit,
		AutoYes:   cc.baseCmd.autoYes,
	})
	if err := m.Restore(signalContext, cc.filePath); err != nil {
		return err
	}

	logrus.Infof("Done")
	return nil
}

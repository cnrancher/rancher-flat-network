package commands

import (
	"fmt"
	"os"

	"github.com/cnrancher/rancher-flat-network/pkg/migrate"
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/rancher/wrangler/v3/pkg/kubeconfig"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type backupCmd struct {
	*baseCmd

	backupV1 bool
	backupV2 bool
	output   string
}

func newBackupCmd() *backupCmd {
	cc := &backupCmd{}
	cc.baseCmd = newBaseCmd(&cobra.Command{
		Use:     "backup",
		Short:   "Backup Macvlan (V1) & FlatNetwork (V2) subnet CRD resources",
		Long:    "",
		Example: "rancher-flat-network-migrator backup",
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
	flags.BoolVarP(&cc.backupV1, "v1", "", true,
		"backup V1 ('macvlansubnets.macvlan.cluster.cattle.io') CRD resources")
	flags.BoolVarP(&cc.backupV2, "v2", "", true,
		"backup V2 ('flatnetworksubnets.flatnetwork.pandaria.io') CRD resources")
	flags.StringVarP(&cc.output, "output", "o", "flat-network-backup-output.yaml", "backup output file")

	return cc
}

func (cc *backupCmd) getCommand() *cobra.Command {
	return cc.cmd
}

func (cc *backupCmd) run() error {
	if cc.output == "" {
		return fmt.Errorf("output file not specified")
	}

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

	if err := utils.CheckFileExistsPrompt(signalContext, cc.output, cc.autoYes); err != nil {
		return err
	}
	file, err := os.OpenFile(cc.output, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open %q: %w", cc.output, err)
	}
	defer file.Close()

	if cc.backupV1 {
		if err := m.BackupV1(signalContext, file); err != nil {
			return fmt.Errorf("failed to backup Rancher Macvlan V1 resources: %w", err)
		}
	}
	if cc.backupV2 {
		if err := m.BackupV2(signalContext, file); err != nil {
			return fmt.Errorf("failed to backup FlatNetwork V2 resources: %w", err)
		}
	}
	logrus.Infof("-----------------------------")
	logrus.Infof("Output CRD resources backup to %q", cc.output)
	logrus.Infof("You can use '%v restore' or 'kubectl create' to restore", os.Args[0])
	logrus.Infof("Done")
	return nil
}

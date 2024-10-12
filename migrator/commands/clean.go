package commands

import (
	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type cleanCmd struct {
	*baseCmd
}

func newCleanCmd() *cleanCmd {
	cc := &cleanCmd{}
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
	_ = flags

	return cc
}

func (cc *cleanCmd) getCommand() *cobra.Command {
	return cc.cmd
}

func (cc *cleanCmd) run() error {

	logrus.Infof("Done")
	return nil
}

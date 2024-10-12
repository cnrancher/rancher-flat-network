package commands

import (
	"fmt"

	"github.com/cnrancher/rancher-flat-network/pkg/utils"
	"github.com/spf13/cobra"
)

type versionCmd struct {
	*baseCmd
}

func newVersionCmd() *versionCmd {
	cc := &versionCmd{}
	cc.baseCmd = newBaseCmd(&cobra.Command{
		Use:     "version",
		Short:   "Show migrator version",
		Example: "rancher-flat-network-migrator version",
		PreRun: func(cmd *cobra.Command, args []string) {
			utils.SetupLogrus(cc.debug)
		},
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("rancher-flat-network-migrator version %s\n",
				getVersion())
		},
	})

	return cc
}

func getVersion() string {
	if utils.GitCommit != "" {
		return fmt.Sprintf("%v - %v", utils.Version, utils.GitCommit)
	}
	return utils.Version
}

func (cc *versionCmd) getCommand() *cobra.Command {
	return cc.cmd
}

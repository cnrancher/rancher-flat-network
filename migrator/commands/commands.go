package commands

import (
	"context"
	"time"

	"github.com/cnrancher/rancher-flat-network/pkg/signal"
	"github.com/spf13/cobra"
)

var (
	signalContext context.Context = signal.SetupSignalContext()
)

type baseCmd struct {
	*baseOpts
	cmd *cobra.Command
}

func newBaseCmd(cmd *cobra.Command) *baseCmd {
	return &baseCmd{
		cmd:      cmd,
		baseOpts: &globalOpts,
	}
}

type baseOpts struct {
	configFile string
	interval   time.Duration
	listLimit  int64
	autoYes    bool
	debug      bool
}

var globalOpts = baseOpts{}

type cmder interface {
	getCommand() *cobra.Command
}

func addCommands(root *cobra.Command, commands ...cmder) {
	for _, command := range commands {
		cmd := command.getCommand()
		if cmd == nil {
			continue
		}
		root.AddCommand(cmd)
	}
}

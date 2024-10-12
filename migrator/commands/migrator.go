package commands

import (
	"time"

	"github.com/spf13/cobra"
)

func Execute(args []string) error {
	migratorCmd := newMigratorCmd()
	migratorCmd.addCommands()
	migratorCmd.cmd.SetArgs(args)

	_, err := migratorCmd.cmd.ExecuteC()
	if err != nil {
		if signalContext.Err() != nil {
			return signalContext.Err()
		}
		return err
	}
	return nil
}

type migratorCmd struct {
	*baseCmd
}

func newMigratorCmd() *migratorCmd {
	cc := &migratorCmd{}
	cc.baseCmd = newBaseCmd(&cobra.Command{
		Use:  "rancher-flat-network-migrator",
		Long: "Cli for backup/restore/migrate FlatNetwork resources to V2",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	})
	cc.cmd.Version = getVersion()
	cc.cmd.SilenceUsage = true
	cc.cmd.SilenceErrors = true
	cc.cmd.CompletionOptions.HiddenDefaultCmd = true

	flags := cc.cmd.PersistentFlags()
	flags.StringVarP(&cc.baseCmd.configFile, "kubeconfig", "", "",
		"path to the kubeconfig file")
	flags.StringVarP(&cc.baseCmd.context, "context", "", "",
		"the name of the kubeconfig context")
	flags.DurationVarP(&cc.baseCmd.interval, "interval", "", time.Millisecond*500,
		"interval between each kube API requests")
	flags.Int64VarP(&cc.baseCmd.listLimit, "list-limit", "", 30,
		"limit for each kube API list requests")
	flags.BoolVarP(&cc.autoYes, "yes", "", false,
		"auto yes when migrating resources")
	flags.BoolVarP(&cc.baseCmd.debug, "debug", "", false, "enable debug output")

	return cc
}

func (cc *migratorCmd) getCommand() *cobra.Command {
	return cc.cmd
}

func (cc *migratorCmd) addCommands() {
	addCommands(
		cc.cmd,
		newMigrateCmd(),
		newBackupCmd(),
		newRestoreCmd(),
		newCleanCmd(),
		newVersionCmd(),
	)
}

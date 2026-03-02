package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/takaaki-s/devlb/internal/daemon"
	devlbexec "github.com/takaaki-s/devlb/internal/exec"
)

var execLabel string

var execCmd = &cobra.Command{
	Use:   "exec <port>[:<backend>][,<port>[:<backend>]...] -- <command> [args...]",
	Short: "Run a command with port interception",
	Long: `Execute a command while intercepting bind() calls to swap listen ports
with allocated backend ports. Multiple ports can be specified with commas.
The backends are automatically registered with the daemon and unregistered on exit.

Examples:
  devlb exec 3000 -- node server.js
  devlb exec 3000,8995 -- my-server
  devlb exec 3000:3001,8995:8996 -- my-server`,
	Args: cobra.MinimumNArgs(2),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return getListenPorts(), cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveDefault
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		listenPorts, backendPorts, err := devlbexec.ParseExecPortArgs(args[0])
		if err != nil {
			return err
		}

		client := daemon.NewClient(getSocketPath())
		if !client.IsRunning() {
			return fmt.Errorf("daemon not running. Start with: devlb start")
		}

		exitCode, err := devlbexec.RunWrapper(devlbexec.WrapperConfig{
			SocketPath:   getSocketPath(),
			ListenPorts:  listenPorts,
			BackendPorts: backendPorts,
			Label:        execLabel,
			Command:      args[1],
			Args:         args[2:],
		})
		if err != nil {
			return err
		}
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return nil
	},
}

func init() {
	execCmd.Flags().StringVar(&execLabel, "label", "", "Label for this backend (default: git branch name)")
	rootCmd.AddCommand(execCmd)
}

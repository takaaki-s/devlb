package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/takaaki-s/devlb/internal/daemon"
)

var switchCmd = &cobra.Command{
	Use:   "switch [port] <label>",
	Short: "Switch active backend by label",
	Long: `Switch the active backend to the one matching the given label.
With one argument, switches all ports.
With two arguments, switches only the specified listen port.`,
	Args: cobra.RangeArgs(1, 2),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		switch len(args) {
		case 0:
			return append(getListenPorts(), getLabels()...), cobra.ShellCompDirectiveNoFileComp
		case 1:
			return getLabels(), cobra.ShellCompDirectiveNoFileComp
		default:
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		client := daemon.NewClient(getSocketPath())
		if !client.IsRunning() {
			return fmt.Errorf("daemon not running. Start with: devlb start")
		}

		var listenPort int
		var label string

		if len(args) == 1 {
			label = args[0]
		} else {
			port, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid port: %s", args[0])
			}
			listenPort = port
			label = args[1]
		}

		if err := client.Switch(listenPort, label); err != nil {
			return err
		}

		if listenPort > 0 {
			fmt.Printf("Switched :%d → %s\n", listenPort, label)
		} else {
			fmt.Printf("Switched all ports → %s\n", label)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(switchCmd)
}

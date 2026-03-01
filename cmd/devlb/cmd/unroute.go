package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/takaaki-s/devlb/internal/daemon"
)

var unrouteCmd = &cobra.Command{
	Use:   "unroute <service>",
	Short: "Remove routing for a service",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		service := args[0]

		client := daemon.NewClient(getSocketPath())
		if !client.IsRunning() {
			return fmt.Errorf("daemon not running. Start with: devlb start")
		}

		if err := client.Unroute(service); err != nil {
			return err
		}

		fmt.Printf("Unrouted %s\n", service)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(unrouteCmd)
}

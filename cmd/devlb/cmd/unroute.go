package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/takaaki-s/devlb/internal/daemon"
)

var unrouteCmd = &cobra.Command{
	Use:   "unroute <port> <backend-port>",
	Short: "Remove a backend route",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		listenPort, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid listen port: %s", args[0])
		}
		backendPort, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid backend port: %s", args[1])
		}

		client := daemon.NewClient(getSocketPath())
		if !client.IsRunning() {
			return fmt.Errorf("daemon not running. Start with: devlb start")
		}

		if err := client.Unregister(listenPort, backendPort); err != nil {
			return err
		}

		fmt.Printf("Unrouted :%d → :%d\n", listenPort, backendPort)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(unrouteCmd)
}

package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/takaaki-s/devlb/internal/daemon"
)

var routeLabel string

var routeCmd = &cobra.Command{
	Use:   "route <service> <port>",
	Short: "Set routing for a service",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		service := args[0]
		port, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid port: %s", args[1])
		}

		client := daemon.NewClient(getSocketPath())
		if !client.IsRunning() {
			return fmt.Errorf("daemon not running. Start with: devlb start")
		}

		if err := client.Route(service, port, routeLabel); err != nil {
			return err
		}

		fmt.Printf("Routed %s → :%d", service, port)
		if routeLabel != "" {
			fmt.Printf(" [%s]", routeLabel)
		}
		fmt.Println()
		return nil
	},
}

func init() {
	routeCmd.Flags().StringVar(&routeLabel, "label", "", "Label for this route")
	rootCmd.AddCommand(routeCmd)
}

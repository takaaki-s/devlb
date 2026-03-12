package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/takaaki-s/devlb/internal/daemon"
	"github.com/takaaki-s/devlb/internal/label"
)

var routeLabel string

var routeCmd = &cobra.Command{
	Use:   "route <port> <backend-port>",
	Short: "Manually register a backend route",
	Long:  "Register a backend for a listen port. Use --label to identify the backend.",
	Args: cobra.ExactArgs(2),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return getListenPorts(), cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
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

		lbl := routeLabel
		if lbl == "" {
			lbl = label.RandomLabel()
		}
		if err := client.Register(listenPort, backendPort, lbl, 0, ""); err != nil {
			return err
		}

		if isJSON() {
			return printJSON(map[string]any{
				"listen_port":  listenPort,
				"backend_port": backendPort,
				"label":        lbl,
			})
		}

		fmt.Printf("Routed :%d → :%d [%s]", listenPort, backendPort, lbl)
		fmt.Println()
		return nil
	},
}

func init() {
	routeCmd.Flags().StringVar(&routeLabel, "label", "", "Label for this route")
	rootCmd.AddCommand(routeCmd)
}

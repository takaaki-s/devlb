package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/takaaki-s/devlb/internal/daemon"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show routing table",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := daemon.NewClient(getSocketPath())
		if !client.IsRunning() {
			return fmt.Errorf("daemon not running. Start with: devlb start")
		}

		status, err := client.Status()
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SERVICE\tLISTEN\tBACKEND\tLABEL\tSTATUS\tCONNS")

		for _, e := range status.Entries {
			listen := fmt.Sprintf(":%d", e.ListenPort)
			backend := "-"
			if e.BackendPort > 0 {
				backend = fmt.Sprintf(":%d", e.BackendPort)
			}
			label := e.Label
			if label == "" {
				label = "-"
			}

			statusIcon := "○"
			switch e.Status {
			case "active":
				statusIcon = "●"
			case "blocked":
				statusIcon = "✗"
			}

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s %s\t%d\n",
				e.Service, listen, backend, label,
				statusIcon, e.Status, e.ActiveConns)
		}

		return w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

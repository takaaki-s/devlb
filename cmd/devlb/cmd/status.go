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
		fmt.Fprintln(w, "PORT\tBACKEND\tLABEL\tSTATUS\tCONNS")

		for _, e := range status.Entries {
			listen := fmt.Sprintf(":%d", e.ListenPort)

			if len(e.Backends) > 0 {
				for _, b := range e.Backends {
					backend := fmt.Sprintf(":%d", b.Port)
					lbl := b.Label
					if lbl == "" {
						lbl = "-"
					}
					statusIcon := "○"
					statusText := "standby"
					if b.Active {
						statusIcon = "●"
						statusText = "active"
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s %s\t%d\n",
						listen, backend, lbl,
						statusIcon, statusText, e.ActiveConns)
				}
			} else {
				backend := "-"
				if e.BackendPort > 0 {
					backend = fmt.Sprintf(":%d", e.BackendPort)
				}
				lbl := e.Label
				if lbl == "" {
					lbl = "-"
				}
				statusIcon := "○"
				statusText := "idle"
				if e.BackendPort > 0 {
					statusIcon = "●"
					statusText = "active"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s %s\t%d\n",
					listen, backend, lbl,
					statusIcon, statusText, e.ActiveConns)
			}
		}

		return w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

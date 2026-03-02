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

		verbose, _ := cmd.Flags().GetBool("verbose")

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		if verbose {
			fmt.Fprintln(w, "PORT\tBACKEND\tLABEL\tSTATUS\tCONNS\tTOTAL\tBYTES_IN\tBYTES_OUT")
		} else {
			fmt.Fprintln(w, "PORT\tBACKEND\tLABEL\tSTATUS\tCONNS")
		}

		for _, e := range status.Entries {
			listen := fmt.Sprintf(":%d", e.ListenPort)

			if e.Status == "blocked" {
				blockedInfo := e.BlockedBy
				if blockedInfo == "" {
					blockedInfo = "unknown"
				}
				if verbose {
					fmt.Fprintf(w, "%s\t-\t-\t✗ blocked (%s)\t-\t-\t-\t-\n", listen, blockedInfo)
				} else {
					fmt.Fprintf(w, "%s\t-\t-\t✗ blocked (%s)\t-\n", listen, blockedInfo)
				}
				continue
			}

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
					if b.Healthy != nil && !*b.Healthy {
						statusIcon = "✗"
						statusText = "unhealthy"
					}
					if verbose {
						fmt.Fprintf(w, "%s\t%s\t%s\t%s %s\t%d\t%d\t%s\t%s\n",
							listen, backend, lbl,
							statusIcon, statusText, b.ActiveConns,
							b.TotalConns, formatBytes(b.BytesIn), formatBytes(b.BytesOut))
					} else {
						fmt.Fprintf(w, "%s\t%s\t%s\t%s %s\t%d\n",
							listen, backend, lbl,
							statusIcon, statusText, e.ActiveConns)
					}
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
				if verbose {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s %s\t%d\t-\t-\t-\n",
						listen, backend, lbl,
						statusIcon, statusText, e.ActiveConns)
				} else {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s %s\t%d\n",
						listen, backend, lbl,
						statusIcon, statusText, e.ActiveConns)
				}
			}
		}

		return w.Flush()
	},
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%dB", b)
	}
}

func init() {
	statusCmd.Flags().BoolP("verbose", "v", false, "Show detailed metrics (total conns, bytes in/out)")
	rootCmd.AddCommand(statusCmd)
}

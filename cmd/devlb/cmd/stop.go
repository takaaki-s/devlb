package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/takaaki-s/devlb/internal/daemon"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the devlb daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := daemon.NewClient(getSocketPath())
		if !client.IsRunning() {
			if isJSON() {
				return printJSON(map[string]any{
					"stopped":         false,
					"already_stopped": true,
				})
			}
			fmt.Println("Daemon is not running")
			return nil
		}

		if err := client.Stop(); err != nil {
			return err
		}

		// Poll until stopped
		for i := 0; i < 30; i++ {
			if !client.IsRunning() {
				if isJSON() {
					return printJSON(map[string]any{
						"stopped": true,
					})
				}
				fmt.Println("Daemon stopped")
				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}

		if isJSON() {
			if jsonErr := printJSON(map[string]any{
				"stopped": false,
				"error":   "daemon did not stop in time",
			}); jsonErr != nil {
				return jsonErr
			}
		}
		return fmt.Errorf("daemon did not stop in time")
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

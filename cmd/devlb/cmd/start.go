package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/takaaki-s/devlb/internal/daemon"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the devlb daemon in the background",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := daemon.NewClient(getSocketPath())
		if client.IsRunning() {
			fmt.Println("Daemon is already running")
			return nil
		}

		exe, err := os.Executable()
		if err != nil {
			return err
		}

		daemonCmd := exec.Command(exe, "daemon")
		daemonCmd.Env = os.Environ()
		daemonCmd.Stdout = nil
		daemonCmd.Stderr = nil
		daemonCmd.Stdin = nil

		if err := daemonCmd.Start(); err != nil {
			return fmt.Errorf("failed to start daemon: %w", err)
		}

		fmt.Printf("Daemon started (PID: %d)\n", daemonCmd.Process.Pid)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
}

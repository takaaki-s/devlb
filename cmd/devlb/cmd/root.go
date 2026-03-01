package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "devlb",
	Short: "Local development TCP reverse proxy",
	Long:  `devlb is a local TCP reverse proxy for routing traffic between multiple worktrees.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func getConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".devlb")
}

func getSocketPath() string {
	return filepath.Join(getConfigDir(), "run", "daemon.sock")
}

func getConfigPath() string {
	return filepath.Join(getConfigDir(), "devlb.yaml")
}

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/takaaki-s/devlb/internal/config"
	"github.com/takaaki-s/devlb/internal/daemon"
)

// daemonCmd runs the daemon in the foreground (hidden, called by start)
var daemonCmd = &cobra.Command{
	Use:    "daemon",
	Short:  "Run the daemon in the foreground",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := getConfigPath()
		cfg, err := config.LoadConfig(cfgPath)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		dataDir := getConfigDir()
		socketDir := filepath.Join(dataDir, "run")
		if err := os.MkdirAll(socketDir, 0755); err != nil {
			return err
		}

		sm, err := config.NewStateManager(dataDir)
		if err != nil {
			return fmt.Errorf("failed to initialize state: %w", err)
		}

		socketPath := getSocketPath()
		srv, err := daemon.NewServer(socketPath, cfg, sm)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}

		fmt.Println("devlb daemon started")
		return srv.Start()
	},
}

func init() {
	rootCmd.AddCommand(daemonCmd)
}

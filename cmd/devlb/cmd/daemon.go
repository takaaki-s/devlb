package cmd

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

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
		srv, err := daemon.NewServerWithConfigPath(socketPath, cfgPath, cfg, sm)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}

		// Signal handling for graceful shutdown
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		errCh := make(chan error, 1)
		go func() {
			errCh <- srv.Start()
		}()

		select {
		case sig := <-sigCh:
			log.Printf("received %s, draining connections...", sig)
			srv.StopGraceful(daemon.DefaultDrainTimeout)
			return nil
		case err := <-errCh:
			return err
		}
	},
}

func init() {
	rootCmd.AddCommand(daemonCmd)
}

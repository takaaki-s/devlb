package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/takaaki-s/devlb/internal/daemon"
)

var (
	outputFormat string
	debug        bool
	version      = "dev"
)

func isJSON() bool {
	return outputFormat == "json"
}

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

var rootCmd = &cobra.Command{
	Use:   "devlb",
	Short: "Local development TCP reverse proxy",
	Long:  `devlb is a local TCP reverse proxy for routing traffic between multiple worktrees.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		switch outputFormat {
		case "text", "json":
			return nil
		default:
			return fmt.Errorf("invalid output format %q: must be text or json", outputFormat)
		}
	},
}

func init() {
	rootCmd.Version = version
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "text", "Output format: text, json")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug logging")
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

// Completion helpers — query daemon for available values

func getListenPorts() []string {
	client := daemon.NewClient(getSocketPath())
	status, err := client.Status()
	if err != nil {
		return nil
	}
	seen := make(map[int]bool)
	var ports []string
	for _, e := range status.Entries {
		if !seen[e.ListenPort] {
			ports = append(ports, fmt.Sprintf("%d", e.ListenPort))
			seen[e.ListenPort] = true
		}
	}
	return ports
}

func getLabels() []string {
	client := daemon.NewClient(getSocketPath())
	status, err := client.Status()
	if err != nil {
		return nil
	}
	seen := make(map[string]bool)
	var labels []string
	for _, e := range status.Entries {
		for _, b := range e.Backends {
			if b.Label != "" && !seen[b.Label] {
				labels = append(labels, b.Label)
				seen[b.Label] = true
			}
		}
	}
	return labels
}

func getBackendPorts(listenPortStr string) []string {
	client := daemon.NewClient(getSocketPath())
	status, err := client.Status()
	if err != nil {
		return nil
	}
	var ports []string
	for _, e := range status.Entries {
		if fmt.Sprintf("%d", e.ListenPort) == listenPortStr {
			for _, b := range e.Backends {
				ports = append(ports, fmt.Sprintf("%d", b.Port))
			}
		}
	}
	return ports
}

package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/takaaki-s/devlb/internal/daemon"
	tuipkg "github.com/takaaki-s/devlb/internal/tui"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive dashboard",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := daemon.NewClient(getSocketPath())
		if !client.IsRunning() {
			return fmt.Errorf("daemon not running. Start with: devlb start")
		}
		m := tuipkg.NewModel(client)
		p := tea.NewProgram(m, tea.WithAltScreen())
		_, err := p.Run()
		return err
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}

package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Header style for the title bar
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("57")).
			Padding(0, 1)

	// Column header style
	ColumnHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("252")).
				Underline(true)

	// Active backend row
	ActiveStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("82"))

	// Standby backend row
	StandbyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	// Unhealthy backend row
	UnhealthyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	// Idle service (no backends)
	IdleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	// Selected row highlight
	SelectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Bold(true)

	// Status indicator symbols
	ActiveIndicator   = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render("●")
	StandbyIndicator  = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("○")
	UnhealthyIndicator = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("✗")
	IdleIndicator     = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("○")

	// Help bar at bottom
	HelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	// Error message style
	ErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
)

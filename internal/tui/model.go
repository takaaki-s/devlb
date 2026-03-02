package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/takaaki-s/devlb/internal/daemon"
)

const refreshInterval = 1 * time.Second

// StatusClient abstracts daemon client for testability.
type StatusClient interface {
	Status() (*daemon.StatusResponse, error)
	Switch(listenPort int, label string) error
}

// Messages
type TickMsg time.Time

type StatusMsg struct {
	Entries []daemon.StatusEntry
	Err     error
}

type SwitchDoneMsg struct {
	Err error
}

// Model is the bubbletea model for the TUI dashboard.
type Model struct {
	client  StatusClient
	entries []daemon.StatusEntry
	rows    []RowInfo
	cursor  int
	err     error
	width   int
	height  int
}

// NewModel creates a new TUI model.
func NewModel(client StatusClient) Model {
	return Model{
		client: client,
		cursor: 0,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(fetchStatus(m.client), tickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case TickMsg:
		return m, fetchStatus(m.client)

	case StatusMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, tickCmd()
		}
		m.err = nil
		m.entries = msg.Entries
		m.rows = FlattenEntries(msg.Entries)
		// Clamp cursor
		if m.cursor >= len(m.rows) {
			m.cursor = max(0, len(m.rows)-1)
		}
		return m, tickCmd()

	case SwitchDoneMsg:
		if msg.Err != nil {
			m.err = msg.Err
		}
		return m, fetchStatus(m.client)
	}

	return m, nil
}

func (m Model) View() string {
	var s string

	// Title
	title := HeaderStyle.Render(" devlb dashboard ")
	refreshInfo := HelpStyle.Render(fmt.Sprintf("  auto-refresh: %s", refreshInterval))
	s += title + refreshInfo + "\n\n"

	// Error
	if m.err != nil {
		s += ErrorStyle.Render(fmt.Sprintf("  Error: %s", m.err)) + "\n\n"
	}

	// Table
	width := m.width
	if width == 0 {
		width = 80
	}
	s += RenderTable(m.entries, m.cursor, width)
	s += "\n\n"

	// Help bar
	s += HelpStyle.Render("  ↑↓ select  s switch  r refresh  q quit")

	return s
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case "down", "j":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
		return m, nil

	case "s":
		if m.cursor >= 0 && m.cursor < len(m.rows) {
			row := m.rows[m.cursor]
			if !row.IsIdle && row.Backend.Label != "" {
				return m, switchBackend(m.client, row.ListenPort, row.Backend.Label)
			}
		}
		return m, nil

	case "r":
		return m, fetchStatus(m.client)
	}

	return m, nil
}

func fetchStatus(client StatusClient) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.Status()
		if err != nil {
			return StatusMsg{Err: err}
		}
		return StatusMsg{Entries: resp.Entries}
	}
}

func switchBackend(client StatusClient, listenPort int, label string) tea.Cmd {
	return func() tea.Msg {
		err := client.Switch(listenPort, label)
		return SwitchDoneMsg{Err: err}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

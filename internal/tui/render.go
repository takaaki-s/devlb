package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/takaaki-s/devlb/internal/daemon"
)

// RowInfo represents a single selectable row in the TUI table.
type RowInfo struct {
	ListenPort int
	Service    string
	Backend    daemon.BackendInfo
	IsIdle     bool
}

// FlattenEntries converts StatusEntries into flat rows (one per backend, or one idle row per service).
func FlattenEntries(entries []daemon.StatusEntry) []RowInfo {
	var rows []RowInfo
	for _, e := range entries {
		if len(e.Backends) == 0 {
			rows = append(rows, RowInfo{
				ListenPort: e.ListenPort,
				Service:    e.Service,
				IsIdle:     true,
			})
			continue
		}
		for _, b := range e.Backends {
			rows = append(rows, RowInfo{
				ListenPort: e.ListenPort,
				Service:    e.Service,
				Backend:    b,
			})
		}
	}
	return rows
}

// FormatBytes formats byte count into human-readable string.
func FormatBytes(b int64) string {
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

// RenderTable renders the status table as a styled string.
func RenderTable(entries []daemon.StatusEntry, cursor int, width int) string {
	if len(entries) == 0 {
		return IdleStyle.Render("No services configured")
	}

	rows := FlattenEntries(entries)
	if len(rows) == 0 {
		return IdleStyle.Render("No services configured")
	}

	var b strings.Builder

	// Header
	header := fmt.Sprintf("  %-7s %-9s %-14s %-14s %5s %8s %8s",
		"PORT", "BACKEND", "LABEL", "STATUS", "CONNS", "  IN", " OUT")
	b.WriteString(ColumnHeaderStyle.Render(header))
	b.WriteString("\n")

	lastPort := 0
	for i, row := range rows {
		prefix := "  "
		if i == cursor {
			prefix = "> "
		}

		var line string
		if row.IsIdle {
			portStr := fmt.Sprintf(":%d", row.ListenPort)
			line = fmt.Sprintf("%s%-7s %-9s %-14s %-14s %5s %8s %8s",
				prefix, portStr, "-", "-", IdleIndicator+" idle", "-", "-", "-")
			line = IdleStyle.Render(line)
		} else {
			portStr := ""
			if row.ListenPort != lastPort {
				portStr = fmt.Sprintf(":%d", row.ListenPort)
			}

			backendStr := fmt.Sprintf(":%d", row.Backend.Port)
			label := row.Backend.Label
			if label == "" {
				label = "-"
			}

			status := backendStatus(row.Backend)
			conns := fmt.Sprintf("%d", row.Backend.ActiveConns)
			bytesIn := FormatBytes(row.Backend.BytesIn)
			bytesOut := FormatBytes(row.Backend.BytesOut)

			line = fmt.Sprintf("%s%-7s %-9s %-14s %-14s %5s %8s %8s",
				prefix, portStr, backendStr, label, status, conns, bytesIn, bytesOut)

			line = styleRow(row.Backend, line)
		}

		if i == cursor {
			line = SelectedStyle.Render(line)
		}

		b.WriteString(line)
		if i < len(rows)-1 {
			b.WriteString("\n")
		}
		lastPort = row.ListenPort
	}

	return b.String()
}

func backendStatus(b daemon.BackendInfo) string {
	if b.Healthy != nil && !*b.Healthy {
		return UnhealthyIndicator + " unhealthy"
	}
	if b.Active {
		return ActiveIndicator + " active"
	}
	return StandbyIndicator + " standby"
}

func styleRow(b daemon.BackendInfo, line string) string {
	if b.Healthy != nil && !*b.Healthy {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(line)
	}
	if b.Active {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render(line)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(line)
}

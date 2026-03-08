package tui

import (
	"strings"
	"testing"

	"github.com/takaaki-s/devlb/internal/daemon"
)

func TestRenderTable_NoEntries(t *testing.T) {
	out := RenderTable(nil, -1, 80)
	if !strings.Contains(out, "No services") {
		t.Errorf("expected 'No services' for empty entries, got:\n%s", out)
	}
}

func TestRenderTable_SingleIdleService(t *testing.T) {
	entries := []daemon.StatusEntry{
		{Service: "api", ListenPort: 3000, Status: "idle"},
	}
	out := RenderTable(entries, -1, 80)
	if !strings.Contains(out, "3000") {
		t.Errorf("expected port 3000 in output, got:\n%s", out)
	}
	if !strings.Contains(out, "idle") {
		t.Errorf("expected 'idle' in output, got:\n%s", out)
	}
}

func TestRenderTable_MultipleBackends(t *testing.T) {
	healthy := true
	entries := []daemon.StatusEntry{
		{
			Service:    "api",
			ListenPort: 3000,
			Status:     "active",
			Backends: []daemon.BackendInfo{
				{Port: 3001, Label: "worktree-a", Active: true, Healthy: &healthy, ActiveConns: 5, BytesIn: 1024, BytesOut: 2048},
				{Port: 3002, Label: "worktree-b", Active: false, Healthy: &healthy, ActiveConns: 0},
			},
		},
	}
	out := RenderTable(entries, -1, 80)
	if !strings.Contains(out, "3001") {
		t.Errorf("expected backend port 3001, got:\n%s", out)
	}
	if !strings.Contains(out, "worktree-a") {
		t.Errorf("expected label worktree-a, got:\n%s", out)
	}
	if !strings.Contains(out, "worktree-b") {
		t.Errorf("expected label worktree-b, got:\n%s", out)
	}
	if !strings.Contains(out, "active") {
		t.Errorf("expected 'active' status, got:\n%s", out)
	}
	if !strings.Contains(out, "standby") {
		t.Errorf("expected 'standby' status, got:\n%s", out)
	}
}

func TestRenderTable_UnhealthyBackend(t *testing.T) {
	healthy := true
	unhealthy := false
	entries := []daemon.StatusEntry{
		{
			Service:    "api",
			ListenPort: 8080,
			Status:     "active",
			Backends: []daemon.BackendInfo{
				{Port: 8081, Label: "main", Active: true, Healthy: &healthy},
				{Port: 8082, Label: "feature-x", Active: false, Healthy: &unhealthy, LastError: "dial failed"},
			},
		},
	}
	out := RenderTable(entries, -1, 80)
	if !strings.Contains(out, "unhealthy") {
		t.Errorf("expected 'unhealthy' for failed backend, got:\n%s", out)
	}
}

func TestRenderTable_CursorHighlight(t *testing.T) {
	healthy := true
	entries := []daemon.StatusEntry{
		{
			Service:    "api",
			ListenPort: 3000,
			Status:     "active",
			Backends: []daemon.BackendInfo{
				{Port: 3001, Label: "worktree-a", Active: true, Healthy: &healthy},
				{Port: 3002, Label: "worktree-b", Active: false, Healthy: &healthy},
			},
		},
	}
	// Cursor=0: first row should have ">" prefix
	out0 := RenderTable(entries, 0, 80)
	if !strings.Contains(out0, "> ") {
		t.Errorf("expected '> ' cursor indicator, got:\n%s", out0)
	}
	// Cursor on different rows should produce different output
	out1 := RenderTable(entries, 1, 80)
	if out0 == out1 {
		t.Errorf("cursor position should affect rendering")
	}
}

func TestFlattenEntries(t *testing.T) {
	healthy := true
	entries := []daemon.StatusEntry{
		{
			Service:    "api",
			ListenPort: 3000,
			Status:     "active",
			Backends: []daemon.BackendInfo{
				{Port: 3001, Label: "worktree-a", Active: true, Healthy: &healthy},
				{Port: 3002, Label: "worktree-b", Active: false, Healthy: &healthy},
			},
		},
		{
			Service:    "auth",
			ListenPort: 4000,
			Status:     "idle",
		},
	}
	rows := FlattenEntries(entries)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (2 backends + 1 idle), got %d", len(rows))
	}
	// First row: backend of api
	if rows[0].ListenPort != 3000 || rows[0].Backend.Port != 3001 {
		t.Errorf("row 0: expected api:3000→3001, got %d→%d", rows[0].ListenPort, rows[0].Backend.Port)
	}
	// Second row: second backend of api
	if rows[1].ListenPort != 3000 || rows[1].Backend.Port != 3002 {
		t.Errorf("row 1: expected api:3000→3002, got %d→%d", rows[1].ListenPort, rows[1].Backend.Port)
	}
	// Third row: idle auth
	if rows[2].ListenPort != 4000 {
		t.Errorf("row 2: expected ListenPort=4000, got %d", rows[2].ListenPort)
	}
	if !rows[2].IsIdle {
		t.Errorf("row 2: expected idle, got IsIdle=%v", rows[2].IsIdle)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0B"},
		{512, "512B"},
		{1024, "1.0K"},
		{1536, "1.5K"},
		{1048576, "1.0M"},
		{1073741824, "1.0G"},
	}
	for _, tt := range tests {
		got := FormatBytes(tt.input)
		if got != tt.expected {
			t.Errorf("FormatBytes(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

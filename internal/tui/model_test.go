package tui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/takaaki-s/devlb/internal/daemon"
)

// mockClient implements StatusClient for testing.
type mockClient struct {
	entries []daemon.StatusEntry
	err     error
	switchCalls []switchCall
}

type switchCall struct {
	listenPort int
	label      string
}

func (m *mockClient) Status() (*daemon.StatusResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &daemon.StatusResponse{Entries: m.entries}, nil
}

func (m *mockClient) Switch(listenPort int, label string) error {
	m.switchCalls = append(m.switchCalls, switchCall{listenPort, label})
	return nil
}

func TestModelUpdate_TickFetchesStatus(t *testing.T) {
	healthy := true
	client := &mockClient{
		entries: []daemon.StatusEntry{
			{Service: "api", ListenPort: 3000, Status: "active", Backends: []daemon.BackendInfo{
				{Port: 3001, Label: "a", Active: true, Healthy: &healthy},
			}},
		},
	}
	m := NewModel(client)

	// Simulate receiving a StatusMsg
	msg := StatusMsg{
		Entries: client.entries,
		Err:     nil,
	}
	newModel, _ := m.Update(msg)
	model := newModel.(Model)

	if len(model.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(model.entries))
	}
	if len(model.rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(model.rows))
	}
}

func TestModelUpdate_CursorMove(t *testing.T) {
	healthy := true
	client := &mockClient{
		entries: []daemon.StatusEntry{
			{Service: "api", ListenPort: 3000, Status: "active", Backends: []daemon.BackendInfo{
				{Port: 3001, Label: "a", Active: true, Healthy: &healthy},
				{Port: 3002, Label: "b", Active: false, Healthy: &healthy},
			}},
		},
	}
	m := NewModel(client)
	// Populate rows
	updated, _ := m.Update(StatusMsg{Entries: client.entries})
	model := updated.(Model)

	// Move down
	m2, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model2 := m2.(Model)
	if model2.cursor != 1 {
		t.Errorf("expected cursor=1 after down, got %d", model2.cursor)
	}

	// Move down again should clamp
	m3, _ := model2.Update(tea.KeyMsg{Type: tea.KeyDown})
	model3 := m3.(Model)
	if model3.cursor != 1 {
		t.Errorf("expected cursor=1 (clamped), got %d", model3.cursor)
	}

	// Move up
	m4, _ := model3.Update(tea.KeyMsg{Type: tea.KeyUp})
	model4 := m4.(Model)
	if model4.cursor != 0 {
		t.Errorf("expected cursor=0 after up, got %d", model4.cursor)
	}

	// Move up again should clamp at 0
	m5, _ := model4.Update(tea.KeyMsg{Type: tea.KeyUp})
	model5 := m5.(Model)
	if model5.cursor != 0 {
		t.Errorf("expected cursor=0 (clamped), got %d", model5.cursor)
	}
}

func TestModelUpdate_Quit(t *testing.T) {
	client := &mockClient{}
	m := NewModel(client)

	for _, key := range []string{"q", "esc"} {
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		if cmd == nil {
			// Try esc as KeyType
			if key == "esc" {
				_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			}
		}
		if cmd == nil {
			t.Errorf("expected quit cmd for key %q", key)
		}
	}
}

func TestModelUpdate_Switch(t *testing.T) {
	healthy := true
	client := &mockClient{
		entries: []daemon.StatusEntry{
			{Service: "api", ListenPort: 3000, Status: "active", Backends: []daemon.BackendInfo{
				{Port: 3001, Label: "worktree-a", Active: true, Healthy: &healthy},
				{Port: 3002, Label: "worktree-b", Active: false, Healthy: &healthy},
			}},
		},
	}
	m := NewModel(client)
	// Populate rows
	updated, _ := m.Update(StatusMsg{Entries: client.entries})
	model := updated.(Model)

	// Move cursor to second row (worktree-b)
	m2, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model2 := m2.(Model)

	// Press 's' to switch
	_, cmd := model2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd == nil {
		t.Fatal("expected switch command, got nil")
	}
}

func TestModelUpdate_StatusError(t *testing.T) {
	client := &mockClient{err: fmt.Errorf("connection refused")}
	m := NewModel(client)

	msg := StatusMsg{Err: fmt.Errorf("connection refused")}
	newModel, _ := m.Update(msg)
	model := newModel.(Model)

	if model.err == nil {
		t.Fatal("expected error to be set")
	}
}

func TestModelView_ShowsError(t *testing.T) {
	client := &mockClient{}
	m := NewModel(client)
	m2, _ := m.Update(StatusMsg{Err: fmt.Errorf("daemon not running")})
	model := m2.(Model)

	view := model.View()
	if view == "" {
		t.Error("expected non-empty view on error")
	}
}

func TestModelUpdate_Refresh(t *testing.T) {
	client := &mockClient{
		entries: []daemon.StatusEntry{
			{Service: "api", ListenPort: 3000, Status: "idle"},
		},
	}
	m := NewModel(client)

	// Press 'r' to refresh
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Fatal("expected refresh command, got nil")
	}
}

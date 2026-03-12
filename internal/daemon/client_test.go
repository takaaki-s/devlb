package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/takaaki-s/devlb/internal/config"
)

func TestClientIsRunningFalse(t *testing.T) {
	c := NewClient("/tmp/nonexistent-devlb-test.sock")
	if c.IsRunning() {
		t.Error("expected IsRunning=false for nonexistent socket")
	}
}

func setupClientTestServer(t *testing.T) (*Server, *Client) {
	t.Helper()
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "daemon.sock")

	cfgPath := filepath.Join(dir, "devlb.yaml")
	_ = os.WriteFile(cfgPath, []byte(`services:
  - name: api
    port: 0
  - name: auth
    port: 0
`), 0644)

	cfg, _ := config.LoadConfig(cfgPath)
	sm, _ := config.NewStateManager(dir)
	srv, _ := NewServer(socketPath, cfg, sm)

	go func() { _ = srv.Start() }()

	// Wait for socket
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	client := NewClient(socketPath)
	return srv, client
}

func TestClientIsRunningTrue(t *testing.T) {
	srv, client := setupClientTestServer(t)
	defer srv.Stop()

	if !client.IsRunning() {
		t.Error("expected IsRunning=true")
	}
}

func TestClientRoute(t *testing.T) {
	srv, client := setupClientTestServer(t)
	defer srv.Stop()

	if err := client.Route("api", 13000, "test-label"); err != nil {
		t.Fatalf("Route failed: %v", err)
	}
}

func TestClientRouteUnknownService(t *testing.T) {
	srv, client := setupClientTestServer(t)
	defer srv.Stop()

	err := client.Route("nonexistent", 13000, "test")
	if err == nil {
		t.Error("expected error for unknown service")
	}
}

func TestClientUnroute(t *testing.T) {
	srv, client := setupClientTestServer(t)
	defer srv.Stop()

	_ = client.Route("api", 13000, "test")
	if err := client.Unroute("api"); err != nil {
		t.Fatalf("Unroute failed: %v", err)
	}
}

func TestClientStatus(t *testing.T) {
	srv, client := setupClientTestServer(t)
	defer srv.Stop()

	status, err := client.Status()
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if len(status.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(status.Entries))
	}
}

func TestClientStatusAfterRoute(t *testing.T) {
	srv, client := setupClientTestServer(t)
	defer srv.Stop()

	_ = client.Route("api", 13000, "feat-x")

	status, err := client.Status()
	if err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, e := range status.Entries {
		if e.Service == "api" {
			found = true
			if e.BackendPort != 13000 {
				t.Errorf("expected BackendPort=13000, got %d", e.BackendPort)
			}
			if e.Label != "feat-x" {
				t.Errorf("expected Label=feat-x, got %s", e.Label)
			}
			if e.Status != "active" {
				t.Errorf("expected Status=active, got %s", e.Status)
			}
		}
	}
	if !found {
		t.Error("api entry not found in status")
	}
}

func TestClientStop(t *testing.T) {
	srv, client := setupClientTestServer(t)
	_ = srv // will be stopped by client

	if err := client.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Give it time to stop
	time.Sleep(200 * time.Millisecond)

	if client.IsRunning() {
		t.Error("expected daemon to be stopped")
	}
}

// --- Phase 2 client tests ---

func TestClientRegister(t *testing.T) {
	srv, client := setupClientTestServer(t)
	defer srv.Stop()

	// Get listen port from status
	status, _ := client.Status()
	var listenPort int
	for _, e := range status.Entries {
		if e.Service == "api" {
			listenPort = e.ListenPort
			break
		}
	}

	if err := client.Register(listenPort, 13001, "worktree-a", 0, ""); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	status2, _ := client.Status()
	found := false
	for _, e := range status2.Entries {
		if e.ListenPort == listenPort {
			for _, b := range e.Backends {
				if b.Port == 13001 && b.Label == "worktree-a" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("registered backend not found")
	}
}

func TestClientUnregister(t *testing.T) {
	srv, client := setupClientTestServer(t)
	defer srv.Stop()

	status, _ := client.Status()
	var listenPort int
	for _, e := range status.Entries {
		if e.Service == "api" {
			listenPort = e.ListenPort
			break
		}
	}

	_ = client.Register(listenPort, 13001, "a", 0, "")
	if err := client.Unregister(listenPort, 13001); err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}
}

func TestClientSwitch(t *testing.T) {
	srv, client := setupClientTestServer(t)
	defer srv.Stop()

	status, _ := client.Status()
	var listenPort int
	for _, e := range status.Entries {
		if e.Service == "api" {
			listenPort = e.ListenPort
			break
		}
	}

	_ = client.Register(listenPort, 13001, "a", 0, "")
	_ = client.Register(listenPort, 13002, "b", 0, "")

	if err := client.Switch(listenPort, "b"); err != nil {
		t.Fatalf("Switch failed: %v", err)
	}

	status2, _ := client.Status()
	for _, e := range status2.Entries {
		if e.ListenPort == listenPort {
			for _, b := range e.Backends {
				if b.Label == "b" && !b.Active {
					t.Error("b should be active after switch")
				}
			}
		}
	}
}

func TestClientAllocate(t *testing.T) {
	srv, client := setupClientTestServer(t)
	defer srv.Stop()

	port, err := client.Allocate(3000)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if port <= 0 {
		t.Errorf("expected positive port, got %d", port)
	}
}

// Suppress unused import warning
var _ = json.Marshal

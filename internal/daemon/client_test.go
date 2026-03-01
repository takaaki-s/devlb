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

// Suppress unused import warning
var _ = json.Marshal

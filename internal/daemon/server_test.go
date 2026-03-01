package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/takaaki-s/devlb/internal/config"
)

func setupTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "daemon.sock")

	// Create config
	cfgPath := filepath.Join(dir, "devlb.yaml")
	cfgData := []byte(`services:
  - name: api
    port: 0
  - name: auth
    port: 0
`)
	if err := os.WriteFile(cfgPath, cfgData, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	sm, err := config.NewStateManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	srv, err := NewServer(socketPath, cfg, sm)
	if err != nil {
		t.Fatal(err)
	}
	return srv, socketPath
}

func startTestServer(t *testing.T, srv *Server) {
	t.Helper()
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// Wait for socket to become available
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(srv.socketPath); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server did not start in time")
}

func sendRequest(t *testing.T, socketPath string, req Request) Response {
	t.Helper()
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		t.Fatalf("encode: %v", err)
	}

	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}

func TestServerStartStop(t *testing.T) {
	srv, socketPath := setupTestServer(t)
	startTestServer(t, srv)
	defer srv.Stop()

	// Socket should exist
	if _, err := os.Stat(socketPath); err != nil {
		t.Fatalf("socket should exist: %v", err)
	}

	// Send stop
	resp := sendRequest(t, socketPath, Request{Action: ActionStop})
	if !resp.Success {
		t.Errorf("stop should succeed: %s", resp.Error)
	}

	// Wait for server to actually stop
	time.Sleep(200 * time.Millisecond)
}

func TestServerRoute(t *testing.T) {
	srv, socketPath := setupTestServer(t)
	startTestServer(t, srv)
	defer srv.Stop()

	// Start a test HTTP backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "proxied")
	}))
	defer backend.Close()

	backendPort := portFromAddr(t, backend.Listener.Addr().String())

	// Route api to backend
	data, _ := json.Marshal(RouteRequest{Service: "api", Port: backendPort, Label: "test"})
	resp := sendRequest(t, socketPath, Request{Action: ActionRoute, Data: data})
	if !resp.Success {
		t.Fatalf("route failed: %s", resp.Error)
	}
}

func TestServerUnroute(t *testing.T) {
	srv, socketPath := setupTestServer(t)
	startTestServer(t, srv)
	defer srv.Stop()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer backend.Close()

	backendPort := portFromAddr(t, backend.Listener.Addr().String())

	// Route then unroute
	data, _ := json.Marshal(RouteRequest{Service: "api", Port: backendPort, Label: "test"})
	sendRequest(t, socketPath, Request{Action: ActionRoute, Data: data})

	undata, _ := json.Marshal(UnrouteRequest{Service: "api"})
	resp := sendRequest(t, socketPath, Request{Action: ActionUnroute, Data: undata})
	if !resp.Success {
		t.Fatalf("unroute failed: %s", resp.Error)
	}
}

func TestServerStatus(t *testing.T) {
	srv, socketPath := setupTestServer(t)
	startTestServer(t, srv)
	defer srv.Stop()

	resp := sendRequest(t, socketPath, Request{Action: ActionStatus})
	if !resp.Success {
		t.Fatalf("status failed: %s", resp.Error)
	}

	var sr StatusResponse
	if err := json.Unmarshal(resp.Data, &sr); err != nil {
		t.Fatal(err)
	}
	if len(sr.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(sr.Entries))
	}
}

func TestServerRouteUnknownService(t *testing.T) {
	srv, socketPath := setupTestServer(t)
	startTestServer(t, srv)
	defer srv.Stop()

	data, _ := json.Marshal(RouteRequest{Service: "nonexistent", Port: 9999})
	resp := sendRequest(t, socketPath, Request{Action: ActionRoute, Data: data})
	if resp.Success {
		t.Error("expected failure for unknown service")
	}
}

func portFromAddr(t *testing.T, addr string) int {
	t.Helper()
	parts := strings.Split(addr, ":")
	var port int
	_, _ = fmt.Sscanf(parts[len(parts)-1], "%d", &port)
	if port == 0 {
		t.Fatalf("could not parse port from %s", addr)
	}
	return port
}

// Verify proxy actually works end-to-end via the daemon
func TestServerRouteE2E(t *testing.T) {
	srv, socketPath := setupTestServer(t)
	startTestServer(t, srv)
	defer srv.Stop()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "e2e-ok")
	}))
	defer backend.Close()

	backendPort := portFromAddr(t, backend.Listener.Addr().String())

	// Route api
	data, _ := json.Marshal(RouteRequest{Service: "api", Port: backendPort, Label: "e2e"})
	resp := sendRequest(t, socketPath, Request{Action: ActionRoute, Data: data})
	if !resp.Success {
		t.Fatalf("route failed: %s", resp.Error)
	}

	// Get listener address from status
	statusResp := sendRequest(t, socketPath, Request{Action: ActionStatus})
	var sr StatusResponse
	_ = json.Unmarshal(statusResp.Data, &sr)

	var apiAddr string
	for _, e := range sr.Entries {
		if e.Service == "api" {
			apiAddr = fmt.Sprintf("127.0.0.1:%d", e.ListenPort)
			break
		}
	}
	if apiAddr == "" {
		t.Fatal("could not find api listener address")
	}

	time.Sleep(50 * time.Millisecond)

	// HTTP request via proxy
	httpResp, err := http.Get("http://" + apiAddr)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer httpResp.Body.Close()

	body, _ := io.ReadAll(httpResp.Body)
	if string(body) != "e2e-ok" {
		t.Errorf("expected 'e2e-ok', got %q", body)
	}
}

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

// --- Phase 2 server tests ---

func TestServerRegister(t *testing.T) {
	srv, socketPath := setupTestServer(t)
	startTestServer(t, srv)
	defer srv.Stop()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "registered")
	}))
	defer backend.Close()

	backendPort := portFromAddr(t, backend.Listener.Addr().String())

	// Get the listener port for "api" from status
	statusResp := sendRequest(t, socketPath, Request{Action: ActionStatus})
	var sr StatusResponse
	_ = json.Unmarshal(statusResp.Data, &sr)
	var listenPort int
	for _, e := range sr.Entries {
		if e.Service == "api" {
			listenPort = e.ListenPort
			break
		}
	}

	data, _ := json.Marshal(RegisterRequest{
		ListenPort:  listenPort,
		BackendPort: backendPort,
		Label:       "worktree-a",
		PID:         12345,
	})
	resp := sendRequest(t, socketPath, Request{Action: ActionRegister, Data: data})
	if !resp.Success {
		t.Fatalf("register failed: %s", resp.Error)
	}

	// Verify via status
	statusResp2 := sendRequest(t, socketPath, Request{Action: ActionStatus})
	var sr2 StatusResponse
	_ = json.Unmarshal(statusResp2.Data, &sr2)
	found := false
	for _, e := range sr2.Entries {
		if e.ListenPort == listenPort {
			for _, b := range e.Backends {
				if b.Port == backendPort && b.Label == "worktree-a" && b.Active {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("registered backend not found in status")
	}
}

func TestServerRegisterMultiple(t *testing.T) {
	srv, socketPath := setupTestServer(t)
	startTestServer(t, srv)
	defer srv.Stop()

	statusResp := sendRequest(t, socketPath, Request{Action: ActionStatus})
	var sr StatusResponse
	_ = json.Unmarshal(statusResp.Data, &sr)
	var listenPort int
	for _, e := range sr.Entries {
		if e.Service == "api" {
			listenPort = e.ListenPort
			break
		}
	}

	// Register two backends
	data1, _ := json.Marshal(RegisterRequest{ListenPort: listenPort, BackendPort: 13001, Label: "a"})
	sendRequest(t, socketPath, Request{Action: ActionRegister, Data: data1})
	data2, _ := json.Marshal(RegisterRequest{ListenPort: listenPort, BackendPort: 13002, Label: "b"})
	sendRequest(t, socketPath, Request{Action: ActionRegister, Data: data2})

	statusResp2 := sendRequest(t, socketPath, Request{Action: ActionStatus})
	var sr2 StatusResponse
	_ = json.Unmarshal(statusResp2.Data, &sr2)
	for _, e := range sr2.Entries {
		if e.ListenPort == listenPort {
			if len(e.Backends) != 2 {
				t.Fatalf("expected 2 backends, got %d", len(e.Backends))
			}
			if !e.Backends[0].Active {
				t.Error("first backend should be active")
			}
			if e.Backends[1].Active {
				t.Error("second backend should be inactive")
			}
			return
		}
	}
	t.Fatal("api listener not found in status")
}

func TestServerUnregister(t *testing.T) {
	srv, socketPath := setupTestServer(t)
	startTestServer(t, srv)
	defer srv.Stop()

	statusResp := sendRequest(t, socketPath, Request{Action: ActionStatus})
	var sr StatusResponse
	_ = json.Unmarshal(statusResp.Data, &sr)
	var listenPort int
	for _, e := range sr.Entries {
		if e.Service == "api" {
			listenPort = e.ListenPort
			break
		}
	}

	data1, _ := json.Marshal(RegisterRequest{ListenPort: listenPort, BackendPort: 13001, Label: "a"})
	sendRequest(t, socketPath, Request{Action: ActionRegister, Data: data1})

	undata, _ := json.Marshal(UnregisterRequest{ListenPort: listenPort, BackendPort: 13001})
	resp := sendRequest(t, socketPath, Request{Action: ActionUnregister, Data: undata})
	if !resp.Success {
		t.Fatalf("unregister failed: %s", resp.Error)
	}

	statusResp2 := sendRequest(t, socketPath, Request{Action: ActionStatus})
	var sr2 StatusResponse
	_ = json.Unmarshal(statusResp2.Data, &sr2)
	for _, e := range sr2.Entries {
		if e.ListenPort == listenPort && len(e.Backends) > 0 {
			t.Error("backend should be removed after unregister")
		}
	}
}

func TestServerSwitch(t *testing.T) {
	srv, socketPath := setupTestServer(t)
	startTestServer(t, srv)
	defer srv.Stop()

	statusResp := sendRequest(t, socketPath, Request{Action: ActionStatus})
	var sr StatusResponse
	_ = json.Unmarshal(statusResp.Data, &sr)
	var listenPort int
	for _, e := range sr.Entries {
		if e.Service == "api" {
			listenPort = e.ListenPort
			break
		}
	}

	data1, _ := json.Marshal(RegisterRequest{ListenPort: listenPort, BackendPort: 13001, Label: "a"})
	sendRequest(t, socketPath, Request{Action: ActionRegister, Data: data1})
	data2, _ := json.Marshal(RegisterRequest{ListenPort: listenPort, BackendPort: 13002, Label: "b"})
	sendRequest(t, socketPath, Request{Action: ActionRegister, Data: data2})

	swdata, _ := json.Marshal(SwitchRequest{ListenPort: listenPort, Label: "b"})
	resp := sendRequest(t, socketPath, Request{Action: ActionSwitch, Data: swdata})
	if !resp.Success {
		t.Fatalf("switch failed: %s", resp.Error)
	}

	statusResp2 := sendRequest(t, socketPath, Request{Action: ActionStatus})
	var sr2 StatusResponse
	_ = json.Unmarshal(statusResp2.Data, &sr2)
	for _, e := range sr2.Entries {
		if e.ListenPort == listenPort {
			for _, b := range e.Backends {
				if b.Label == "b" && !b.Active {
					t.Error("backend 'b' should be active after switch")
				}
				if b.Label == "a" && b.Active {
					t.Error("backend 'a' should be inactive after switch")
				}
			}
			return
		}
	}
	t.Fatal("api listener not found")
}

func TestServerAllocate(t *testing.T) {
	srv, socketPath := setupTestServer(t)
	startTestServer(t, srv)
	defer srv.Stop()

	data, _ := json.Marshal(AllocateRequest{ListenPort: 3000})
	resp := sendRequest(t, socketPath, Request{Action: ActionAllocate, Data: data})
	if !resp.Success {
		t.Fatalf("allocate failed: %s", resp.Error)
	}

	var ar AllocateResponse
	if err := json.Unmarshal(resp.Data, &ar); err != nil {
		t.Fatal(err)
	}
	if ar.BackendPort <= 0 {
		t.Errorf("expected positive port, got %d", ar.BackendPort)
	}
}

func TestServerDynamicListener(t *testing.T) {
	srv, socketPath := setupTestServer(t)
	startTestServer(t, srv)
	defer srv.Stop()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "dynamic")
	}))
	defer backend.Close()

	backendPort := portFromAddr(t, backend.Listener.Addr().String())

	// Register a backend on a port not in config - should create dynamic listener
	data, _ := json.Marshal(RegisterRequest{
		ListenPort:  19999,
		BackendPort: backendPort,
		Label:       "dynamic-test",
	})
	resp := sendRequest(t, socketPath, Request{Action: ActionRegister, Data: data})
	if !resp.Success {
		t.Fatalf("dynamic register failed: %s", resp.Error)
	}

	// Check status includes the dynamic listener
	statusResp := sendRequest(t, socketPath, Request{Action: ActionStatus})
	var sr StatusResponse
	_ = json.Unmarshal(statusResp.Data, &sr)
	found := false
	for _, e := range sr.Entries {
		if e.ListenPort == 19999 {
			found = true
			break
		}
	}
	if !found {
		t.Error("dynamic listener should appear in status")
	}
}

// --- Phase 4: Hot reload server tests ---

func setupTestServerWithConfigPath(t *testing.T) (*Server, string, string) {
	t.Helper()
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "daemon.sock")
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

	srv, err := NewServerWithConfigPath(socketPath, cfgPath, cfg, sm)
	if err != nil {
		t.Fatal(err)
	}
	return srv, socketPath, cfgPath
}

func TestServerHotReloadAddService(t *testing.T) {
	srv, socketPath, cfgPath := setupTestServerWithConfigPath(t)
	startTestServer(t, srv)
	defer srv.Stop()

	// Verify initial: 2 services
	resp := sendRequest(t, socketPath, Request{Action: ActionStatus})
	var sr StatusResponse
	_ = json.Unmarshal(resp.Data, &sr)
	if len(sr.Entries) != 2 {
		t.Fatalf("expected 2 entries initially, got %d", len(sr.Entries))
	}

	// Add a new service via config file
	newCfg := []byte(`services:
  - name: api
    port: 0
  - name: auth
    port: 0
  - name: web
    port: 0
`)
	if err := os.WriteFile(cfgPath, newCfg, 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for watcher to detect (watcher uses 2s interval, but server creates with configPath)
	// We call applyConfigChange directly for testing since the watcher interval in prod is 2s
	newCfgObj, _ := config.LoadConfig(cfgPath)
	srv.applyConfigChange(srv.config, newCfgObj)

	// Verify: 3 services
	resp2 := sendRequest(t, socketPath, Request{Action: ActionStatus})
	var sr2 StatusResponse
	_ = json.Unmarshal(resp2.Data, &sr2)
	if len(sr2.Entries) != 3 {
		t.Fatalf("expected 3 entries after add, got %d", len(sr2.Entries))
	}
	found := false
	for _, e := range sr2.Entries {
		if e.Service == "web" {
			found = true
		}
	}
	if !found {
		t.Error("new 'web' service should appear in status")
	}
}

func TestServerHotReloadRemoveService(t *testing.T) {
	srv, socketPath, cfgPath := setupTestServerWithConfigPath(t)
	startTestServer(t, srv)
	defer srv.Stop()

	// Remove auth service
	newCfg := []byte(`services:
  - name: api
    port: 0
`)
	if err := os.WriteFile(cfgPath, newCfg, 0644); err != nil {
		t.Fatal(err)
	}

	newCfgObj, _ := config.LoadConfig(cfgPath)
	srv.applyConfigChange(srv.config, newCfgObj)

	// Verify: 1 service
	resp := sendRequest(t, socketPath, Request{Action: ActionStatus})
	var sr StatusResponse
	_ = json.Unmarshal(resp.Data, &sr)
	if len(sr.Entries) != 1 {
		t.Fatalf("expected 1 entry after removal, got %d", len(sr.Entries))
	}
	if sr.Entries[0].Service != "api" {
		t.Errorf("remaining service should be 'api', got %q", sr.Entries[0].Service)
	}
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

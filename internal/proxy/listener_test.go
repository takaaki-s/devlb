package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServiceListenerStartStop(t *testing.T) {
	sl := NewServiceListener("test-svc", 0) // ephemeral port
	if err := sl.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer sl.Stop()

	addr := sl.Addr()
	if addr == "" {
		t.Fatal("expected non-empty address")
	}
}

func TestServiceListenerNoBackend(t *testing.T) {
	sl := NewServiceListener("test-svc", 0)
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}
	defer sl.Stop()

	conn, err := net.DialTimeout("tcp", sl.Addr(), time.Second)
	if err != nil {
		// Connection refused is acceptable when no backend
		return
	}
	// If connected, should be closed immediately
	buf := make([]byte, 1)
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, err = conn.Read(buf)
	if err == nil {
		t.Error("expected connection to be closed when no backend is set")
	}
	conn.Close()
}

func TestServiceListenerProxyHTTP(t *testing.T) {
	// Start a test HTTP server as the backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello from backend")
	}))
	defer backend.Close()

	// Extract port from backend address
	backendPort := portFromAddr(t, backend.Listener.Addr().String())

	sl := NewServiceListener("test-svc", 0)
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}
	defer sl.Stop()

	sl.SetBackend(backendPort, "test-label")

	// Give listener a moment to process
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://" + sl.Addr())
	if err != nil {
		t.Fatalf("HTTP request via proxy failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello from backend" {
		t.Errorf("expected 'hello from backend', got %q", body)
	}
}

func TestServiceListenerSwitchBackend(t *testing.T) {
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "backend-1")
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "backend-2")
	}))
	defer backend2.Close()

	sl := NewServiceListener("test-svc", 0)
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}
	defer sl.Stop()

	// Disable keep-alive so each request uses a new connection
	client := &http.Client{
		Transport: &http.Transport{DisableKeepAlives: true},
	}

	// Route to backend1
	sl.SetBackend(portFromAddr(t, backend1.Listener.Addr().String()), "b1")
	time.Sleep(50 * time.Millisecond)

	resp1, err := client.Get("http://" + sl.Addr())
	if err != nil {
		t.Fatal(err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	if string(body1) != "backend-1" {
		t.Errorf("expected 'backend-1', got %q", body1)
	}

	// Switch to backend2
	sl.SetBackend(portFromAddr(t, backend2.Listener.Addr().String()), "b2")
	time.Sleep(50 * time.Millisecond)

	resp2, err := client.Get("http://" + sl.Addr())
	if err != nil {
		t.Fatal(err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if string(body2) != "backend-2" {
		t.Errorf("expected 'backend-2', got %q", body2)
	}
}

func TestServiceListenerClearBackend(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer backend.Close()

	sl := NewServiceListener("test-svc", 0)
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}
	defer sl.Stop()

	sl.SetBackend(portFromAddr(t, backend.Listener.Addr().String()), "test")
	time.Sleep(50 * time.Millisecond)

	// Should work
	resp, err := http.Get("http://" + sl.Addr())
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Clear backend
	sl.ClearBackend()
	time.Sleep(50 * time.Millisecond)

	// New connections should fail
	conn, err := net.DialTimeout("tcp", sl.Addr(), time.Second)
	if err != nil {
		return // Connection refused is acceptable
	}
	buf := make([]byte, 1)
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, err = conn.Read(buf)
	if err == nil {
		t.Error("expected connection to be closed after ClearBackend")
	}
	conn.Close()
}

func TestServiceListenerInfo(t *testing.T) {
	sl := NewServiceListener("api", 3000)
	if err := sl.Start(); err != nil {
		// Port 3000 might be in use; use ephemeral
		sl = NewServiceListener("api", 0)
		if err := sl.Start(); err != nil {
			t.Fatal(err)
		}
	}
	defer sl.Stop()

	info := sl.Info()
	if info.Name != "api" {
		t.Errorf("expected Name=api, got %s", info.Name)
	}
	if !info.Listening {
		t.Error("expected Listening=true")
	}
}

// --- Phase 2: Multi-backend tests ---

func TestAddMultipleBackends(t *testing.T) {
	sl := NewServiceListener("test-svc", 0)
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}
	defer sl.Stop()

	_ = sl.AddBackend(3001, "worktree-a", 0)
	_ = sl.AddBackend(3002, "worktree-b", 0)

	backends := sl.Backends()
	if len(backends) != 2 {
		t.Fatalf("expected 2 backends, got %d", len(backends))
	}
	// First added should be active
	if !backends[0].Active {
		t.Error("first backend should be active")
	}
	if backends[1].Active {
		t.Error("second backend should not be active")
	}
}

func TestOnlyActiveReceivesConnections(t *testing.T) {
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "backend-a")
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "backend-b")
	}))
	defer backend2.Close()

	sl := NewServiceListener("test-svc", 0)
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}
	defer sl.Stop()

	_ = sl.AddBackend(portFromAddr(t, backend1.Listener.Addr().String()), "a", 0)
	_ = sl.AddBackend(portFromAddr(t, backend2.Listener.Addr().String()), "b", 0)
	time.Sleep(50 * time.Millisecond)

	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Get("http://" + sl.Addr())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if string(body) != "backend-a" {
		t.Errorf("expected 'backend-a', got %q", body)
	}
}

func TestSwitchBackendByLabel(t *testing.T) {
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "backend-a")
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "backend-b")
	}))
	defer backend2.Close()

	sl := NewServiceListener("test-svc", 0)
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}
	defer sl.Stop()

	_ = sl.AddBackend(portFromAddr(t, backend1.Listener.Addr().String()), "a", 0)
	_ = sl.AddBackend(portFromAddr(t, backend2.Listener.Addr().String()), "b", 0)
	time.Sleep(50 * time.Millisecond)

	// Switch to "b"
	if err := sl.SwitchBackend("b"); err != nil {
		t.Fatal(err)
	}

	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Get("http://" + sl.Addr())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if string(body) != "backend-b" {
		t.Errorf("expected 'backend-b', got %q", body)
	}
}

func TestRemoveBackend(t *testing.T) {
	sl := NewServiceListener("test-svc", 0)
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}
	defer sl.Stop()

	_ = sl.AddBackend(3001, "a", 0)
	_ = sl.AddBackend(3002, "b", 0)

	removed := sl.RemoveBackend(3001)
	if !removed {
		t.Error("RemoveBackend should return true for existing backend")
	}

	backends := sl.Backends()
	if len(backends) != 1 {
		t.Fatalf("expected 1 backend after removal, got %d", len(backends))
	}
	if backends[0].Port != 3002 {
		t.Errorf("remaining backend should be port 3002, got %d", backends[0].Port)
	}
}

func TestRemoveActivePromotesNext(t *testing.T) {
	sl := NewServiceListener("test-svc", 0)
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}
	defer sl.Stop()

	_ = sl.AddBackend(3001, "a", 0)
	_ = sl.AddBackend(3002, "b", 0)

	// Remove active backend
	sl.RemoveBackend(3001)

	backends := sl.Backends()
	if len(backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(backends))
	}
	if !backends[0].Active {
		t.Error("remaining backend should be promoted to active")
	}
}

func TestSwitchUnknownLabelError(t *testing.T) {
	sl := NewServiceListener("test-svc", 0)
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}
	defer sl.Stop()

	_ = sl.AddBackend(3001, "a", 0)

	err := sl.SwitchBackend("nonexistent")
	if err == nil {
		t.Error("SwitchBackend with unknown label should return error")
	}
}

func TestStartPortConflictBlocked(t *testing.T) {
	// Start a listener on a port
	sl1 := NewServiceListener("first", 0)
	if err := sl1.Start(); err != nil {
		t.Fatal(err)
	}
	defer sl1.Stop()

	port := portFromAddr(t, sl1.Addr())

	// Try to start a second listener on the same port
	sl2 := NewServiceListener("second", port)
	err := sl2.Start()
	if err == nil {
		sl2.Stop()
		t.Fatal("expected Start to fail on occupied port")
	}

	if !sl2.IsBlocked() {
		t.Error("expected listener to be blocked")
	}

	info := sl2.Info()
	if !info.Blocked {
		t.Error("expected Info().Blocked to be true")
	}
}

func TestHandleConnBackendDown(t *testing.T) {
	sl := NewServiceListener("test-svc", 0)
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}
	defer sl.Stop()

	// Set backend to a port where nothing is listening
	sl.SetBackend(59999, "dead-backend")
	time.Sleep(50 * time.Millisecond)

	// Connect to proxy — should be closed promptly
	conn, err := net.DialTimeout("tcp", sl.Addr(), time.Second)
	if err != nil {
		return // Connection refused is also acceptable
	}
	defer conn.Close()

	buf := make([]byte, 1)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Read(buf)
	if err == nil {
		t.Error("expected connection to be closed when backend is down")
	}
}

func TestStopGracefulDrain(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a slow response
		time.Sleep(500 * time.Millisecond)
		fmt.Fprint(w, "done")
	}))
	defer backend.Close()

	sl := NewServiceListener("test-svc", 0)
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}

	sl.SetBackend(portFromAddr(t, backend.Listener.Addr().String()), "slow")
	time.Sleep(50 * time.Millisecond)

	// Start a slow request in background
	done := make(chan string, 1)
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	go func() {
		resp, err := client.Get("http://" + sl.Addr())
		if err != nil {
			done <- "error"
			return
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		done <- string(body)
	}()

	// Wait for connection to be established
	time.Sleep(100 * time.Millisecond)

	// Graceful stop with 2s timeout — should wait for the slow request
	sl.StopGraceful(2 * time.Second)

	result := <-done
	if result != "done" {
		t.Errorf("expected request to complete during drain, got %q", result)
	}
}

func TestStopGracefulTimeout(t *testing.T) {
	// Create a backend that never responds
	backendLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backendLn.Close()

	// Accept but never respond
	go func() {
		for {
			conn, err := backendLn.Accept()
			if err != nil {
				return
			}
			// Hold connection open, never write
			_ = conn
		}
	}()

	sl := NewServiceListener("test-svc", 0)
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}

	sl.SetBackend(portFromAddr(t, backendLn.Addr().String()), "hanging")
	time.Sleep(50 * time.Millisecond)

	// Connect a client
	conn, err := net.DialTimeout("tcp", sl.Addr(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Write some data to ensure Bridge is established
	_, _ = conn.Write([]byte("GET / HTTP/1.0\r\n\r\n"))
	time.Sleep(100 * time.Millisecond)

	// Graceful stop with short timeout
	start := time.Now()
	sl.StopGraceful(200 * time.Millisecond)
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("StopGraceful took %v, expected ~200ms timeout", elapsed)
	}
}

func TestStopGracefulNoConnections(t *testing.T) {
	sl := NewServiceListener("test-svc", 0)
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	sl.StopGraceful(5 * time.Second)
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("StopGraceful with no connections took %v, expected immediate return", elapsed)
	}
}

func TestHandleConnBackendRecovers(t *testing.T) {
	sl := NewServiceListener("test-svc", 0)
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}
	defer sl.Stop()

	// Allocate a port for backend
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	backendPort := portFromAddr(t, ln.Addr().String())
	ln.Close() // Close so the port is free but nothing listens

	sl.SetBackend(backendPort, "recovering")
	time.Sleep(50 * time.Millisecond)

	// First connection should fail (backend not listening)
	conn1, err := net.DialTimeout("tcp", sl.Addr(), time.Second)
	if err == nil {
		buf := make([]byte, 1)
		_ = conn1.SetReadDeadline(time.Now().Add(time.Second))
		_, _ = conn1.Read(buf)
		conn1.Close()
	}

	// Start a real backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "recovered")
	}))
	defer backend.Close()

	sl.SetBackend(portFromAddr(t, backend.Listener.Addr().String()), "alive")
	time.Sleep(50 * time.Millisecond)

	// Second connection should succeed
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Get("http://" + sl.Addr())
	if err != nil {
		t.Fatalf("expected recovery to work: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "recovered" {
		t.Errorf("expected 'recovered', got %q", body)
	}
}

// --- Phase 4: Health check integration tests ---

func TestHealthCheckFailoverToHealthyBackend(t *testing.T) {
	// backend1 is dead (port allocated but not listening)
	deadLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadPort := portFromAddr(t, deadLn.Addr().String())
	deadLn.Close()

	// backend2 is alive
	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "healthy-backend")
	}))
	defer backend2.Close()
	alivePort := portFromAddr(t, backend2.Listener.Addr().String())

	sl := NewServiceListener("test-svc", 0)
	sl.SetHealthChecker(NewHealthChecker(HealthConfig{
		Interval:       50 * time.Millisecond,
		Timeout:        50 * time.Millisecond,
		UnhealthyAfter: 2,
	}))
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}
	defer sl.Stop()

	// Add dead backend as active, alive backend as standby
	_ = sl.AddBackend(deadPort, "dead", 0)
	_ = sl.AddBackend(alivePort, "alive", 0)

	// Wait for health checks to detect dead backend
	time.Sleep(300 * time.Millisecond)

	// Request should be routed to the healthy backend
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Get("http://" + sl.Addr())
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if string(body) != "healthy-backend" {
		t.Errorf("expected 'healthy-backend', got %q", body)
	}
}

func TestHealthCheckAllUnhealthyReturnsZeroPort(t *testing.T) {
	// Both backends are dead
	ln1, _ := net.Listen("tcp", "127.0.0.1:0")
	port1 := portFromAddr(t, ln1.Addr().String())
	ln1.Close()

	ln2, _ := net.Listen("tcp", "127.0.0.1:0")
	port2 := portFromAddr(t, ln2.Addr().String())
	ln2.Close()

	sl := NewServiceListener("test-svc", 0)
	sl.SetHealthChecker(NewHealthChecker(HealthConfig{
		Interval:       50 * time.Millisecond,
		Timeout:        50 * time.Millisecond,
		UnhealthyAfter: 2,
	}))
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}
	defer sl.Stop()

	_ = sl.AddBackend(port1, "dead1", 0)
	_ = sl.AddBackend(port2, "dead2", 0)

	// Wait for health checks
	time.Sleep(300 * time.Millisecond)

	// Connection should be closed (no healthy backend)
	conn, err := net.DialTimeout("tcp", sl.Addr(), time.Second)
	if err != nil {
		return // connection refused is acceptable
	}
	buf := make([]byte, 1)
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, err = conn.Read(buf)
	if err == nil {
		t.Error("expected connection to be closed when all backends are unhealthy")
	}
	conn.Close()
}

// --- Phase 4: HTTP 503 integration tests ---

func TestNoBackendHTTPGets503(t *testing.T) {
	sl := NewServiceListener("test-svc", 0)
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}
	defer sl.Stop()

	// No backend set — HTTP request should get 503
	time.Sleep(50 * time.Millisecond)
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Get("http://" + sl.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 503 {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "503") {
		t.Errorf("expected 503 in body, got: %s", body)
	}
}

func TestDeadBackendHTTPGets503(t *testing.T) {
	sl := NewServiceListener("test-svc", 0)
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}
	defer sl.Stop()

	// Set backend to a port where nothing is listening
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadPort := portFromAddr(t, ln.Addr().String())
	ln.Close()

	sl.SetBackend(deadPort, "dead")
	time.Sleep(50 * time.Millisecond)

	client := &http.Client{
		Transport: &http.Transport{DisableKeepAlives: true},
		Timeout:   5 * time.Second,
	}
	resp, err := client.Get("http://" + sl.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 503 {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
}

func TestMetricsAfterProxy(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "metrics-test")
	}))
	defer backend.Close()
	backendPort := portFromAddr(t, backend.Listener.Addr().String())

	sl := NewServiceListener("test-svc", 0)
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}
	defer sl.Stop()

	sl.SetBackend(backendPort, "test")
	time.Sleep(50 * time.Millisecond)

	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Get("http://" + sl.Addr())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()

	// Give time for connection to close and metrics to be recorded
	time.Sleep(100 * time.Millisecond)

	m := sl.Metrics().Get(backendPort)
	if m.TotalConns != 1 {
		t.Errorf("TotalConns = %d, want 1", m.TotalConns)
	}
	if m.ActiveConns != 0 {
		t.Errorf("ActiveConns = %d, want 0", m.ActiveConns)
	}
	if m.BytesIn == 0 {
		t.Error("BytesIn should be > 0")
	}
	if m.BytesOut == 0 {
		t.Error("BytesOut should be > 0")
	}
}

func TestNoHealthCheckerUsesActivePortDirectly(t *testing.T) {
	// Without a health checker, behavior is unchanged
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "no-hc")
	}))
	defer backend.Close()

	sl := NewServiceListener("test-svc", 0)
	// No SetHealthChecker call
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}
	defer sl.Stop()

	sl.SetBackend(portFromAddr(t, backend.Listener.Addr().String()), "test")
	time.Sleep(50 * time.Millisecond)

	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Get("http://" + sl.Addr())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "no-hc" {
		t.Errorf("expected 'no-hc', got %q", body)
	}
}

func TestAddBackendDuplicate(t *testing.T) {
	sl := NewServiceListener("test-svc", 0)
	if err := sl.Start(); err != nil {
		t.Fatal(err)
	}
	defer sl.Stop()

	if err := sl.AddBackend(3001, "first", 0); err != nil {
		t.Fatalf("first AddBackend should succeed: %v", err)
	}

	// Same port should be rejected
	err := sl.AddBackend(3001, "second", 0)
	if err == nil {
		t.Fatal("expected error for duplicate backend port, got nil")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("expected 'already registered' error, got: %v", err)
	}

	// Different port should succeed
	if err := sl.AddBackend(3002, "third", 0); err != nil {
		t.Fatalf("different port should succeed: %v", err)
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

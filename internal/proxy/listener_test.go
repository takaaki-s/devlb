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

package proxy

import (
	"fmt"
	"net"
	"testing"
	"time"
)

func TestDefaultHealthConfig(t *testing.T) {
	cfg := DefaultHealthConfig()
	if cfg.Interval != 5*time.Second {
		t.Errorf("Interval = %v, want 5s", cfg.Interval)
	}
	if cfg.Timeout != 1*time.Second {
		t.Errorf("Timeout = %v, want 1s", cfg.Timeout)
	}
	if cfg.UnhealthyAfter != 3 {
		t.Errorf("UnhealthyAfter = %d, want 3", cfg.UnhealthyAfter)
	}
}

func TestHealthCheckerHealthyBackend(t *testing.T) {
	// Start a TCP listener as a healthy backend
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := portFromAddr(t, ln.Addr().String())

	hc := NewHealthChecker(HealthConfig{
		Interval:       100 * time.Millisecond,
		Timeout:        500 * time.Millisecond,
		UnhealthyAfter: 3,
	})
	hc.AddBackend(port)
	hc.Start()
	defer hc.Stop()

	// Wait for at least one check
	time.Sleep(200 * time.Millisecond)

	if !hc.IsHealthy(port) {
		t.Error("expected healthy backend to be healthy")
	}
}

func TestHealthCheckerUnhealthyBackend(t *testing.T) {
	// Use a port that nothing listens on
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := portFromAddr(t, ln.Addr().String())
	ln.Close() // close immediately so port is unresponsive

	hc := NewHealthChecker(HealthConfig{
		Interval:       50 * time.Millisecond,
		Timeout:        50 * time.Millisecond,
		UnhealthyAfter: 3,
	})
	hc.AddBackend(port)
	hc.Start()
	defer hc.Stop()

	// Wait for 3+ check intervals
	time.Sleep(300 * time.Millisecond)

	if hc.IsHealthy(port) {
		t.Error("expected unhealthy backend to be unhealthy")
	}

	status := hc.Status(port)
	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if status.ConsecFails < 3 {
		t.Errorf("ConsecFails = %d, want >= 3", status.ConsecFails)
	}
}

func TestHealthCheckerRecovery(t *testing.T) {
	// Start with dead backend
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := portFromAddr(t, ln.Addr().String())
	ln.Close()

	hc := NewHealthChecker(HealthConfig{
		Interval:       50 * time.Millisecond,
		Timeout:        50 * time.Millisecond,
		UnhealthyAfter: 2,
	})
	hc.AddBackend(port)
	hc.Start()
	defer hc.Stop()

	// Wait for unhealthy
	time.Sleep(200 * time.Millisecond)
	if hc.IsHealthy(port) {
		t.Error("expected backend to be unhealthy")
	}

	// Bring backend back
	ln2, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Skipf("could not rebind port %d: %v", port, err)
	}
	defer ln2.Close()

	// Wait for recovery
	time.Sleep(200 * time.Millisecond)
	if !hc.IsHealthy(port) {
		t.Error("expected backend to recover to healthy")
	}
}

func TestHealthCheckerRemoveBackend(t *testing.T) {
	hc := NewHealthChecker(HealthConfig{
		Interval:       100 * time.Millisecond,
		Timeout:        50 * time.Millisecond,
		UnhealthyAfter: 3,
	})
	hc.AddBackend(9999)
	hc.RemoveBackend(9999)

	// Should not panic, and unknown port returns true (healthy by default)
	if !hc.IsHealthy(9999) {
		t.Error("removed backend should default to healthy")
	}
}

func TestHealthCheckerUnknownPortIsHealthy(t *testing.T) {
	hc := NewHealthChecker(DefaultHealthConfig())
	if !hc.IsHealthy(12345) {
		t.Error("unknown port should be considered healthy")
	}
}

package proxy

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// HealthConfig holds health check configuration.
type HealthConfig struct {
	Interval       time.Duration
	Timeout        time.Duration
	UnhealthyAfter int
}

// DefaultHealthConfig returns sensible defaults.
func DefaultHealthConfig() HealthConfig {
	return HealthConfig{
		Interval:       5 * time.Second,
		Timeout:        1 * time.Second,
		UnhealthyAfter: 3,
	}
}

// BackendHealth tracks health state for a single backend.
type BackendHealth struct {
	Healthy     bool
	ConsecFails int
	LastCheck   time.Time
	LastError   string
}

// HealthChecker performs periodic TCP health checks on backends.
type HealthChecker struct {
	config   HealthConfig
	mu       sync.RWMutex
	backends map[int]*BackendHealth
	done     chan struct{}
}

// NewHealthChecker creates a new health checker with the given config.
func NewHealthChecker(cfg HealthConfig) *HealthChecker {
	return &HealthChecker{
		config:   cfg,
		backends: make(map[int]*BackendHealth),
		done:     make(chan struct{}),
	}
}

// AddBackend registers a backend port for health checking.
func (hc *HealthChecker) AddBackend(port int) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	if _, ok := hc.backends[port]; !ok {
		hc.backends[port] = &BackendHealth{Healthy: true}
	}
}

// RemoveBackend unregisters a backend port from health checking.
func (hc *HealthChecker) RemoveBackend(port int) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	delete(hc.backends, port)
}

// Start begins periodic health checking.
func (hc *HealthChecker) Start() {
	go hc.loop()
}

// Stop stops the health checker.
func (hc *HealthChecker) Stop() {
	select {
	case <-hc.done:
		return
	default:
		close(hc.done)
	}
}

// IsHealthy returns whether the given port is healthy.
// Unknown ports are considered healthy.
func (hc *HealthChecker) IsHealthy(port int) bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	bh, ok := hc.backends[port]
	if !ok {
		return true
	}
	return bh.Healthy
}

// Status returns a copy of the health status for a port. Returns nil if unknown.
func (hc *HealthChecker) Status(port int) *BackendHealth {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	bh, ok := hc.backends[port]
	if !ok {
		return nil
	}
	cp := *bh
	return &cp
}

// AllStatuses returns a copy of all health statuses.
func (hc *HealthChecker) AllStatuses() map[int]BackendHealth {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	result := make(map[int]BackendHealth, len(hc.backends))
	for port, bh := range hc.backends {
		result[port] = *bh
	}
	return result
}

func (hc *HealthChecker) loop() {
	ticker := time.NewTicker(hc.config.Interval)
	defer ticker.Stop()

	// Run an immediate first check
	hc.checkAll()

	for {
		select {
		case <-hc.done:
			return
		case <-ticker.C:
			hc.checkAll()
		}
	}
}

func (hc *HealthChecker) checkAll() {
	hc.mu.RLock()
	ports := make([]int, 0, len(hc.backends))
	for port := range hc.backends {
		ports = append(ports, port)
	}
	hc.mu.RUnlock()

	for _, port := range ports {
		healthy, errMsg := hc.checkBackend(port)

		hc.mu.Lock()
		bh, ok := hc.backends[port]
		if !ok {
			hc.mu.Unlock()
			continue
		}
		bh.LastCheck = time.Now()
		if healthy {
			bh.ConsecFails = 0
			bh.Healthy = true
			bh.LastError = ""
		} else {
			bh.ConsecFails++
			bh.LastError = errMsg
			if bh.ConsecFails >= hc.config.UnhealthyAfter {
				bh.Healthy = false
			}
		}
		hc.mu.Unlock()
	}
}

func (hc *HealthChecker) checkBackend(port int) (bool, string) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", addr, hc.config.Timeout)
	if err != nil {
		return false, err.Error()
	}
	conn.Close()
	return true, ""
}

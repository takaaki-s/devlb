package proxy

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// BackendEntry represents a registered backend for a service listener.
type BackendEntry struct {
	Port    int
	Label   string
	Active  bool
	PID     int
	LogFile string
}

// ListenerInfo holds the current state of a ServiceListener.
type ListenerInfo struct {
	Name        string
	ListenAddr  string
	BackendPort int
	Label       string
	Listening   bool
	ActiveConns int64
	Blocked     bool
	BlockedBy   string // e.g., "PID 12345 (node)"
}

// ServiceListener manages a TCP listener for a single service.
type ServiceListener struct {
	name       string
	listenPort int
	listener   net.Listener

	mu       sync.RWMutex
	backends []BackendEntry

	activeConns atomic.Int64
	done        chan struct{}

	blocked   bool
	blockInfo *PortOwner

	connsMu  sync.Mutex
	connsSet map[net.Conn]struct{}

	healthChecker *HealthChecker
	metrics       *MetricsStore
}

func NewServiceListener(name string, listenPort int) *ServiceListener {
	return &ServiceListener{
		name:       name,
		listenPort: listenPort,
		done:       make(chan struct{}),
		connsSet:   make(map[net.Conn]struct{}),
		metrics:    NewMetricsStore(),
	}
}

// SetHealthChecker sets the health checker for this listener.
// Must be called before Start().
func (sl *ServiceListener) SetHealthChecker(hc *HealthChecker) {
	sl.healthChecker = hc
}

// HealthChecker returns the health checker, or nil if not set.
func (sl *ServiceListener) GetHealthChecker() *HealthChecker {
	return sl.healthChecker
}

func (sl *ServiceListener) Start() error {
	addr := fmt.Sprintf("127.0.0.1:%d", sl.listenPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		if isAddrInUse(err) {
			sl.mu.Lock()
			sl.blocked = true
			sl.blockInfo = FindPortOwner(sl.listenPort)
			sl.mu.Unlock()
		}
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	sl.mu.Lock()
	sl.listener = ln
	sl.mu.Unlock()

	if sl.healthChecker != nil {
		sl.healthChecker.Start()
	}

	go sl.acceptLoop()
	return nil
}

// IsBlocked returns true if the listener failed to start due to port conflict.
func (sl *ServiceListener) IsBlocked() bool {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	return sl.blocked
}

func (sl *ServiceListener) Stop() {
	select {
	case <-sl.done:
		return // already stopped
	default:
		close(sl.done)
	}
	if sl.healthChecker != nil {
		sl.healthChecker.Stop()
	}
	sl.mu.RLock()
	ln := sl.listener
	sl.mu.RUnlock()
	if ln != nil {
		ln.Close()
	}
}

// StopGraceful stops accepting new connections and waits for active connections
// to drain, or force-closes them after timeout.
func (sl *ServiceListener) StopGraceful(timeout time.Duration) {
	select {
	case <-sl.done:
		return
	default:
		close(sl.done)
	}

	if sl.healthChecker != nil {
		sl.healthChecker.Stop()
	}
	sl.mu.RLock()
	ln := sl.listener
	sl.mu.RUnlock()
	if ln != nil {
		ln.Close()
	}

	deadline := time.After(timeout)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		if sl.activeConns.Load() == 0 {
			return
		}
		select {
		case <-deadline:
			slog.Warn("drain timeout, force-closing connections", "service", sl.name, "active_conns", sl.activeConns.Load())
			sl.connsMu.Lock()
			for conn := range sl.connsSet {
				conn.Close()
			}
			sl.connsMu.Unlock()
			return
		case <-ticker.C:
			continue
		}
	}
}

func (sl *ServiceListener) Addr() string {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	return sl.addrLocked()
}

// addrLocked returns the listen address. Caller must hold mu (read or write).
func (sl *ServiceListener) addrLocked() string {
	if sl.listener == nil {
		return ""
	}
	return sl.listener.Addr().String()
}

// SetBackend sets a single backend, clearing all existing backends.
// Maintains Phase 1 API compatibility.
func (sl *ServiceListener) SetBackend(port int, label string) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.backends = []BackendEntry{{Port: port, Label: label, Active: true}}
}

// ClearBackend removes all backends.
// Maintains Phase 1 API compatibility.
func (sl *ServiceListener) ClearBackend() {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.backends = nil
}

// AddBackend registers a new backend. The first backend added becomes active automatically.
func (sl *ServiceListener) AddBackend(port int, label string, pid int, logFile string) error {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	for _, b := range sl.backends {
		if b.Port == port {
			return fmt.Errorf("backend :%d already registered", port)
		}
	}

	active := len(sl.backends) == 0 // first backend is active
	sl.backends = append(sl.backends, BackendEntry{
		Port:    port,
		Label:   label,
		Active:  active,
		PID:     pid,
		LogFile: logFile,
	})

	if sl.healthChecker != nil {
		sl.healthChecker.AddBackend(port)
	}
	return nil
}

// RemoveBackend removes the backend with the given port.
// If the removed backend was active, the next available backend is promoted.
func (sl *ServiceListener) RemoveBackend(port int) bool {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	idx := -1
	for i, b := range sl.backends {
		if b.Port == port {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false
	}

	wasActive := sl.backends[idx].Active
	removedPort := sl.backends[idx].Port
	sl.backends = append(sl.backends[:idx], sl.backends[idx+1:]...)

	// Promote next backend if the active one was removed
	if wasActive && len(sl.backends) > 0 {
		sl.backends[0].Active = true
	}

	if sl.healthChecker != nil {
		sl.healthChecker.RemoveBackend(removedPort)
	}

	return true
}

// SwitchBackend changes the active backend to the one with the given label.
func (sl *ServiceListener) SwitchBackend(label string) error {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	found := false
	for i := range sl.backends {
		if sl.backends[i].Label == label {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("backend with label %q not found", label)
	}

	for i := range sl.backends {
		sl.backends[i].Active = (sl.backends[i].Label == label)
	}
	return nil
}

// Backends returns a copy of the current backend list.
func (sl *ServiceListener) Backends() []BackendEntry {
	sl.mu.RLock()
	defer sl.mu.RUnlock()

	result := make([]BackendEntry, len(sl.backends))
	copy(result, sl.backends)
	return result
}

// activePort returns the port of the active backend. Must be called with mu held.
func (sl *ServiceListener) activePort() int {
	for _, b := range sl.backends {
		if b.Active {
			return b.Port
		}
	}
	return 0
}

// healthyActivePort returns the active backend's port if healthy, or the first healthy
// standby backend's port. Returns 0 if no healthy backend is available.
// Must be called with mu held (read lock is sufficient).
func (sl *ServiceListener) healthyActivePort() int {
	if sl.healthChecker == nil {
		return sl.activePort()
	}

	active := sl.activePort()
	if active != 0 && sl.healthChecker.IsHealthy(active) {
		return active
	}

	// Failover: find first healthy backend
	for _, b := range sl.backends {
		if b.Port != active && sl.healthChecker.IsHealthy(b.Port) {
			return b.Port
		}
	}
	return 0
}

// Info returns a snapshot of the listener's current state.
func (sl *ServiceListener) Info() ListenerInfo {
	sl.mu.RLock()
	defer sl.mu.RUnlock()

	var port int
	var label string
	for _, b := range sl.backends {
		if b.Active {
			port = b.Port
			label = b.Label
			break
		}
	}

	blockedBy := ""
	if sl.blocked && sl.blockInfo != nil {
		blockedBy = fmt.Sprintf("PID %d (%s)", sl.blockInfo.PID, sl.blockInfo.Command)
	}

	return ListenerInfo{
		Name:        sl.name,
		ListenAddr:  sl.addrLocked(),
		BackendPort: port,
		Label:       label,
		Listening:   sl.listener != nil,
		ActiveConns: sl.activeConns.Load(),
		Blocked:     sl.blocked,
		BlockedBy:   blockedBy,
	}
}

// Metrics returns the metrics store for this listener.
func (sl *ServiceListener) Metrics() *MetricsStore {
	return sl.metrics
}

func (sl *ServiceListener) acceptLoop() {
	for {
		conn, err := sl.listener.Accept()
		if err != nil {
			select {
			case <-sl.done:
				return
			default:
				slog.Error("accept error", "service", sl.name, "error", err)
				return
			}
		}
		go sl.handleConn(conn)
	}
}

func (sl *ServiceListener) handleConn(client net.Conn) {
	sl.mu.RLock()
	port := sl.healthyActivePort()
	sl.mu.RUnlock()

	if port == 0 {
		PeekAndRespond503(client, sl.name, sl.listenPort)
		client.Close()
		return
	}

	backend, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 3*time.Second)
	if err != nil {
		slog.Warn("backend connect failed", "service", sl.name, "backend_port", port, "error", err)
		if !PeekAndRespond503(client, sl.name, sl.listenPort) {
			// Non-HTTP: RST close so client sees connection refused
			if tc, ok := client.(*net.TCPConn); ok {
				_ = tc.SetLinger(0)
			}
		}
		client.Close()
		return
	}

	sl.activeConns.Add(1)
	sl.metrics.RecordConnect(port)
	sl.connsMu.Lock()
	sl.connsSet[client] = struct{}{}
	sl.connsSet[backend] = struct{}{}
	sl.connsMu.Unlock()

	// Wrap connections for byte counting
	clientCC := NewCountingConn(client)
	backendCC := NewCountingConn(backend)

	defer func() {
		sl.connsMu.Lock()
		delete(sl.connsSet, client)
		delete(sl.connsSet, backend)
		sl.connsMu.Unlock()
		sl.metrics.AddBytesIn(port, clientCC.BytesRead())
		sl.metrics.AddBytesOut(port, backendCC.BytesRead())
		sl.metrics.RecordDisconnect(port)
		sl.activeConns.Add(-1)
	}()

	Bridge(clientCC, backendCC)
}

func isAddrInUse(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var sysErr *os.SyscallError
		if errors.As(opErr.Err, &sysErr) {
			return errors.Is(sysErr.Err, syscall.EADDRINUSE)
		}
	}
	return false
}

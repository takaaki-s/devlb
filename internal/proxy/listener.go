package proxy

import (
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
)

// BackendEntry represents a registered backend for a service listener.
type BackendEntry struct {
	Port   int
	Label  string
	Active bool
	PID    int
}

// ListenerInfo holds the current state of a ServiceListener.
type ListenerInfo struct {
	Name        string
	ListenAddr  string
	BackendPort int
	Label       string
	Listening   bool
	ActiveConns int64
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
}

func NewServiceListener(name string, listenPort int) *ServiceListener {
	return &ServiceListener{
		name:       name,
		listenPort: listenPort,
		done:       make(chan struct{}),
	}
}

func (sl *ServiceListener) Start() error {
	addr := fmt.Sprintf("127.0.0.1:%d", sl.listenPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	sl.listener = ln

	go sl.acceptLoop()
	return nil
}

func (sl *ServiceListener) Stop() {
	select {
	case <-sl.done:
		return // already stopped
	default:
		close(sl.done)
	}
	if sl.listener != nil {
		sl.listener.Close()
	}
}

func (sl *ServiceListener) Addr() string {
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
func (sl *ServiceListener) AddBackend(port int, label string, pid int) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	active := len(sl.backends) == 0 // first backend is active
	sl.backends = append(sl.backends, BackendEntry{
		Port:   port,
		Label:  label,
		Active: active,
		PID:    pid,
	})
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
	sl.backends = append(sl.backends[:idx], sl.backends[idx+1:]...)

	// Promote next backend if the active one was removed
	if wasActive && len(sl.backends) > 0 {
		sl.backends[0].Active = true
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

	return ListenerInfo{
		Name:        sl.name,
		ListenAddr:  sl.Addr(),
		BackendPort: port,
		Label:       label,
		Listening:   sl.listener != nil,
		ActiveConns: sl.activeConns.Load(),
	}
}

func (sl *ServiceListener) acceptLoop() {
	for {
		conn, err := sl.listener.Accept()
		if err != nil {
			select {
			case <-sl.done:
				return
			default:
				log.Printf("[%s] accept error: %v", sl.name, err)
				return
			}
		}
		go sl.handleConn(conn)
	}
}

func (sl *ServiceListener) handleConn(client net.Conn) {
	sl.mu.RLock()
	port := sl.activePort()
	sl.mu.RUnlock()

	if port == 0 {
		client.Close()
		return
	}

	backend, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		log.Printf("[%s] backend connect failed: %v", sl.name, err)
		client.Close()
		return
	}

	sl.activeConns.Add(1)
	defer sl.activeConns.Add(-1)

	Bridge(client, backend)
}

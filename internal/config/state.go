package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// BackendEntry represents a backend for a listen port (Phase 2).
type BackendEntry struct {
	BackendPort int    `yaml:"backend_port"`
	Label       string `yaml:"label,omitempty"`
	Active      bool   `yaml:"active"`
	PID         int    `yaml:"pid,omitempty"`
}

// RouteEntry represents a single route (Phase 1 legacy).
type RouteEntry struct {
	BackendPort int    `yaml:"backend_port"`
	Label       string `yaml:"label,omitempty"`
	Active      bool   `yaml:"active"`
}

// State holds the persistent routing state.
type State struct {
	Routes   map[string]*RouteEntry    `yaml:"routes,omitempty"`   // Phase 1 legacy
	Backends map[int][]*BackendEntry   `yaml:"backends,omitempty"` // Phase 2
}

type StateManager struct {
	mu       sync.RWMutex
	state    *State
	filePath string
}

func NewStateManager(dataDir string) (*StateManager, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	m := &StateManager{
		filePath: filepath.Join(dataDir, "state.yaml"),
		state: &State{
			Routes:   make(map[string]*RouteEntry),
			Backends: make(map[int][]*BackendEntry),
		},
	}

	if err := m.load(); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return m, nil
}

func (m *StateManager) load() error {
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return err
	}

	state := &State{}
	if err := yaml.Unmarshal(data, state); err != nil {
		return err
	}
	if state.Routes == nil {
		state.Routes = make(map[string]*RouteEntry)
	}
	if state.Backends == nil {
		state.Backends = make(map[int][]*BackendEntry)
	}
	m.state = state
	return nil
}

func (m *StateManager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, err := yaml.Marshal(m.state)
	if err != nil {
		return err
	}
	return os.WriteFile(m.filePath, data, 0644)
}

func (m *StateManager) FilePath() string {
	return m.filePath
}

// --- Phase 1 legacy methods (kept for server.go compat, removed in Step 5) ---

func (m *StateManager) SetRoute(name string, backendPort int, label string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.state.Routes[name] = &RouteEntry{
		BackendPort: backendPort,
		Label:       label,
		Active:      true,
	}
}

func (m *StateManager) GetRoute(name string) (*RouteEntry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	r, ok := m.state.Routes[name]
	return r, ok
}

func (m *StateManager) DeleteRoute(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.state.Routes, name)
}

func (m *StateManager) GetAllRoutes() map[string]*RouteEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*RouteEntry, len(m.state.Routes))
	for k, v := range m.state.Routes {
		copied := *v
		result[k] = &copied
	}
	return result
}

// --- Phase 2 multi-backend methods ---

// AddBackend registers a backend for a listen port. The first backend becomes active.
func (m *StateManager) AddBackend(listenPort, backendPort int, label string, pid int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	active := len(m.state.Backends[listenPort]) == 0
	m.state.Backends[listenPort] = append(m.state.Backends[listenPort], &BackendEntry{
		BackendPort: backendPort,
		Label:       label,
		Active:      active,
		PID:         pid,
	})
}

// RemoveBackend removes a backend by its port. Promotes next if active was removed.
func (m *StateManager) RemoveBackend(listenPort, backendPort int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	backends := m.state.Backends[listenPort]
	idx := -1
	for i, b := range backends {
		if b.BackendPort == backendPort {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}

	wasActive := backends[idx].Active
	backends = append(backends[:idx], backends[idx+1:]...)

	if wasActive && len(backends) > 0 {
		backends[0].Active = true
	}

	if len(backends) == 0 {
		delete(m.state.Backends, listenPort)
	} else {
		m.state.Backends[listenPort] = backends
	}
}

// SwitchActive changes the active backend for a listen port.
func (m *StateManager) SwitchActive(listenPort int, label string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	backends := m.state.Backends[listenPort]
	found := false
	for _, b := range backends {
		if b.Label == label {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("backend with label %q not found for port %d", label, listenPort)
	}

	for _, b := range backends {
		b.Active = (b.Label == label)
	}
	return nil
}

// GetBackends returns a copy of backends for a listen port.
func (m *StateManager) GetBackends(listenPort int) []*BackendEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	orig := m.state.Backends[listenPort]
	result := make([]*BackendEntry, len(orig))
	for i, b := range orig {
		copied := *b
		result[i] = &copied
	}
	return result
}

// GetAllBackends returns a copy of all backends.
func (m *StateManager) GetAllBackends() map[int][]*BackendEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[int][]*BackendEntry, len(m.state.Backends))
	for port, backends := range m.state.Backends {
		copied := make([]*BackendEntry, len(backends))
		for i, b := range backends {
			c := *b
			copied[i] = &c
		}
		result[port] = copied
	}
	return result
}

// AllocatePort finds an available ephemeral port by temporarily binding to :0.
func (m *StateManager) AllocatePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("allocate port: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port, nil
}

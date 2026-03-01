package config

import (
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

type RouteEntry struct {
	BackendPort int    `yaml:"backend_port"`
	Label       string `yaml:"label,omitempty"`
	Active      bool   `yaml:"active"`
}

type State struct {
	Routes map[string]*RouteEntry `yaml:"routes,omitempty"`
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
		state:    &State{Routes: make(map[string]*RouteEntry)},
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

func (m *StateManager) FilePath() string {
	return m.filePath
}

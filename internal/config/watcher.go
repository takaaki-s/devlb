package config

import (
	"crypto/sha256"
	"log"
	"os"
	"time"
)

// ConfigDiff represents changes between two configs.
type ConfigDiff struct {
	Added   []Service
	Removed []Service
	Changed []ServiceChange
}

// ServiceChange represents a service whose port changed.
type ServiceChange struct {
	Name    string
	OldPort int
	NewPort int
}

// DiffConfigs compares two configs and returns the differences.
func DiffConfigs(old, new *Config) ConfigDiff {
	var diff ConfigDiff

	oldMap := make(map[string]Service, len(old.Services))
	for _, s := range old.Services {
		oldMap[s.Name] = s
	}

	newMap := make(map[string]Service, len(new.Services))
	for _, s := range new.Services {
		newMap[s.Name] = s
	}

	// Find added and changed
	for _, s := range new.Services {
		oldSvc, exists := oldMap[s.Name]
		if !exists {
			diff.Added = append(diff.Added, s)
		} else if oldSvc.Port != s.Port {
			diff.Changed = append(diff.Changed, ServiceChange{
				Name:    s.Name,
				OldPort: oldSvc.Port,
				NewPort: s.Port,
			})
		}
	}

	// Find removed
	for _, s := range old.Services {
		if _, exists := newMap[s.Name]; !exists {
			diff.Removed = append(diff.Removed, s)
		}
	}

	return diff
}

// ConfigWatcher watches a config file for changes and notifies via callback.
type ConfigWatcher struct {
	path     string
	interval time.Duration
	lastHash [32]byte
	lastCfg  *Config
	onChange func(*Config, *Config)
	done     chan struct{}
}

// NewConfigWatcher creates a new config file watcher.
func NewConfigWatcher(path string, interval time.Duration, onChange func(*Config, *Config)) *ConfigWatcher {
	return &ConfigWatcher{
		path:     path,
		interval: interval,
		onChange: onChange,
		done:     make(chan struct{}),
	}
}

// Start begins watching the config file.
func (cw *ConfigWatcher) Start() {
	// Load initial state
	data, err := os.ReadFile(cw.path)
	if err != nil {
		log.Printf("[config-watcher] failed to read initial config: %v", err)
		return
	}
	cw.lastHash = sha256.Sum256(data)
	cw.lastCfg, _ = LoadConfig(cw.path)

	go cw.loop()
}

// Stop stops the watcher.
func (cw *ConfigWatcher) Stop() {
	select {
	case <-cw.done:
		return
	default:
		close(cw.done)
	}
}

func (cw *ConfigWatcher) loop() {
	ticker := time.NewTicker(cw.interval)
	defer ticker.Stop()

	for {
		select {
		case <-cw.done:
			return
		case <-ticker.C:
			cw.check()
		}
	}
}

func (cw *ConfigWatcher) check() {
	data, err := os.ReadFile(cw.path)
	if err != nil {
		return
	}

	hash := sha256.Sum256(data)
	if hash == cw.lastHash {
		return
	}

	newCfg, err := LoadConfig(cw.path)
	if err != nil {
		log.Printf("[config-watcher] failed to parse updated config: %v", err)
		return
	}

	oldCfg := cw.lastCfg
	cw.lastHash = hash
	cw.lastCfg = newCfg

	log.Printf("[config-watcher] config change detected")
	cw.onChange(oldCfg, newCfg)
}

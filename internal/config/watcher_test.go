package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiffConfigsNoChange(t *testing.T) {
	old := &Config{Services: []Service{{Name: "api", Port: 8080}}}
	new := &Config{Services: []Service{{Name: "api", Port: 8080}}}

	diff := DiffConfigs(old, new)
	if len(diff.Added) != 0 || len(diff.Removed) != 0 || len(diff.Changed) != 0 {
		t.Errorf("expected empty diff, got: added=%d removed=%d changed=%d",
			len(diff.Added), len(diff.Removed), len(diff.Changed))
	}
}

func TestDiffConfigsAddService(t *testing.T) {
	old := &Config{Services: []Service{{Name: "api", Port: 8080}}}
	new := &Config{Services: []Service{
		{Name: "api", Port: 8080},
		{Name: "auth", Port: 9090},
	}}

	diff := DiffConfigs(old, new)
	if len(diff.Added) != 1 {
		t.Fatalf("expected 1 added, got %d", len(diff.Added))
	}
	if diff.Added[0].Name != "auth" {
		t.Errorf("expected added service 'auth', got %q", diff.Added[0].Name)
	}
}

func TestDiffConfigsRemoveService(t *testing.T) {
	old := &Config{Services: []Service{
		{Name: "api", Port: 8080},
		{Name: "auth", Port: 9090},
	}}
	new := &Config{Services: []Service{{Name: "api", Port: 8080}}}

	diff := DiffConfigs(old, new)
	if len(diff.Removed) != 1 {
		t.Fatalf("expected 1 removed, got %d", len(diff.Removed))
	}
	if diff.Removed[0].Name != "auth" {
		t.Errorf("expected removed service 'auth', got %q", diff.Removed[0].Name)
	}
}

func TestDiffConfigsChangePort(t *testing.T) {
	old := &Config{Services: []Service{{Name: "api", Port: 8080}}}
	new := &Config{Services: []Service{{Name: "api", Port: 9090}}}

	diff := DiffConfigs(old, new)
	if len(diff.Changed) != 1 {
		t.Fatalf("expected 1 changed, got %d", len(diff.Changed))
	}
	if diff.Changed[0].Name != "api" || diff.Changed[0].OldPort != 8080 || diff.Changed[0].NewPort != 9090 {
		t.Errorf("unexpected change: %+v", diff.Changed[0])
	}
}

func TestDiffConfigsMixed(t *testing.T) {
	old := &Config{Services: []Service{
		{Name: "api", Port: 8080},
		{Name: "old-svc", Port: 7070},
	}}
	new := &Config{Services: []Service{
		{Name: "api", Port: 9090},
		{Name: "new-svc", Port: 6060},
	}}

	diff := DiffConfigs(old, new)
	if len(diff.Added) != 1 {
		t.Errorf("expected 1 added, got %d", len(diff.Added))
	}
	if len(diff.Removed) != 1 {
		t.Errorf("expected 1 removed, got %d", len(diff.Removed))
	}
	if len(diff.Changed) != 1 {
		t.Errorf("expected 1 changed, got %d", len(diff.Changed))
	}
}

func TestConfigWatcherDetectsChange(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "devlb.yaml")

	// Write initial config
	initial := []byte("services:\n  - name: api\n    port: 8080\n")
	if err := os.WriteFile(cfgPath, initial, 0644); err != nil {
		t.Fatal(err)
	}

	called := make(chan ConfigDiff, 1)
	cw := NewConfigWatcher(cfgPath, 100*time.Millisecond, func(oldCfg, newCfg *Config) {
		called <- DiffConfigs(oldCfg, newCfg)
	})
	cw.Start()
	defer cw.Stop()

	// Wait for initial load
	time.Sleep(200 * time.Millisecond)

	// Modify config
	modified := []byte("services:\n  - name: api\n    port: 8080\n  - name: auth\n    port: 9090\n")
	if err := os.WriteFile(cfgPath, modified, 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for detection
	select {
	case diff := <-called:
		if len(diff.Added) != 1 || diff.Added[0].Name != "auth" {
			t.Errorf("expected auth to be added, got: %+v", diff)
		}
	case <-time.After(2 * time.Second):
		t.Error("watcher did not detect change within 2s")
	}
}

func TestConfigWatcherNoChangeNoCallback(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "devlb.yaml")

	initial := []byte("services:\n  - name: api\n    port: 8080\n")
	if err := os.WriteFile(cfgPath, initial, 0644); err != nil {
		t.Fatal(err)
	}

	callCount := 0
	cw := NewConfigWatcher(cfgPath, 100*time.Millisecond, func(oldCfg, newCfg *Config) {
		callCount++
	})
	cw.Start()
	defer cw.Stop()

	// Wait several intervals without changing
	time.Sleep(500 * time.Millisecond)

	if callCount != 0 {
		t.Errorf("expected 0 callbacks, got %d", callCount)
	}
}

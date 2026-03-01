package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "devlb.yaml")
	data := []byte(`services:
  - name: api
    port: 3000
  - name: auth
    port: 8995
`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if len(cfg.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(cfg.Services))
	}
	if cfg.Services[0].Name != "api" || cfg.Services[0].Port != 3000 {
		t.Errorf("unexpected first service: %+v", cfg.Services[0])
	}
	if cfg.Services[1].Name != "auth" || cfg.Services[1].Port != 8995 {
		t.Errorf("unexpected second service: %+v", cfg.Services[1])
	}
}

func TestFindService(t *testing.T) {
	cfg := &Config{
		Services: []Service{
			{Name: "api", Port: 3000},
			{Name: "auth", Port: 8995},
		},
	}

	svc, ok := cfg.FindService("api")
	if !ok {
		t.Fatal("expected to find service 'api'")
	}
	if svc.Port != 3000 {
		t.Errorf("expected port 3000, got %d", svc.Port)
	}

	_, ok = cfg.FindService("nonexistent")
	if ok {
		t.Error("expected not to find 'nonexistent'")
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "devlb.yaml")
	if err := os.WriteFile(path, []byte(":::invalid"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/devlb.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

package config

import (
	"path/filepath"
	"testing"
)

func TestNewStateManager(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStateManager(dir)
	if err != nil {
		t.Fatalf("NewStateManager failed: %v", err)
	}
	routes := sm.GetAllRoutes()
	if len(routes) != 0 {
		t.Errorf("expected empty routes, got %d", len(routes))
	}
}

func TestSetAndGetRoute(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStateManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	sm.SetRoute("api", 13000, "feat-x")

	r, ok := sm.GetRoute("api")
	if !ok {
		t.Fatal("expected to find route for 'api'")
	}
	if r.BackendPort != 13000 {
		t.Errorf("expected BackendPort=13000, got %d", r.BackendPort)
	}
	if r.Label != "feat-x" {
		t.Errorf("expected Label=feat-x, got %s", r.Label)
	}
	if !r.Active {
		t.Error("expected Active=true")
	}
}

func TestDeleteRoute(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStateManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	sm.SetRoute("api", 13000, "feat-x")
	sm.DeleteRoute("api")

	_, ok := sm.GetRoute("api")
	if ok {
		t.Error("expected route to be deleted")
	}
}

func TestGetAllRoutes(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStateManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	sm.SetRoute("api", 13000, "feat-x")
	sm.SetRoute("auth", 18995, "feat-x")

	routes := sm.GetAllRoutes()
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStateManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	sm.SetRoute("api", 13000, "feat-x")
	sm.SetRoute("auth", 18995, "feat-y")

	if err := sm.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load into a new StateManager
	sm2, err := NewStateManager(dir)
	if err != nil {
		t.Fatalf("NewStateManager (reload) failed: %v", err)
	}

	r, ok := sm2.GetRoute("api")
	if !ok {
		t.Fatal("expected to find route 'api' after reload")
	}
	if r.BackendPort != 13000 {
		t.Errorf("expected BackendPort=13000, got %d", r.BackendPort)
	}
	if r.Label != "feat-x" {
		t.Errorf("expected Label=feat-x, got %s", r.Label)
	}

	r2, ok := sm2.GetRoute("auth")
	if !ok {
		t.Fatal("expected to find route 'auth' after reload")
	}
	if r2.BackendPort != 18995 {
		t.Errorf("expected BackendPort=18995, got %d", r2.BackendPort)
	}
}

func TestStateFilePath(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStateManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(dir, "state.yaml")
	if sm.FilePath() != expected {
		t.Errorf("expected filePath=%s, got %s", expected, sm.FilePath())
	}
}

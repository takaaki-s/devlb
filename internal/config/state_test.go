package config

import (
	"os"
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

// --- Phase 2: Multi-backend state tests ---

func TestAddBackend(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStateManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	sm.AddBackend(3000, 3001, "worktree-a", 12345, "")

	backends := sm.GetBackends(3000)
	if len(backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(backends))
	}
	if !backends[0].Active {
		t.Error("first backend should be active")
	}
	if backends[0].BackendPort != 3001 {
		t.Errorf("expected BackendPort=3001, got %d", backends[0].BackendPort)
	}
	if backends[0].Label != "worktree-a" {
		t.Errorf("expected Label=worktree-a, got %s", backends[0].Label)
	}
	if backends[0].PID != 12345 {
		t.Errorf("expected PID=12345, got %d", backends[0].PID)
	}
}

func TestAddMultipleBackends(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStateManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	sm.AddBackend(3000, 3001, "a", 0, "")
	sm.AddBackend(3000, 3002, "b", 0, "")

	backends := sm.GetBackends(3000)
	if len(backends) != 2 {
		t.Fatalf("expected 2 backends, got %d", len(backends))
	}
	if !backends[0].Active {
		t.Error("first should be active")
	}
	if backends[1].Active {
		t.Error("second should be inactive")
	}
}

func TestRemoveBackendState(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStateManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	sm.AddBackend(3000, 3001, "a", 0, "")
	sm.AddBackend(3000, 3002, "b", 0, "")
	sm.RemoveBackend(3000, 3001)

	backends := sm.GetBackends(3000)
	if len(backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(backends))
	}
	if backends[0].BackendPort != 3002 {
		t.Errorf("remaining should be port 3002, got %d", backends[0].BackendPort)
	}
}

func TestRemoveActivePromotesState(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStateManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	sm.AddBackend(3000, 3001, "a", 0, "")
	sm.AddBackend(3000, 3002, "b", 0, "")
	sm.RemoveBackend(3000, 3001)

	backends := sm.GetBackends(3000)
	if len(backends) != 1 {
		t.Fatal("expected 1 backend")
	}
	if !backends[0].Active {
		t.Error("remaining backend should be promoted to active")
	}
}

func TestSwitchActive(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStateManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	sm.AddBackend(3000, 3001, "a", 0, "")
	sm.AddBackend(3000, 3002, "b", 0, "")

	if err := sm.SwitchActive(3000, "b"); err != nil {
		t.Fatal(err)
	}

	backends := sm.GetBackends(3000)
	if backends[0].Active {
		t.Error("a should now be inactive")
	}
	if !backends[1].Active {
		t.Error("b should now be active")
	}
}

func TestSwitchActiveUnknownLabel(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStateManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	sm.AddBackend(3000, 3001, "a", 0, "")

	if err := sm.SwitchActive(3000, "nonexistent"); err == nil {
		t.Error("SwitchActive with unknown label should return error")
	}
}

func TestGetAllBackends(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStateManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	sm.AddBackend(3000, 3001, "a", 0, "")
	sm.AddBackend(8995, 8996, "a", 0, "")

	all := sm.GetAllBackends()
	if len(all) != 2 {
		t.Fatalf("expected 2 listen ports, got %d", len(all))
	}
	if len(all[3000]) != 1 {
		t.Error("expected 1 backend for port 3000")
	}
	if len(all[8995]) != 1 {
		t.Error("expected 1 backend for port 8995")
	}
}

func TestBackendsSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStateManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	sm.AddBackend(3000, 3001, "worktree-a", 12345, "")
	sm.AddBackend(3000, 3002, "worktree-b", 12346, "")

	if err := sm.Save(); err != nil {
		t.Fatal(err)
	}

	sm2, err := NewStateManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	backends := sm2.GetBackends(3000)
	if len(backends) != 2 {
		t.Fatalf("expected 2 backends after reload, got %d", len(backends))
	}
	if backends[0].BackendPort != 3001 {
		t.Errorf("expected first backend port 3001, got %d", backends[0].BackendPort)
	}
	if backends[0].Label != "worktree-a" {
		t.Errorf("expected first label worktree-a, got %s", backends[0].Label)
	}
	if !backends[0].Active {
		t.Error("first backend should be active")
	}
	if backends[1].Active {
		t.Error("second backend should be inactive")
	}
}

func TestAllocatePort(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStateManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	port1, err := sm.AllocatePort()
	if err != nil {
		t.Fatal(err)
	}
	if port1 <= 0 {
		t.Errorf("expected positive port, got %d", port1)
	}

	port2, err := sm.AllocatePort()
	if err != nil {
		t.Fatal(err)
	}
	if port2 <= 0 {
		t.Errorf("expected positive port, got %d", port2)
	}
	// Ports should be different (highly likely with ephemeral ports)
	if port1 == port2 {
		t.Logf("warning: two allocations returned same port %d (unlikely but possible)", port1)
	}
}

func TestCleanStalePIDs(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStateManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Add a backend with a guaranteed-dead PID
	sm.AddBackend(3000, 3001, "stale", 99999999, "")

	removed := sm.CleanStalePIDs()
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	backends := sm.GetBackends(3000)
	if len(backends) != 0 {
		t.Errorf("expected 0 backends after cleanup, got %d", len(backends))
	}
}

func TestCleanStalePIDsKeepsLive(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStateManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Add a backend with our own PID (definitely alive)
	sm.AddBackend(3000, 3001, "live", os.Getpid(), "")

	removed := sm.CleanStalePIDs()
	if removed != 0 {
		t.Errorf("expected 0 removed, got %d", removed)
	}

	backends := sm.GetBackends(3000)
	if len(backends) != 1 {
		t.Errorf("expected 1 backend kept, got %d", len(backends))
	}
}

func TestCleanStalePIDsZeroPIDKept(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStateManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	// PID=0 means manual route — should NOT be cleaned
	sm.AddBackend(3000, 3001, "manual", 0, "")

	removed := sm.CleanStalePIDs()
	if removed != 0 {
		t.Errorf("expected 0 removed for PID=0, got %d", removed)
	}

	backends := sm.GetBackends(3000)
	if len(backends) != 1 {
		t.Errorf("expected 1 backend kept, got %d", len(backends))
	}
}

func TestCleanStalePIDsPromotesActive(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStateManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	// First backend (active) has a dead PID, second has PID=0 (manual)
	sm.AddBackend(3000, 3001, "stale-active", 99999999, "")
	sm.AddBackend(3000, 3002, "manual", 0, "")

	removed := sm.CleanStalePIDs()
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	backends := sm.GetBackends(3000)
	if len(backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(backends))
	}
	if !backends[0].Active {
		t.Error("remaining backend should be promoted to active")
	}
	if backends[0].Label != "manual" {
		t.Errorf("expected remaining to be 'manual', got %q", backends[0].Label)
	}
}

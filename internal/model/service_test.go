package model

import "testing"

func TestServiceFields(t *testing.T) {
	s := Service{Name: "api", Port: 3000}
	if s.Name != "api" {
		t.Errorf("expected Name=api, got %s", s.Name)
	}
	if s.Port != 3000 {
		t.Errorf("expected Port=3000, got %d", s.Port)
	}
}

func TestRouteFields(t *testing.T) {
	r := Route{BackendPort: 13000, Label: "feat-x", Active: true}
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

func TestRouteDefaults(t *testing.T) {
	r := Route{}
	if r.BackendPort != 0 {
		t.Errorf("expected zero BackendPort, got %d", r.BackendPort)
	}
	if r.Label != "" {
		t.Errorf("expected empty Label, got %s", r.Label)
	}
	if r.Active {
		t.Error("expected Active=false by default")
	}
}

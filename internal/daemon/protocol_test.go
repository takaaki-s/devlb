package daemon

import (
	"encoding/json"
	"testing"
)

func TestRequestMarshal(t *testing.T) {
	data, _ := json.Marshal(RouteRequest{Service: "api", Port: 13000, Label: "feat-x"})
	req := Request{Action: ActionRoute, Data: data}

	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}

	var decoded Request
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Action != ActionRoute {
		t.Errorf("expected action=%s, got %s", ActionRoute, decoded.Action)
	}

	var rr RouteRequest
	if err := json.Unmarshal(decoded.Data, &rr); err != nil {
		t.Fatal(err)
	}
	if rr.Service != "api" || rr.Port != 13000 || rr.Label != "feat-x" {
		t.Errorf("unexpected RouteRequest: %+v", rr)
	}
}

func TestResponseSuccess(t *testing.T) {
	resp := Response{Success: true}
	b, _ := json.Marshal(resp)

	var decoded Response
	_ = json.Unmarshal(b, &decoded)
	if !decoded.Success {
		t.Error("expected Success=true")
	}
	if decoded.Error != "" {
		t.Errorf("expected empty error, got %s", decoded.Error)
	}
}

func TestResponseError(t *testing.T) {
	resp := Response{Success: false, Error: "service not found"}
	b, _ := json.Marshal(resp)

	var decoded Response
	_ = json.Unmarshal(b, &decoded)
	if decoded.Success {
		t.Error("expected Success=false")
	}
	if decoded.Error != "service not found" {
		t.Errorf("expected error='service not found', got %s", decoded.Error)
	}
}

func TestActionConstants(t *testing.T) {
	actions := []string{
		ActionRoute, ActionUnroute, ActionStatus, ActionStop,
		ActionRegister, ActionUnregister, ActionSwitch, ActionAllocate,
	}
	seen := make(map[string]bool)
	for _, a := range actions {
		if a == "" {
			t.Error("action constant should not be empty")
		}
		if seen[a] {
			t.Errorf("duplicate action constant: %s", a)
		}
		seen[a] = true
	}
}

// --- Phase 2 protocol tests ---

func TestRegisterRequestMarshal(t *testing.T) {
	rr := RegisterRequest{
		ListenPort:  3000,
		BackendPort: 3001,
		Label:       "worktree-a",
		PID:         12345,
	}
	data, err := json.Marshal(rr)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RegisterRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ListenPort != 3000 || decoded.BackendPort != 3001 ||
		decoded.Label != "worktree-a" || decoded.PID != 12345 {
		t.Errorf("unexpected RegisterRequest: %+v", decoded)
	}
}

func TestSwitchRequestMarshal(t *testing.T) {
	// With listen port
	sr := SwitchRequest{ListenPort: 3000, Label: "worktree-b"}
	data, _ := json.Marshal(sr)
	var decoded SwitchRequest
	_ = json.Unmarshal(data, &decoded)
	if decoded.ListenPort != 3000 || decoded.Label != "worktree-b" {
		t.Errorf("unexpected SwitchRequest: %+v", decoded)
	}

	// Without listen port (all ports)
	sr2 := SwitchRequest{Label: "worktree-b"}
	data2, _ := json.Marshal(sr2)
	var decoded2 SwitchRequest
	_ = json.Unmarshal(data2, &decoded2)
	if decoded2.ListenPort != 0 || decoded2.Label != "worktree-b" {
		t.Errorf("unexpected SwitchRequest without port: %+v", decoded2)
	}
}

func TestAllocateRequestResponseMarshal(t *testing.T) {
	req := AllocateRequest{ListenPort: 3000}
	data, _ := json.Marshal(req)
	var decodedReq AllocateRequest
	_ = json.Unmarshal(data, &decodedReq)
	if decodedReq.ListenPort != 3000 {
		t.Errorf("expected ListenPort=3000, got %d", decodedReq.ListenPort)
	}

	resp := AllocateResponse{BackendPort: 3001}
	data2, _ := json.Marshal(resp)
	var decodedResp AllocateResponse
	_ = json.Unmarshal(data2, &decodedResp)
	if decodedResp.BackendPort != 3001 {
		t.Errorf("expected BackendPort=3001, got %d", decodedResp.BackendPort)
	}
}

func TestStatusEntryWithBackends(t *testing.T) {
	entry := StatusEntry{
		ListenPort: 3000,
		Status:     "active",
		Backends: []BackendInfo{
			{Port: 3001, Label: "a", Active: true},
			{Port: 3002, Label: "b", Active: false},
		},
	}
	data, _ := json.Marshal(entry)
	var decoded StatusEntry
	_ = json.Unmarshal(data, &decoded)
	if len(decoded.Backends) != 2 {
		t.Fatalf("expected 2 backends, got %d", len(decoded.Backends))
	}
	if !decoded.Backends[0].Active || decoded.Backends[1].Active {
		t.Error("backend active flags mismatch")
	}
}

func TestUnrouteRequestMarshal(t *testing.T) {
	data, _ := json.Marshal(UnrouteRequest{Service: "auth"})
	req := Request{Action: ActionUnroute, Data: data}

	b, _ := json.Marshal(req)
	var decoded Request
	_ = json.Unmarshal(b, &decoded)

	var ur UnrouteRequest
	_ = json.Unmarshal(decoded.Data, &ur)
	if ur.Service != "auth" {
		t.Errorf("expected Service=auth, got %s", ur.Service)
	}
}

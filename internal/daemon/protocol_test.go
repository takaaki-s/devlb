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
	actions := []string{ActionRoute, ActionUnroute, ActionStatus, ActionStop}
	for _, a := range actions {
		if a == "" {
			t.Error("action constant should not be empty")
		}
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

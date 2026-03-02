package daemon

import "encoding/json"

const (
	ActionRoute   = "route"
	ActionUnroute = "unroute"
	ActionStatus  = "status"
	ActionStop    = "stop"

	// Phase 2 actions
	ActionRegister   = "register"
	ActionUnregister = "unregister"
	ActionSwitch     = "switch"
	ActionAllocate   = "allocate"
)

type Request struct {
	Action string          `json:"action"`
	Data   json.RawMessage `json:"data,omitempty"`
}

type Response struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type RouteRequest struct {
	Service string `json:"service"`
	Port    int    `json:"port"`
	Label   string `json:"label,omitempty"`
}

type UnrouteRequest struct {
	Service string `json:"service"`
}

type StatusEntry struct {
	Service     string        `json:"service,omitempty"`
	ListenPort  int           `json:"listen_port"`
	BackendPort int           `json:"backend_port,omitempty"`
	Label       string        `json:"label,omitempty"`
	Status      string        `json:"status"`
	ActiveConns int64         `json:"active_conns"`
	Backends    []BackendInfo `json:"backends,omitempty"`
	BlockedBy   string        `json:"blocked_by,omitempty"`
}

type BackendInfo struct {
	Port   int    `json:"port"`
	Label  string `json:"label"`
	Active bool   `json:"active"`
	PID    int    `json:"pid,omitempty"`
}

type StatusResponse struct {
	Entries []StatusEntry `json:"entries"`
}

// Phase 2 request/response types

type RegisterRequest struct {
	ListenPort  int    `json:"listen_port"`
	BackendPort int    `json:"backend_port"`
	Label       string `json:"label"`
	PID         int    `json:"pid,omitempty"`
}

type UnregisterRequest struct {
	ListenPort  int `json:"listen_port"`
	BackendPort int `json:"backend_port"`
}

type SwitchRequest struct {
	ListenPort int    `json:"listen_port,omitempty"`
	Label      string `json:"label"`
}

type AllocateRequest struct {
	ListenPort int `json:"listen_port"`
}

type AllocateResponse struct {
	BackendPort int `json:"backend_port"`
}

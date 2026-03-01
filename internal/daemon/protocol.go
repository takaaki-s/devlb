package daemon

import "encoding/json"

const (
	ActionRoute   = "route"
	ActionUnroute = "unroute"
	ActionStatus  = "status"
	ActionStop    = "stop"
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
	Service     string `json:"service"`
	ListenPort  int    `json:"listen_port"`
	BackendPort int    `json:"backend_port,omitempty"`
	Label       string `json:"label,omitempty"`
	Status      string `json:"status"`
	ActiveConns int64  `json:"active_conns"`
}

type StatusResponse struct {
	Entries []StatusEntry `json:"entries"`
}

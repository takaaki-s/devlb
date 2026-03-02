package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	"github.com/takaaki-s/devlb/internal/config"
	"github.com/takaaki-s/devlb/internal/proxy"
)

type Server struct {
	socketPath string
	config     *config.Config
	state      *config.StateManager
	listeners  map[string]*proxy.ServiceListener // Phase 1: by service name
	portMap    map[int]*proxy.ServiceListener    // Phase 2: by listen port
	mu         sync.Mutex                        // protects portMap for dynamic listeners
	listener   net.Listener
	done       chan struct{}
}

func NewServer(socketPath string, cfg *config.Config, state *config.StateManager) (*Server, error) {
	s := &Server{
		socketPath: socketPath,
		config:     cfg,
		state:      state,
		listeners:  make(map[string]*proxy.ServiceListener),
		portMap:    make(map[int]*proxy.ServiceListener),
		done:       make(chan struct{}),
	}

	// Create a ServiceListener for each configured service
	for _, svc := range cfg.Services {
		sl := proxy.NewServiceListener(svc.Name, svc.Port)
		s.listeners[svc.Name] = sl
		if svc.Port > 0 {
			s.portMap[svc.Port] = sl
		}
	}

	return s, nil
}

func (s *Server) Start() error {
	// Start all service listeners
	for name, sl := range s.listeners {
		if err := sl.Start(); err != nil {
			log.Printf("[%s] failed to start listener: %v", name, err)
			continue
		}
		// Update portMap with actual port (for ephemeral port 0)
		info := sl.Info()
		if info.ListenAddr != "" {
			var p int
			_, _ = fmt.Sscanf(info.ListenAddr, "127.0.0.1:%d", &p)
			if p > 0 {
				s.portMap[p] = sl
			}
		}
	}

	// Restore routes from state (Phase 1 legacy)
	for name, route := range s.state.GetAllRoutes() {
		if sl, ok := s.listeners[name]; ok && route.Active {
			sl.SetBackend(route.BackendPort, route.Label)
		}
	}

	// Restore backends from state (Phase 2)
	for listenPort, backends := range s.state.GetAllBackends() {
		sl := s.getOrCreateListener(listenPort)
		if sl == nil {
			continue
		}
		for _, b := range backends {
			sl.AddBackend(b.BackendPort, b.Label, b.PID)
			if b.Active {
				_ = sl.SwitchBackend(b.Label)
			}
		}
	}

	// Remove existing socket file
	os.Remove(s.socketPath)

	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.socketPath, err)
	}
	s.listener = ln

	// Accept connections
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.done:
				return nil
			default:
				return fmt.Errorf("accept: %w", err)
			}
		}
		go s.handleConnection(conn)
	}
}

func (s *Server) Stop() {
	select {
	case <-s.done:
		return
	default:
		close(s.done)
	}

	if s.listener != nil {
		s.listener.Close()
	}

	for _, sl := range s.listeners {
		sl.Stop()
	}

	// Stop dynamic listeners not in config
	s.mu.Lock()
	for port, sl := range s.portMap {
		if _, ok := s.listeners[s.serviceNameForPort(port)]; !ok {
			sl.Stop()
		}
	}
	s.mu.Unlock()

	os.Remove(s.socketPath)
}

func (s *Server) serviceNameForPort(port int) string {
	for _, svc := range s.config.Services {
		if svc.Port == port {
			return svc.Name
		}
	}
	return ""
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	var req Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return
	}

	var resp Response

	switch req.Action {
	case ActionRoute:
		resp = s.handleRoute(req.Data)
	case ActionUnroute:
		resp = s.handleUnroute(req.Data)
	case ActionStatus:
		resp = s.handleStatus()
	case ActionStop:
		resp = Response{Success: true}
		_ = json.NewEncoder(conn).Encode(resp)
		s.Stop()
		return
	case ActionRegister:
		resp = s.handleRegister(req.Data)
	case ActionUnregister:
		resp = s.handleUnregister(req.Data)
	case ActionSwitch:
		resp = s.handleSwitch(req.Data)
	case ActionAllocate:
		resp = s.handleAllocate(req.Data)
	default:
		resp = Response{Success: false, Error: fmt.Sprintf("unknown action: %s", req.Action)}
	}

	_ = json.NewEncoder(conn).Encode(resp)
}

// Phase 1 legacy handler - uses service name
func (s *Server) handleRoute(data json.RawMessage) Response {
	var rr RouteRequest
	if err := json.Unmarshal(data, &rr); err != nil {
		return Response{Success: false, Error: fmt.Sprintf("invalid route request: %v", err)}
	}

	sl, ok := s.listeners[rr.Service]
	if !ok {
		return Response{Success: false, Error: fmt.Sprintf("unknown service: %s", rr.Service)}
	}

	sl.SetBackend(rr.Port, rr.Label)
	s.state.SetRoute(rr.Service, rr.Port, rr.Label)
	if err := s.state.Save(); err != nil {
		log.Printf("failed to save state: %v", err)
	}

	return Response{Success: true}
}

// Phase 1 legacy handler - uses service name
func (s *Server) handleUnroute(data json.RawMessage) Response {
	var ur UnrouteRequest
	if err := json.Unmarshal(data, &ur); err != nil {
		return Response{Success: false, Error: fmt.Sprintf("invalid unroute request: %v", err)}
	}

	sl, ok := s.listeners[ur.Service]
	if !ok {
		return Response{Success: false, Error: fmt.Sprintf("unknown service: %s", ur.Service)}
	}

	sl.ClearBackend()
	s.state.DeleteRoute(ur.Service)
	if err := s.state.Save(); err != nil {
		log.Printf("failed to save state: %v", err)
	}

	return Response{Success: true}
}

// Phase 2 handler - register backend by port
func (s *Server) handleRegister(data json.RawMessage) Response {
	var rr RegisterRequest
	if err := json.Unmarshal(data, &rr); err != nil {
		return Response{Success: false, Error: fmt.Sprintf("invalid register request: %v", err)}
	}

	sl := s.getOrCreateListener(rr.ListenPort)
	if sl == nil {
		return Response{Success: false, Error: fmt.Sprintf("failed to create listener for port %d", rr.ListenPort)}
	}

	sl.AddBackend(rr.BackendPort, rr.Label, rr.PID)
	s.state.AddBackend(rr.ListenPort, rr.BackendPort, rr.Label, rr.PID)
	if err := s.state.Save(); err != nil {
		log.Printf("failed to save state: %v", err)
	}

	return Response{Success: true}
}

// Phase 2 handler - unregister backend by port
func (s *Server) handleUnregister(data json.RawMessage) Response {
	var ur UnregisterRequest
	if err := json.Unmarshal(data, &ur); err != nil {
		return Response{Success: false, Error: fmt.Sprintf("invalid unregister request: %v", err)}
	}

	s.mu.Lock()
	sl, ok := s.portMap[ur.ListenPort]
	s.mu.Unlock()
	if !ok {
		return Response{Success: false, Error: fmt.Sprintf("no listener for port %d", ur.ListenPort)}
	}

	sl.RemoveBackend(ur.BackendPort)
	s.state.RemoveBackend(ur.ListenPort, ur.BackendPort)
	if err := s.state.Save(); err != nil {
		log.Printf("failed to save state: %v", err)
	}

	return Response{Success: true}
}

// Phase 2 handler - switch active backend
func (s *Server) handleSwitch(data json.RawMessage) Response {
	var sr SwitchRequest
	if err := json.Unmarshal(data, &sr); err != nil {
		return Response{Success: false, Error: fmt.Sprintf("invalid switch request: %v", err)}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if sr.ListenPort > 0 {
		// Switch specific port
		sl, ok := s.portMap[sr.ListenPort]
		if !ok {
			return Response{Success: false, Error: fmt.Sprintf("no listener for port %d", sr.ListenPort)}
		}
		if err := sl.SwitchBackend(sr.Label); err != nil {
			return Response{Success: false, Error: err.Error()}
		}
		if err := s.state.SwitchActive(sr.ListenPort, sr.Label); err != nil {
			log.Printf("failed to switch state: %v", err)
		}
	} else {
		// Switch all ports that have a backend with this label
		for port, sl := range s.portMap {
			// Try to switch, ignore errors (port may not have this label)
			if err := sl.SwitchBackend(sr.Label); err == nil {
				_ = s.state.SwitchActive(port, sr.Label)
			}
		}
	}

	if err := s.state.Save(); err != nil {
		log.Printf("failed to save state: %v", err)
	}

	return Response{Success: true}
}

// Phase 2 handler - allocate ephemeral port
func (s *Server) handleAllocate(data json.RawMessage) Response {
	var ar AllocateRequest
	if err := json.Unmarshal(data, &ar); err != nil {
		return Response{Success: false, Error: fmt.Sprintf("invalid allocate request: %v", err)}
	}

	port, err := s.state.AllocatePort()
	if err != nil {
		return Response{Success: false, Error: err.Error()}
	}

	respData, _ := json.Marshal(AllocateResponse{BackendPort: port})
	return Response{Success: true, Data: respData}
}

func (s *Server) handleStatus() Response {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Collect all unique listen ports from config and portMap
	type entryData struct {
		service    string
		listenPort int
		sl         *proxy.ServiceListener
	}
	var entries []entryData

	// Config services first
	seen := make(map[int]bool)
	for _, svc := range s.config.Services {
		sl, ok := s.listeners[svc.Name]
		if !ok {
			continue
		}
		info := sl.Info()
		listenPort := svc.Port
		if info.ListenAddr != "" {
			var p int
			_, _ = fmt.Sscanf(info.ListenAddr, "127.0.0.1:%d", &p)
			if p > 0 {
				listenPort = p
			}
		}
		entries = append(entries, entryData{service: svc.Name, listenPort: listenPort, sl: sl})
		seen[listenPort] = true
	}

	// Dynamic listeners
	for port, sl := range s.portMap {
		if !seen[port] {
			entries = append(entries, entryData{listenPort: port, sl: sl})
		}
	}

	result := make([]StatusEntry, 0, len(entries))
	for _, ed := range entries {
		info := ed.sl.Info()
		status := "idle"
		if info.BackendPort > 0 {
			status = "active"
		}
		if !info.Listening {
			status = "stopped"
		}

		backends := ed.sl.Backends()
		var backendInfos []BackendInfo
		for _, b := range backends {
			backendInfos = append(backendInfos, BackendInfo{
				Port:   b.Port,
				Label:  b.Label,
				Active: b.Active,
				PID:    b.PID,
			})
		}

		result = append(result, StatusEntry{
			Service:     ed.service,
			ListenPort:  ed.listenPort,
			BackendPort: info.BackendPort,
			Label:       info.Label,
			Status:      status,
			ActiveConns: info.ActiveConns,
			Backends:    backendInfos,
		})
	}

	respData, _ := json.Marshal(StatusResponse{Entries: result})
	return Response{Success: true, Data: respData}
}

// getOrCreateListener returns a listener for the given port, creating one dynamically if needed.
func (s *Server) getOrCreateListener(port int) *proxy.ServiceListener {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sl, ok := s.portMap[port]; ok {
		return sl
	}

	// Check if any running config listener is on this actual port
	for _, sl := range s.listeners {
		info := sl.Info()
		if info.ListenAddr != "" {
			var p int
			_, _ = fmt.Sscanf(info.ListenAddr, "127.0.0.1:%d", &p)
			if p == port {
				s.portMap[port] = sl
				return sl
			}
		}
	}

	// Create dynamic listener
	sl := proxy.NewServiceListener(fmt.Sprintf("dynamic-%d", port), port)
	if err := sl.Start(); err != nil {
		log.Printf("failed to start dynamic listener on port %d: %v", port, err)
		return nil
	}
	s.portMap[port] = sl
	return sl
}

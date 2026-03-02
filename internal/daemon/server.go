package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/takaaki-s/devlb/internal/config"
	"github.com/takaaki-s/devlb/internal/proxy"
)

// DefaultDrainTimeout is the maximum time to wait for active connections to
// drain during graceful shutdown.
const DefaultDrainTimeout = 10 * time.Second

type Server struct {
	socketPath    string
	configPath    string
	config        *config.Config
	state         *config.StateManager
	listeners     map[string]*proxy.ServiceListener // Phase 1: by service name
	portMap       map[int]*proxy.ServiceListener    // Phase 2: by listen port
	mu            sync.Mutex                        // protects portMap for dynamic listeners
	listener      net.Listener
	done          chan struct{}
	configWatcher *config.ConfigWatcher
}

func NewServer(socketPath string, cfg *config.Config, state *config.StateManager) (*Server, error) {
	return NewServerWithConfigPath(socketPath, "", cfg, state)
}

func NewServerWithConfigPath(socketPath, configPath string, cfg *config.Config, state *config.StateManager) (*Server, error) {
	s := &Server{
		socketPath: socketPath,
		configPath: configPath,
		config:     cfg,
		state:      state,
		listeners:  make(map[string]*proxy.ServiceListener),
		portMap:    make(map[int]*proxy.ServiceListener),
		done:       make(chan struct{}),
	}

	// Create a ServiceListener for each configured service
	for _, svc := range cfg.Services {
		sl := proxy.NewServiceListener(svc.Name, svc.Port)
		if hcCfg := s.healthConfig(); hcCfg != nil {
			sl.SetHealthChecker(proxy.NewHealthChecker(*hcCfg))
		}
		s.listeners[svc.Name] = sl
		if svc.Port > 0 {
			s.portMap[svc.Port] = sl
		}
	}

	return s, nil
}

// healthConfig parses the config's health check settings into a proxy.HealthConfig.
// Returns nil if health checks are not enabled.
func (s *Server) healthConfig() *proxy.HealthConfig {
	if s.config.HealthCheck == nil || !s.config.HealthCheck.Enabled {
		return nil
	}

	cfg := proxy.DefaultHealthConfig()

	if s.config.HealthCheck.Interval != "" {
		if d, err := time.ParseDuration(s.config.HealthCheck.Interval); err == nil {
			cfg.Interval = d
		}
	}
	if s.config.HealthCheck.Timeout != "" {
		if d, err := time.ParseDuration(s.config.HealthCheck.Timeout); err == nil {
			cfg.Timeout = d
		}
	}
	if s.config.HealthCheck.UnhealthyAfter > 0 {
		cfg.UnhealthyAfter = s.config.HealthCheck.UnhealthyAfter
	}

	return &cfg
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

	// Clean stale PIDs before restoring Phase 2 backends
	if n := s.state.CleanStalePIDs(); n > 0 {
		log.Printf("cleaned %d stale backend entries", n)
		if err := s.state.Save(); err != nil {
			log.Printf("failed to save cleaned state: %v", err)
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

	// Start periodic PID sweeper
	s.startPIDSweeper(30 * time.Second)

	// Start config watcher if config path is set
	if s.configPath != "" {
		s.configWatcher = config.NewConfigWatcher(s.configPath, 2*time.Second, s.applyConfigChange)
		s.configWatcher.Start()
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
	s.StopGraceful(0)
}

// StopGraceful stops the server, draining active connections up to the given timeout.
// If timeout is 0, connections are closed immediately.
func (s *Server) StopGraceful(timeout time.Duration) {
	select {
	case <-s.done:
		return
	default:
		close(s.done)
	}

	if s.configWatcher != nil {
		s.configWatcher.Stop()
	}
	if s.listener != nil {
		s.listener.Close()
	}

	// Drain all listeners in parallel
	var wg sync.WaitGroup
	stopped := make(map[*proxy.ServiceListener]bool)

	for _, sl := range s.listeners {
		stopped[sl] = true
		wg.Add(1)
		go func(sl *proxy.ServiceListener) {
			defer wg.Done()
			if timeout > 0 {
				sl.StopGraceful(timeout)
			} else {
				sl.Stop()
			}
		}(sl)
	}

	s.mu.Lock()
	for _, sl := range s.portMap {
		if !stopped[sl] {
			stopped[sl] = true
			wg.Add(1)
			go func(sl *proxy.ServiceListener) {
				defer wg.Done()
				if timeout > 0 {
					sl.StopGraceful(timeout)
				} else {
					sl.Stop()
				}
			}(sl)
		}
	}
	s.mu.Unlock()

	wg.Wait()
	os.Remove(s.socketPath)
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
		go s.StopGraceful(DefaultDrainTimeout)
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
		if info.Blocked {
			status = "blocked"
		} else if !info.Listening {
			status = "stopped"
		}

		backends := ed.sl.Backends()
		hc := ed.sl.GetHealthChecker()
		ms := ed.sl.Metrics()
		var backendInfos []BackendInfo
		for _, b := range backends {
			bi := BackendInfo{
				Port:   b.Port,
				Label:  b.Label,
				Active: b.Active,
				PID:    b.PID,
			}
			if hc != nil {
				h := hc.IsHealthy(b.Port)
				bi.Healthy = &h
				if st := hc.Status(b.Port); st != nil && st.LastError != "" {
					bi.LastError = st.LastError
				}
			}
			if ms != nil {
				m := ms.Get(b.Port)
				bi.TotalConns = m.TotalConns
				bi.ActiveConns = m.ActiveConns
				bi.BytesIn = m.BytesIn
				bi.BytesOut = m.BytesOut
			}
			backendInfos = append(backendInfos, bi)
		}

		result = append(result, StatusEntry{
			Service:     ed.service,
			ListenPort:  ed.listenPort,
			BackendPort: info.BackendPort,
			Label:       info.Label,
			Status:      status,
			ActiveConns: info.ActiveConns,
			Backends:    backendInfos,
			BlockedBy:   info.BlockedBy,
		})
	}

	respData, _ := json.Marshal(StatusResponse{Entries: result})
	return Response{Success: true, Data: respData}
}

func (s *Server) startPIDSweeper(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-s.done:
				ticker.Stop()
				return
			case <-ticker.C:
				if n := s.state.CleanStalePIDs(); n > 0 {
					log.Printf("periodic sweep: cleaned %d stale entries", n)
					// Also remove from in-memory listeners
					s.mu.Lock()
					for port, sl := range s.portMap {
						for _, b := range sl.Backends() {
							if b.PID > 0 && !config.IsProcessAlive(b.PID) {
								sl.RemoveBackend(b.Port)
								log.Printf("removed stale backend :%d→:%d from listener", port, b.Port)
							}
						}
					}
					s.mu.Unlock()
					_ = s.state.Save()
				}
			}
		}
	}()
}

// applyConfigChange handles a config file change by adding/removing/updating service listeners.
func (s *Server) applyConfigChange(oldCfg, newCfg *config.Config) {
	diff := config.DiffConfigs(oldCfg, newCfg)
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, svc := range diff.Added {
		sl := proxy.NewServiceListener(svc.Name, svc.Port)
		if hcCfg := s.healthConfig(); hcCfg != nil {
			sl.SetHealthChecker(proxy.NewHealthChecker(*hcCfg))
		}
		if err := sl.Start(); err != nil {
			log.Printf("[hot-reload] failed to start %s on port %d: %v", svc.Name, svc.Port, err)
			continue
		}
		s.listeners[svc.Name] = sl
		if svc.Port > 0 {
			s.portMap[svc.Port] = sl
		}
		log.Printf("[hot-reload] added service %s on port %d", svc.Name, svc.Port)
	}

	for _, svc := range diff.Removed {
		if sl, ok := s.listeners[svc.Name]; ok {
			// Find actual port from listener info (may differ from config if port was 0)
			info := sl.Info()
			var actualPort int
			if info.ListenAddr != "" {
				fmt.Sscanf(info.ListenAddr, "127.0.0.1:%d", &actualPort)
			}
			sl.StopGraceful(DefaultDrainTimeout)
			delete(s.listeners, svc.Name)
			if actualPort > 0 {
				delete(s.portMap, actualPort)
			}
			delete(s.portMap, svc.Port)
			log.Printf("[hot-reload] removed service %s", svc.Name)
		}
	}

	for _, ch := range diff.Changed {
		if sl, ok := s.listeners[ch.Name]; ok {
			// Find actual port (may differ if config was 0)
			info := sl.Info()
			var actualOldPort int
			if info.ListenAddr != "" {
				fmt.Sscanf(info.ListenAddr, "127.0.0.1:%d", &actualOldPort)
			}
			// Stop the old listener
			sl.StopGraceful(DefaultDrainTimeout)
			if actualOldPort > 0 {
				delete(s.portMap, actualOldPort)
			}
			delete(s.portMap, ch.OldPort)

			// Create a new listener on the new port
			newSL := proxy.NewServiceListener(ch.Name, ch.NewPort)
			if hcCfg := s.healthConfig(); hcCfg != nil {
				newSL.SetHealthChecker(proxy.NewHealthChecker(*hcCfg))
			}
			if err := newSL.Start(); err != nil {
				log.Printf("[hot-reload] failed to start %s on new port %d: %v", ch.Name, ch.NewPort, err)
				delete(s.listeners, ch.Name)
				continue
			}
			s.listeners[ch.Name] = newSL
			s.portMap[ch.NewPort] = newSL
			log.Printf("[hot-reload] service %s port changed %d -> %d", ch.Name, ch.OldPort, ch.NewPort)
		}
	}

	s.config = newCfg
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
	if hcCfg := s.healthConfig(); hcCfg != nil {
		sl.SetHealthChecker(proxy.NewHealthChecker(*hcCfg))
	}
	if err := sl.Start(); err != nil {
		log.Printf("failed to start dynamic listener on port %d: %v", port, err)
		return nil
	}
	s.portMap[port] = sl
	return sl
}

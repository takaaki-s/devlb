package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/takaaki-s/devlb/internal/config"
	"github.com/takaaki-s/devlb/internal/proxy"
)

type Server struct {
	socketPath string
	config     *config.Config
	state      *config.StateManager
	listeners  map[string]*proxy.ServiceListener
	listener   net.Listener
	done chan struct{}
}

func NewServer(socketPath string, cfg *config.Config, state *config.StateManager) (*Server, error) {
	s := &Server{
		socketPath: socketPath,
		config:     cfg,
		state:      state,
		listeners:  make(map[string]*proxy.ServiceListener),
		done:       make(chan struct{}),
	}

	// Create a ServiceListener for each configured service
	for _, svc := range cfg.Services {
		sl := proxy.NewServiceListener(svc.Name, svc.Port)
		s.listeners[svc.Name] = sl
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
	}

	// Restore routes from state
	for name, route := range s.state.GetAllRoutes() {
		if sl, ok := s.listeners[name]; ok && route.Active {
			sl.SetBackend(route.BackendPort, route.Label)
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
		s.Stop()
		return
	default:
		resp = Response{Success: false, Error: fmt.Sprintf("unknown action: %s", req.Action)}
	}

	_ = json.NewEncoder(conn).Encode(resp)
}

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

func (s *Server) handleStatus() Response {
	entries := make([]StatusEntry, 0, len(s.config.Services))

	for _, svc := range s.config.Services {
		sl, ok := s.listeners[svc.Name]
		if !ok {
			continue
		}

		info := sl.Info()
		status := "idle"
		if info.BackendPort > 0 {
			status = "active"
		}
		if !info.Listening {
			status = "stopped"
		}

		// Get the actual listen port from the listener
		listenPort := svc.Port
		if info.ListenAddr != "" {
			var p int
			_, _ = fmt.Sscanf(info.ListenAddr, "127.0.0.1:%d", &p)
			if p > 0 {
				listenPort = p
			}
		}

		entries = append(entries, StatusEntry{
			Service:     svc.Name,
			ListenPort:  listenPort,
			BackendPort: info.BackendPort,
			Label:       info.Label,
			Status:      status,
			ActiveConns: info.ActiveConns,
		})
	}

	data, _ := json.Marshal(StatusResponse{Entries: entries})
	return Response{Success: true, Data: data}
}

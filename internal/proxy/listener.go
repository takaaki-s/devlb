package proxy

import (
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
)

// ListenerInfo holds the current state of a ServiceListener.
type ListenerInfo struct {
	Name        string
	ListenAddr  string
	BackendPort int
	Label       string
	Listening   bool
	ActiveConns int64
}

// ServiceListener manages a TCP listener for a single service.
type ServiceListener struct {
	name       string
	listenPort int
	listener   net.Listener

	mu          sync.RWMutex
	backendPort int
	label       string

	activeConns atomic.Int64
	done        chan struct{}
}

func NewServiceListener(name string, listenPort int) *ServiceListener {
	return &ServiceListener{
		name:       name,
		listenPort: listenPort,
		done:       make(chan struct{}),
	}
}

func (sl *ServiceListener) Start() error {
	addr := fmt.Sprintf("127.0.0.1:%d", sl.listenPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	sl.listener = ln

	go sl.acceptLoop()
	return nil
}

func (sl *ServiceListener) Stop() {
	close(sl.done)
	if sl.listener != nil {
		sl.listener.Close()
	}
}

func (sl *ServiceListener) Addr() string {
	if sl.listener == nil {
		return ""
	}
	return sl.listener.Addr().String()
}

func (sl *ServiceListener) SetBackend(port int, label string) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.backendPort = port
	sl.label = label
}

func (sl *ServiceListener) ClearBackend() {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.backendPort = 0
	sl.label = ""
}

func (sl *ServiceListener) Info() ListenerInfo {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	return ListenerInfo{
		Name:        sl.name,
		ListenAddr:  sl.Addr(),
		BackendPort: sl.backendPort,
		Label:       sl.label,
		Listening:   sl.listener != nil,
		ActiveConns: sl.activeConns.Load(),
	}
}

func (sl *ServiceListener) acceptLoop() {
	for {
		conn, err := sl.listener.Accept()
		if err != nil {
			select {
			case <-sl.done:
				return
			default:
				log.Printf("[%s] accept error: %v", sl.name, err)
				return
			}
		}
		go sl.handleConn(conn)
	}
}

func (sl *ServiceListener) handleConn(client net.Conn) {
	sl.mu.RLock()
	port := sl.backendPort
	sl.mu.RUnlock()

	if port == 0 {
		client.Close()
		return
	}

	backend, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		log.Printf("[%s] backend connect failed: %v", sl.name, err)
		client.Close()
		return
	}

	sl.activeConns.Add(1)
	defer sl.activeConns.Add(-1)

	Bridge(client, backend)
}

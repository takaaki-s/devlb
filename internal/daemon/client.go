package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
)

type Client struct {
	socketPath string
}

func NewClient(socketPath string) *Client {
	return &Client{socketPath: socketPath}
}

func (c *Client) IsRunning() bool {
	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (c *Client) send(req Request) (*Response, error) {
	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		return nil, fmt.Errorf("daemon not running. Start with: devlb start")
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, err
	}

	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Route(service string, port int, label string) error {
	data, _ := json.Marshal(RouteRequest{Service: service, Port: port, Label: label})
	resp, err := c.send(Request{Action: ActionRoute, Data: data})
	if err != nil {
		return err
	}
	if !resp.Success {
		return errors.New(resp.Error)
	}
	return nil
}

func (c *Client) Unroute(service string) error {
	data, _ := json.Marshal(UnrouteRequest{Service: service})
	resp, err := c.send(Request{Action: ActionUnroute, Data: data})
	if err != nil {
		return err
	}
	if !resp.Success {
		return errors.New(resp.Error)
	}
	return nil
}

func (c *Client) Status() (*StatusResponse, error) {
	resp, err := c.send(Request{Action: ActionStatus})
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, errors.New(resp.Error)
	}

	var sr StatusResponse
	if err := json.Unmarshal(resp.Data, &sr); err != nil {
		return nil, err
	}
	return &sr, nil
}

// Phase 2 methods

func (c *Client) Register(listenPort, backendPort int, label string, pid int, logFile string) error {
	data, _ := json.Marshal(RegisterRequest{
		ListenPort:  listenPort,
		BackendPort: backendPort,
		Label:       label,
		PID:         pid,
		LogFile:     logFile,
	})
	resp, err := c.send(Request{Action: ActionRegister, Data: data})
	if err != nil {
		return err
	}
	if !resp.Success {
		return errors.New(resp.Error)
	}
	return nil
}

func (c *Client) Unregister(listenPort, backendPort int) error {
	data, _ := json.Marshal(UnregisterRequest{
		ListenPort:  listenPort,
		BackendPort: backendPort,
	})
	resp, err := c.send(Request{Action: ActionUnregister, Data: data})
	if err != nil {
		return err
	}
	if !resp.Success {
		return errors.New(resp.Error)
	}
	return nil
}

func (c *Client) Switch(listenPort int, label string) error {
	data, _ := json.Marshal(SwitchRequest{
		ListenPort: listenPort,
		Label:      label,
	})
	resp, err := c.send(Request{Action: ActionSwitch, Data: data})
	if err != nil {
		return err
	}
	if !resp.Success {
		return errors.New(resp.Error)
	}
	return nil
}

func (c *Client) Allocate(listenPort int) (int, error) {
	data, _ := json.Marshal(AllocateRequest{ListenPort: listenPort})
	resp, err := c.send(Request{Action: ActionAllocate, Data: data})
	if err != nil {
		return 0, err
	}
	if !resp.Success {
		return 0, errors.New(resp.Error)
	}

	var ar AllocateResponse
	if err := json.Unmarshal(resp.Data, &ar); err != nil {
		return 0, err
	}
	return ar.BackendPort, nil
}

func (c *Client) Stop() error {
	resp, err := c.send(Request{Action: ActionStop})
	if err != nil {
		return err
	}
	if !resp.Success {
		return errors.New(resp.Error)
	}
	return nil
}

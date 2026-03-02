package exec

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/takaaki-s/devlb/internal/daemon"
	"github.com/takaaki-s/devlb/internal/label"
	"github.com/takaaki-s/devlb/internal/portswap"
)

// WrapperConfig configures the exec wrapper.
type WrapperConfig struct {
	SocketPath   string
	ListenPorts  []int          // e.g., [3000, 8995]
	BackendPorts map[int]int    // listen→backend overrides (empty = auto-allocate all)
	Label        string         // empty = auto-detect via git branch
	Command      string
	Args         []string
}

// RunWrapper orchestrates: allocate ports → register → portswap.Run → unregister.
func RunWrapper(cfg WrapperConfig) (int, error) {
	client := daemon.NewClient(cfg.SocketPath)
	lbl := label.DetectLabel(cfg.Label)

	// Build port map: listen → backend
	portMap := make(map[int]int, len(cfg.ListenPorts))
	for _, lp := range cfg.ListenPorts {
		bp, ok := cfg.BackendPorts[lp]
		if !ok {
			var err error
			bp, err = client.Allocate(lp)
			if err != nil {
				return 1, fmt.Errorf("allocate port for %d: %w", lp, err)
			}
		}
		portMap[lp] = bp
	}

	// Register all backends
	for lp, bp := range portMap {
		if err := client.Register(lp, bp, lbl, os.Getpid()); err != nil {
			return 1, fmt.Errorf("register %d→%d: %w", lp, bp, err)
		}
	}
	defer func() {
		for lp, bp := range portMap {
			_ = client.Unregister(lp, bp)
		}
	}()

	result := portswap.Run(portswap.TracerConfig{
		PortMap: portMap,
		Command: cfg.Command,
		Args:    cfg.Args,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	})
	if result.Error != nil {
		return 1, result.Error
	}
	return result.ExitCode, nil
}

// ParseSwitchArgs parses switch command arguments: [port] <label>.
func ParseSwitchArgs(args []string) (listenPort int, label string, err error) {
	switch len(args) {
	case 1:
		return 0, args[0], nil
	case 2:
		lp, err := strconv.Atoi(args[0])
		if err != nil {
			return 0, "", fmt.Errorf("invalid port: %s", args[0])
		}
		return lp, args[1], nil
	default:
		return 0, "", fmt.Errorf("expected 1 or 2 arguments, got %d", len(args))
	}
}

// ParseExecPortArgs parses comma-separated port specs:
//   - "3000"            → listenPorts=[3000], backendPorts={}
//   - "3000,8995"       → listenPorts=[3000, 8995], backendPorts={}
//   - "3000:3001,8995:8996" → listenPorts=[3000, 8995], backendPorts={3000:3001, 8995:8996}
func ParseExecPortArgs(arg string) (listenPorts []int, backendPorts map[int]int, err error) {
	if arg == "" {
		return nil, nil, fmt.Errorf("empty port argument")
	}

	backendPorts = make(map[int]int)
	specs := strings.Split(arg, ",")
	for _, spec := range specs {
		parts := strings.Split(spec, ":")
		switch len(parts) {
		case 1:
			lp, err := strconv.Atoi(parts[0])
			if err != nil {
				return nil, nil, fmt.Errorf("invalid listen port: %s", parts[0])
			}
			listenPorts = append(listenPorts, lp)
		case 2:
			lp, err := strconv.Atoi(parts[0])
			if err != nil {
				return nil, nil, fmt.Errorf("invalid listen port: %s", parts[0])
			}
			bp, err := strconv.Atoi(parts[1])
			if err != nil {
				return nil, nil, fmt.Errorf("invalid backend port: %s", parts[1])
			}
			listenPorts = append(listenPorts, lp)
			backendPorts[lp] = bp
		default:
			return nil, nil, fmt.Errorf("invalid port spec: %s (expected <port> or <port>:<backend-port>)", spec)
		}
	}
	return listenPorts, backendPorts, nil
}

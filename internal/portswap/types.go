package portswap

import "io"

// TracerConfig configures the port interception tracer.
type TracerConfig struct {
	PortMap map[int]int // target port → replace port (e.g., {3000: 3001, 8995: 8996})
	Command string      // executable path
	Args    []string    // command arguments
	Stdout  io.Writer   // child stdout (nil = os.Stdout)
	Stderr  io.Writer   // child stderr (nil = os.Stderr)
}

// Result holds the outcome of a traced child process execution.
type Result struct {
	ExitCode int
	Error    error
}

//go:build !linux

package proxy

// PortOwner describes the process using a port.
type PortOwner struct {
	PID     int
	Command string
}

// FindPortOwner is not supported on non-Linux platforms.
func FindPortOwner(port int) *PortOwner {
	return nil
}

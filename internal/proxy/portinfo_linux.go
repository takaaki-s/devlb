//go:build linux

package proxy

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// PortOwner describes the process using a port.
type PortOwner struct {
	PID     int
	Command string
}

// FindPortOwner attempts to find the process listening on the given port
// by reading /proc/net/tcp and /proc/net/tcp6.
func FindPortOwner(port int) *PortOwner {
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		if owner := findPortOwnerInFile(path, port); owner != nil {
			return owner
		}
	}
	return nil
}

func findPortOwnerInFile(path string, port int) *PortOwner {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if i == 0 {
			continue // skip header
		}
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}

		// fields[1] is local_address in hex (e.g., "0100007F:1F90")
		localAddr := fields[1]
		parts := strings.Split(localAddr, ":")
		if len(parts) != 2 {
			continue
		}

		hexPort := parts[1]
		p, err := strconv.ParseUint(hexPort, 16, 16)
		if err != nil {
			continue
		}

		if int(p) != port {
			continue
		}

		// fields[3] is state: "0A" = LISTEN
		if fields[3] != "0A" {
			continue
		}

		// fields[9] is inode
		inode := fields[9]
		if inode == "0" {
			continue
		}

		pid := findPIDByInode(inode)
		if pid == 0 {
			return nil
		}

		cmd := readProcessComm(pid)
		return &PortOwner{PID: pid, Command: cmd}
	}
	return nil
}

func findPIDByInode(inode string) int {
	socketLink := fmt.Sprintf("socket:[%s]", inode)

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}

		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if link == socketLink {
				return pid
			}
		}
	}
	return 0
}

func readProcessComm(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

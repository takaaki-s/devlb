//go:build linux

package portswap

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func buildBindTest(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "bindtest")
	cmd := exec.Command("go", "build", "-o", binPath, "./testdata/bindtest")
	cmd.Dir = filepath.Join(findModuleRoot(t), "internal", "portswap")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build bindtest failed: %v\n%s", err, out)
	}
	return binPath
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find module root")
		}
		dir = parent
	}
}

func allocatePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func TestTracerRewritesTargetPort(t *testing.T) {
	binPath := buildBindTest(t)
	targetPort := allocatePort(t)
	replacePort := allocatePort(t)

	var stdout bytes.Buffer
	result := Run(TracerConfig{
		PortMap: map[int]int{targetPort: replacePort},
		Command: binPath,
		Args:    []string{strconv.Itoa(targetPort)},
		Stdout:  &stdout,
	})
	if result.Error != nil {
		t.Fatalf("tracer error: %v", result.Error)
	}
	if result.ExitCode != 0 {
		t.Fatalf("child exit code: %d", result.ExitCode)
	}

	// Child should have bound to replacePort, not targetPort
	actual := strings.TrimSpace(stdout.String())
	expected := fmt.Sprintf("127.0.0.1:%d", replacePort)
	if actual != expected {
		t.Errorf("child bound to %s, expected %s", actual, expected)
	}
}

func TestTracerPassesThroughOtherPorts(t *testing.T) {
	binPath := buildBindTest(t)
	targetPort := allocatePort(t)
	otherPort := allocatePort(t)
	replacePort := allocatePort(t)

	var stdout bytes.Buffer
	result := Run(TracerConfig{
		PortMap: map[int]int{targetPort: replacePort},
		Command: binPath,
		Args:    []string{strconv.Itoa(otherPort)}, // binds to a different port
		Stdout:  &stdout,
	})
	if result.Error != nil {
		t.Fatalf("tracer error: %v", result.Error)
	}

	// Child should have bound to otherPort (not rewritten)
	actual := strings.TrimSpace(stdout.String())
	expected := fmt.Sprintf("127.0.0.1:%d", otherPort)
	if actual != expected {
		t.Errorf("child bound to %s, expected %s (should not rewrite non-target port)", actual, expected)
	}
}

func TestTracerChildNormalExit(t *testing.T) {
	binPath := buildBindTest(t)
	port := allocatePort(t)

	result := Run(TracerConfig{
		PortMap: map[int]int{port: allocatePort(t)},
		Command: binPath,
		Args:    []string{strconv.Itoa(port)},
		Stdout:  &bytes.Buffer{},
	})
	if result.Error != nil {
		t.Fatalf("tracer error: %v", result.Error)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
}

func TestTracerRewritesMultiplePorts(t *testing.T) {
	binPath := buildBindTest(t)
	targetPort1 := allocatePort(t)
	replacePort1 := allocatePort(t)
	targetPort2 := allocatePort(t)
	replacePort2 := allocatePort(t)

	var stdout bytes.Buffer
	result := Run(TracerConfig{
		PortMap: map[int]int{targetPort1: replacePort1, targetPort2: replacePort2},
		Command: binPath,
		Args:    []string{fmt.Sprintf("%d,%d", targetPort1, targetPort2)},
		Stdout:  &stdout,
	})
	if result.Error != nil {
		t.Fatalf("tracer error: %v", result.Error)
	}
	if result.ExitCode != 0 {
		t.Fatalf("child exit code: %d", result.ExitCode)
	}

	// Child should have bound to replacePort1 and replacePort2
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines of output, got %d: %q", len(lines), stdout.String())
	}
	expected1 := fmt.Sprintf("127.0.0.1:%d", replacePort1)
	expected2 := fmt.Sprintf("127.0.0.1:%d", replacePort2)
	if lines[0] != expected1 {
		t.Errorf("port1: child bound to %s, expected %s", lines[0], expected1)
	}
	if lines[1] != expected2 {
		t.Errorf("port2: child bound to %s, expected %s", lines[1], expected2)
	}
}

func TestTracerChildAbnormalExit(t *testing.T) {
	// Use a command that exits with non-zero
	result := Run(TracerConfig{
		PortMap: map[int]int{9999: 9998},
		Command: "/bin/false",
		Args:    []string{},
		Stderr:  &bytes.Buffer{},
	})
	if result.Error != nil {
		t.Fatalf("tracer error: %v", result.Error)
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code for /bin/false")
	}
}

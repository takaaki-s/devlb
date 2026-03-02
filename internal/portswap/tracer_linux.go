//go:build linux

package portswap

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

const (
	_WALL           = 0x40000000 // __WALL: wait for any child
	_PTRACE_O_EXITKILL = 0x100000  // PTRACE_O_EXITKILL: kill tracee on tracer exit (Linux 3.8+)
)

// Run forks a child process, ptraces it, and rewrites bind() calls
// that target ports in PortMap to their replacement ports.
// Handles multi-threaded children (Go runtime, etc.) via PTRACE_O_TRACECLONE.
func Run(cfg TracerConfig) Result {
	// ptrace has thread affinity: all ptrace calls (ForkExec, Wait4,
	// PtraceSetOptions, PtraceGetRegs, etc.) must happen on the same OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	execPath, err := resolveCommand(cfg.Command)
	if err != nil {
		return Result{Error: fmt.Errorf("resolve command: %w", err)}
	}

	argv := append([]string{cfg.Command}, cfg.Args...)

	// Set up pipes for stdout/stderr if custom writers are provided
	var stdoutR, stdoutW, stderrR, stderrW *os.File
	stdoutFd := uintptr(1)
	stderrFd := uintptr(2)

	if cfg.Stdout != nil {
		stdoutR, stdoutW, err = os.Pipe()
		if err != nil {
			return Result{Error: fmt.Errorf("pipe: %w", err)}
		}
		defer stdoutR.Close()
		stdoutFd = stdoutW.Fd()
	}
	if cfg.Stderr != nil {
		stderrR, stderrW, err = os.Pipe()
		if err != nil {
			return Result{Error: fmt.Errorf("pipe: %w", err)}
		}
		defer stderrR.Close()
		stderrFd = stderrW.Fd()
	}

	pid, err := syscall.ForkExec(execPath, argv, &syscall.ProcAttr{
		Files: []uintptr{0, stdoutFd, stderrFd},
		Env:   os.Environ(),
		Sys: &syscall.SysProcAttr{
			Ptrace: true,
		},
	})
	if err != nil {
		return Result{Error: fmt.Errorf("forkexec: %w", err)}
	}

	// Close write ends in parent
	if stdoutW != nil {
		stdoutW.Close()
	}
	if stderrW != nil {
		stderrW.Close()
	}

	// Read pipes in background
	pipeDone := make(chan struct{}, 2)
	if stdoutR != nil {
		go func() {
			defer func() { pipeDone <- struct{}{} }()
			copyPipe(stdoutR, cfg.Stdout)
		}()
	} else {
		pipeDone <- struct{}{}
	}
	if stderrR != nil {
		go func() {
			defer func() { pipeDone <- struct{}{} }()
			copyPipe(stderrR, cfg.Stderr)
		}()
	} else {
		pipeDone <- struct{}{}
	}

	// Signal forwarding
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for sig := range sigCh {
			_ = syscall.Kill(pid, sig.(syscall.Signal))
		}
	}()
	defer signal.Stop(sigCh)

	result := traceLoop(pid, cfg.PortMap)

	// After traceLoop, all child threads should be dead.
	// Close pipe read ends to unblock copyPipe goroutines in case
	// any orphan threads still hold the write end open.
	if stdoutR != nil {
		stdoutR.Close()
	}
	if stderrR != nil {
		stderrR.Close()
	}

	<-pipeDone
	<-pipeDone

	return result
}

func traceLoop(leaderPid int, portMap map[int]int) Result {
	// Wait for initial ptrace stop (SIGTRAP at exec)
	var ws syscall.WaitStatus
	_, err := syscall.Wait4(leaderPid, &ws, 0, nil)
	if err != nil {
		return Result{Error: fmt.Errorf("wait4 initial: %w", err)}
	}
	if ws.Exited() {
		return Result{ExitCode: ws.ExitStatus()}
	}

	// Set options: trace syscalls + auto-trace cloned threads
	opts := syscall.PTRACE_O_TRACESYSGOOD |
		syscall.PTRACE_O_TRACECLONE |
		syscall.PTRACE_O_TRACEFORK |
		syscall.PTRACE_O_TRACEVFORK |
		_PTRACE_O_EXITKILL
	if err := syscall.PtraceSetOptions(leaderPid, opts); err != nil {
		return Result{Error: fmt.Errorf("ptrace set options: %w", err)}
	}

	if err := syscall.PtraceSyscall(leaderPid, 0); err != nil {
		return Result{Error: fmt.Errorf("ptrace syscall: %w", err)}
	}

	// Per-thread syscall enter/exit state
	threadInSyscall := make(map[int]bool)

	// Track leader exit separately: after the leader exits, we must
	// reap all remaining threads (Go runtime threads hold pipe fds open).
	var leaderResult *Result

	for {
		wpid, err := syscall.Wait4(-1, &ws, _WALL, nil)
		if err != nil {
			if err == syscall.ECHILD {
				// No more children — all threads reaped
				if leaderResult != nil {
					return *leaderResult
				}
				return Result{ExitCode: 0}
			}
			return Result{Error: fmt.Errorf("wait4: %w", err)}
		}

		if ws.Exited() {
			delete(threadInSyscall, wpid)
			if wpid == leaderPid {
				leaderResult = &Result{ExitCode: ws.ExitStatus()}
				// Kill remaining threads so they release fds
				for tid := range threadInSyscall {
					_ = syscall.Kill(tid, syscall.SIGKILL)
				}
			}
			continue
		}

		if ws.Signaled() {
			delete(threadInSyscall, wpid)
			if wpid == leaderPid {
				leaderResult = &Result{ExitCode: 128 + int(ws.Signal())}
				for tid := range threadInSyscall {
					_ = syscall.Kill(tid, syscall.SIGKILL)
				}
			}
			continue
		}

		if !ws.Stopped() {
			_ = syscall.PtraceSyscall(wpid, 0)
			continue
		}

		// If leader already exited, deliver SIGKILL and keep tracing
		// so Wait4 can reap them (PtraceDetach would orphan them).
		if leaderResult != nil {
			_ = syscall.PtraceCont(wpid, int(syscall.SIGKILL))
			continue
		}

		sig := ws.StopSignal()

		// Check for ptrace events (clone/fork/vfork)
		if sig == syscall.SIGTRAP {
			event := ws.TrapCause()
			if event == syscall.PTRACE_EVENT_CLONE ||
				event == syscall.PTRACE_EVENT_FORK ||
				event == syscall.PTRACE_EVENT_VFORK {
				_ = syscall.PtraceSyscall(wpid, 0)
				continue
			}
		}

		// Syscall stop: SIGTRAP | 0x80
		if sig == (syscall.SIGTRAP | 0x80) {
			inSyscall := threadInSyscall[wpid]
			if !inSyscall {
				handleSyscallEnter(wpid, portMap)
			}
			threadInSyscall[wpid] = !inSyscall
			_ = syscall.PtraceSyscall(wpid, 0)
			continue
		}

		// New thread stopped: set options and continue
		if sig == syscall.SIGSTOP {
			_ = syscall.PtraceSetOptions(wpid, opts)
			_ = syscall.PtraceSyscall(wpid, 0)
			continue
		}

		// Regular signal: deliver to child
		_ = syscall.PtraceSyscall(wpid, int(sig))
	}
}

func handleSyscallEnter(pid int, portMap map[int]int) {
	var regs syscall.PtraceRegs
	if err := syscall.PtraceGetRegs(pid, &regs); err != nil {
		return
	}

	if regs.Orig_rax != syscall.SYS_BIND {
		return
	}

	addrPtr := regs.Rsi

	buf := make([]byte, 8)
	n, err := syscall.PtracePeekText(pid, uintptr(addrPtr), buf)
	if err != nil || n < 4 {
		return
	}

	family := binary.LittleEndian.Uint16(buf[0:2])
	if family != syscall.AF_INET && family != syscall.AF_INET6 {
		return
	}

	port := binary.BigEndian.Uint16(buf[2:4])
	replacePort, ok := portMap[int(port)]
	if !ok {
		return
	}

	binary.BigEndian.PutUint16(buf[2:4], uint16(replacePort))
	_, _ = syscall.PtracePokeText(pid, uintptr(addrPtr), buf)
}

func copyPipe(src *os.File, dst interface{ Write([]byte) (int, error) }) {
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			_, _ = dst.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func resolveCommand(name string) (string, error) {
	if len(name) > 0 && name[0] == '/' {
		return name, nil
	}
	if len(name) > 1 && name[0] == '.' && name[1] == '/' {
		return name, nil
	}
	pathEnv := os.Getenv("PATH")
	for _, dir := range splitPath(pathEnv) {
		full := dir + "/" + name
		if _, err := os.Stat(full); err == nil {
			return full, nil
		}
	}
	return "", fmt.Errorf("command not found: %s", name)
}

func splitPath(path string) []string {
	var result []string
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == ':' {
			if i > start {
				result = append(result, path[start:i])
			}
			start = i + 1
		}
	}
	if start < len(path) {
		result = append(result, path[start:])
	}
	return result
}

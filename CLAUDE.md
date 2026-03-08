# devlb -- Developer Guide

## Project Overview

devlb is a local development TCP reverse proxy for multi-worktree workflows, written in Go 1.24+.
It sits on default ports and transparently routes traffic between multiple backend instances,
solving port conflict issues when running the same service on different worktrees.

## Architecture

Layered architecture -- dependencies flow strictly downward. Violations are caught by `architecture_test.go`.

```
Layer 0 (leaf):   model, label, config, proxy, portswap  ← no internal deps
Layer 1:          exec   → daemon, label, portswap
                  daemon → config, proxy
                  tui    → daemon
Layer 2 (root):   cmd/devlb/cmd → config, daemon, exec, label, tui
```

### Package Dependency Rules

| Package          | Allowed internal imports        |
|------------------|---------------------------------|
| internal/model   | NONE                            |
| internal/label   | NONE                            |
| internal/config  | NONE                            |
| internal/proxy   | NONE                            |
| internal/portswap| NONE                            |
| internal/exec    | daemon, label, portswap         |
| internal/daemon  | config, proxy                   |
| internal/tui     | daemon                          |

### Package Responsibilities

- **cmd/devlb/cmd/** -- CLI layer (Cobra). Wires packages together. No business logic.
- **internal/daemon/** -- Unix socket server, JSON-over-socket protocol, client.
- **internal/proxy/** -- TCP listener, health checks, metrics, HTTP 503 fallback.
- **internal/portswap/** -- ptrace-based bind() interception. Linux-only (stub on other OS).
- **internal/config/** -- YAML config parsing, state persistence, file watcher.
- **internal/model/** -- Shared data types. Zero dependencies.
- **internal/label/** -- Git branch detection and random label generation.
- **internal/exec/** -- Process execution wrapper (port allocation + registration + portswap).
- **internal/tui/** -- bubbletea + lipgloss interactive dashboard.

## Build & Test

```bash
make build       # go build -o bin/devlb ./cmd/devlb
make test        # go test ./... (CI runs with -race)
make e2e         # scripts/e2e-test.sh (requires built binary, Linux for ptrace tests)
make lint        # golangci-lint v1.64.8
make fmt         # gofmt -l -w .
```

Always run `make test` and `make lint` before committing.

## Coding Conventions

- Standard library test style only -- no testify or other test frameworks.
- Table-driven tests where applicable.
- `t.TempDir()` for temp files in tests.
- Wrap errors with `fmt.Errorf("context: %w", err)`. No panics in library code.
- Build tags via `_linux.go` / `_stub.go` suffixes for platform-specific code.
- Commit messages: conventional commits (`feat:`, `fix:`, `test:`, `docs:`, `refactor:`).

## Do NOT

- Add internal dependencies to leaf packages (model, label, config, proxy, portswap).
- Use testify or other test frameworks.
- Add `go:generate` or code generation.
- Modify the Unix socket protocol without updating both `daemon/server.go` and `daemon/client.go`.
- Add network-calling tests (no external HTTP calls in unit tests).
- Commit `.env` files or credentials.
- Remove `_stub.go` fallbacks for Linux-only features.

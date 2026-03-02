# devlb

Local development TCP reverse proxy for multi-worktree workflows.

When running multiple git worktrees for the same microservice, port conflicts are inevitable. **devlb** sits on the default port, intercepts each process's `bind()` via ptrace, and transparently routes traffic — no config changes, no environment variable juggling.

```
┌─────────── devlb daemon (listens on default ports) ──────────┐
│  :3000  → :3001 (worktree-a/api)    ● active                 │
│           :3002 (worktree-b/api)    ○ standby                 │
│  :8995  → :8996 (worktree-a/auth)   ● active                 │
└───────────────────────────────────────────────────────────────┘
```

## Features

- **Automatic port interception** — `devlb exec` uses ptrace to rewrite `bind()` syscalls, assigning a free port to each process while the proxy holds the original (Linux only)
- **Instant switching** — `devlb switch worktree-b` reroutes traffic without restarting any process
- **Health checks** — periodic TCP probes with automatic failover to healthy backends
- **Connection metrics** — track active connections, bytes in/out per backend
- **HTTP 503 error page** — returns a helpful 503 when no backend is available (non-HTTP traffic passes through cleanly)
- **Config hot reload** — add/remove services by editing `devlb.yaml`; changes are picked up automatically
- **TUI dashboard** — real-time interactive terminal UI for monitoring and switching

## Install

```bash
go install github.com/takaaki-s/devlb/cmd/devlb@latest
```

Or build from source:

```bash
make build          # → bin/devlb
make install        # → $GOPATH/bin/devlb
```

Requires **Go 1.24+**. The proxy daemon works on any platform, but `devlb exec` (ptrace-based port interception) is **Linux-only**.

## Quick Start

```bash
# 1. Generate config
devlb init

# 2. Edit ~/.devlb/devlb.yaml
cat ~/.devlb/devlb.yaml
# services:
#   - name: api
#     port: 3000
#   - name: auth
#     port: 8995

# 3. Start daemon
devlb start

# 4. Run your service (port 3000 is automatically intercepted)
devlb exec 3000 -- go run ./cmd/api

# 5. Run another worktree's service on the same port
devlb exec 3000 -- go run ./cmd/api    # from worktree-b

# 6. Switch traffic
devlb switch worktree-b

# 7. Check status
devlb status
devlb status -v    # verbose: show metrics

# 8. Interactive dashboard
devlb tui
```

## Commands

| Command | Description |
|---------|-------------|
| `devlb init` | Generate `~/.devlb/devlb.yaml` config template |
| `devlb start` | Start the daemon in the background |
| `devlb stop` | Stop the daemon |
| `devlb status [-v]` | Show routing table (verbose: metrics) |
| `devlb route <port> <backend> [--label NAME]` | Manually register a backend |
| `devlb unroute <port> <backend>` | Remove a backend |
| `devlb switch [port] <label>` | Switch active backend by label |
| `devlb exec <port>[,...] -- <cmd> [args]` | Run command with port interception (Linux only) |
| `devlb tui` | Interactive terminal dashboard |

## Configuration

`~/.devlb/devlb.yaml`:

```yaml
services:
  - name: api
    port: 3000
  - name: auth
    port: 8995

# Optional: health checking
health_check:
  enabled: true
  interval: "1s"
  timeout: "500ms"
  unhealthy_after: 3
```

Services can be added or removed while the daemon is running — changes are detected automatically.

## TUI Dashboard

`devlb tui` provides a real-time terminal dashboard:

```
 devlb dashboard                                    auto-refresh: 1s

  PORT    BACKEND   LABEL          STATUS           CONNS       IN      OUT
  :3000   :3001     worktree-a     ● active             5    1.2M    567K
          :3002     worktree-b     ○ standby             0      0B      0B
  :8995   :8996     main           ● active              2    823K    1.4M
          :8997     feature-x      ✗ unhealthy           0      0B      0B

  ↑↓ select  s switch  r refresh  q quit
```

## How Port Interception Works

When you run `devlb exec 3000 -- your-server`, devlb:

1. Launches the child process under ptrace
2. Intercepts `bind()` syscalls targeting port 3000
3. Rewrites the port argument to an ephemeral port (e.g. 3001)
4. Registers the new port as a backend with the daemon
5. The daemon proxies `:3000 → :3001` transparently

This works with any language/runtime — Go, Ruby, Node.js, Python, etc. Note that `exec` requires **Linux** (ptrace is a Linux-specific API). On other platforms, use `devlb route` to register backends manually.

## Architecture

```
cmd/devlb/cmd/       CLI (cobra)
internal/daemon/      Unix socket server, JSON protocol, client
internal/proxy/       TCP listener, health check, metrics, HTTP 503
internal/portswap/    ptrace bind() interception
internal/config/      YAML config, state persistence, file watcher
internal/tui/         bubbletea + lipgloss dashboard
internal/exec/        Process execution helpers
internal/label/       Git branch label detection
internal/model/       Shared data types
```

## Development

```bash
make test       # unit tests
make e2e        # end-to-end tests (32 scenarios)
make lint       # golangci-lint
make fmt        # gofmt
```

## License

MIT

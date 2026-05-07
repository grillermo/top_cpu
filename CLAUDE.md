# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
go build -o top_cpu .              # build
go test ./...                      # all tests
go test ./... -run TestParsePS -v  # single test pattern
./top_cpu                          # TUI
./top_cpu --daemon                 # background recorder (writes SQLite)
```

## Architecture

macOS-only TUI app. Two modes:

- **TUI** (default): polls `ps -eo %cpu,pid,args` every 2s, accumulates session-cumulative CPU per process, displays top 60. Two tabs: **List** (live cumulative) and **Timeline** (historical chart from SQLite).
- **Daemon** (`--daemon`): background recorder. Polls `ps` every 30s, inserts samples into SQLite, purges rows older than 30 days hourly. No TUI.

File responsibilities:

| File | Responsibility |
|------|---------------|
| `main.go` | Entry: parse `--daemon` flag, branch to daemon or TUI |
| `model.go` | Bubble Tea `Init`/`Update`/`View`, tab bar, list-tab handlers |
| `poller.go` | `parsePS` (pure fn), `fetchProcesses`, `pollCmd`, types |
| `ports.go` | `parseLsof` (pure fn), `fetchListeningPorts`, `formatPorts` |
| `exclusions.go` | `loadExclusions`, `appendExclusion` — file I/O only |
| `daemon.go` | `runDaemon` loop: poll → insert → purge; signal-aware |
| `store.go` | SQLite (modernc.org/sqlite): schema, insert, `QueryTopN`, purge |
| `timeline.go` | Timeline tab state, asciigraph rendering, log-transform, legend |

**Live data flow (List tab):** `pollCmd` → `tea.Tick` → `fetchProcesses()` (runs `ps` and `lsof -iTCP -sTCP:LISTEN` concurrently, joins on PID) → `tickMsg` → `Update` accumulates CPU into `model.cumulative` and rebuilds `model.latestPorts` (ports always reflect newest tick) → `buildDisplayList` filters (name OR port substring) / sorts / caps. Rendered as `idx  cpu%  ports  name (pid)`.

**Historical data flow (Timeline tab):** Daemon writes `cpu_samples(process_name, cpu_pct, recorded_at)` to `~/.local/share/top_cpu/top_cpu.db`. TUI calls `store.QueryTopN(start, end, n=10, buckets=chartWidth)` which returns top 10 processes by `MAX(cpu_pct)` in window, each pre-bucketed by SQLite `GROUP BY (recorded_at - start) / bucketWidth`. Values log-transformed (`log10(cpu+1)`) before passing to `asciigraph.PlotMany`.

**Process identity:** tracked by PID, displayed as `name (pid)`. `processDisplayName` uses `filepath.Base(argv[0])` from `args` column (not `comm`) so `exec -a` aliases appear correctly. Bracketed kernel thread names like `[kworker]` are preserved with PID appended. Daemon reuses the same `parsePS` so names match across tabs.

**Exclusions:** persisted to `./top_cpu_excluded.txt` (relative to CWD at launch). `appendExclusion` appends; `loadExclusions` deduplicates via map. Excluded processes are filtered in `buildDisplayList`, not at ingestion. Exclusions apply to List tab only — Timeline tab queries SQLite directly.

**TUI state machine:** three orthogonal modes — tab (`activeTab` int, ← / → switch), filter (`F4` activates, `Esc` clears), selection (`Enter` selects, `F1` excludes, `Esc` deselects). `Update` dispatches to `updateList` / `updateFiltering` / `updateTimeline` after handling tab switching and global keys. Tab switching is gated by `!filtering`.

**Timeline tab:** time range `Today` / `This Week` / `All Time`, cycle with `t`. Manual refresh `r`. Auto-refreshes every 60s via `timelineRefreshTickMsg` while active. Empty state when DB missing prompts `top_cpu --daemon`.

**Tests cover pure functions and store:** `parsePS`, `buildDisplayList`, `loadExclusions`/`appendExclusion`, `clamp`, `Store.Insert`/`QueryTopN`/`Purge`. No tests for `Update`/`View`/`renderTimeline` (Bubble Tea integration, asciigraph rendering).

## Development flow

* After each change build the app

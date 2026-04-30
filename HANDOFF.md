# Handoff: top_cpu TUI

**Generated**: 2026-04-30
**Branch**: main
**Status**: In Progress — core feature complete, user actively iterating

## Goal

Go TUI app (Bubble Tea) that replaces `top_cpu.sh`: aggregates cumulative CPU usage per process instance (by PID), refreshes every 2s, lets user filter and permanently exclude noisy processes.

## Completed

- [x] Go module with bubbletea v1.3.10 + lipgloss v1.1.0
- [x] `exclusions.go` — load/append `./top_cpu_excluded.txt` (newline-separated, deduped)
- [x] `poller.go` — `ps -eo %cpu,pid,args` every 2s, name = `filepath.Base(argv[0]) (pid)`
- [x] `model.go` — full Bubble Tea model with cumulative aggregation, filter, selection, exclusion, scrolling viewport
- [x] `main.go` — entry point with alt screen
- [x] All 16 tests passing
- [x] README with macOS setup/build/run instructions

## Not Yet Done

No explicit backlog — user has been making incremental changes. Likely next requests involve display polish or more filtering options.

## Failed Approaches (Don't Repeat These)

- **`ps -eo %cpu,comm`** — groups all instances of same binary under one name. Switched to `ps -eo %cpu,pid,args` so each PID appears separately.
- **`os.IsNotExist(err)`** — deprecated since Go 1.13. Use `errors.Is(err, os.ErrNotExist)`.
- **Direct mutation of `m.excluded` map in value receiver** — maps are shared across value copies in Go. Fixed with copy-on-write in `KeyDelete` handler.
- **Byte-slicing backspace** (`m.filter[:len-1]`) — breaks multibyte UTF-8. Fixed with `[]rune` conversion.

## Key Decisions

| Decision | Rationale |
|----------|-----------|
| Track by `name (pid)` not just `comm` | User has multiple python instances, wants them separate |
| `filepath.Base(argv[0])` from `args` column | `exec -a process_name python` sets argv[0]; `comm` shows binary name, not custom name |
| Cumulative CPU (never reset) | Shows long-term hogs, not just current spike — mirrors original shell script |
| Append-only exclusions file | Simple, no data loss; dedup on load |
| Enter to select → F1 to exclude | List re-sorts every 2s; selecting locks a name so user doesn't exclude wrong process |
| F4 to enter filter mode | Prevents accidental filtering when navigating |
| Value receivers throughout model | Bubble Tea convention; copy-on-write for maps |

## Current State

**Working**: Full TUI — live CPU list, per-PID tracking, filter (F4), selection (Enter), exclusion (F1), scrolling viewport, persistent exclusions file.

**Broken**: Nothing known.

**Uncommitted Changes**: None — all committed. Two untracked items: `docs/` (design spec + plan) and `top_cpu.sh` (original shell script reference).

## Files to Know

| File | Why It Matters |
|------|----------------|
| `model.go` | All TUI state and logic — most likely to change |
| `poller.go` | ps command + parsePS — change here to alter what data is collected |
| `exclusions.go` | File I/O only — load/append `top_cpu_excluded.txt` |
| `main.go` | Thin entry point — loads exclusions, starts Bubble Tea with alt screen |
| `top_cpu_excluded.txt` | Runtime file (gitignored) — newline-separated excluded `name (pid)` strings |

## Code Context

**Model struct:**
```go
type model struct {
    cumulative   map[string]float64    // key: "name (pid)", running CPU total
    excluded     map[string]struct{}   // key: "name (pid)", loaded from file
    excludedPath string                // "./top_cpu_excluded.txt"
    filter       string                // current filter text (empty when not filtering)
    filtering    bool                  // true when F4 filter mode active
    cursor       int                   // index into displayList
    offset       int                   // first visible row (scroll offset)
    height       int                   // terminal height from WindowSizeMsg
    selected     string                // locked process name for F1 exclusion (empty = none)
    displayList  []procEntry           // filtered+sorted+capped slice, rebuilt on each change
}
```

**Key types:**
```go
type procEntry struct { name string; cpu float64 }
type rawEntry  struct { cpu float64; name string }  // from ps parse
type tickMsg   struct { entries []rawEntry }         // sent every 2s
```

**parsePS signature (poller.go):**
```go
// input: raw output of `ps -eo %cpu,pid,args`
// output: entries with name = "basename(argv[0]) (pid)"
func parsePS(output string) []rawEntry
```

**Keyboard map (model.go Update):**
```
KeyCtrlC / q (no filter) → quit
KeyUp / KeyDown           → move cursor + sync scroll offset
KeyEnter                  → select process at cursor (locks name into m.selected)
KeyF1 (when selected)     → exclude m.selected, write to file, clear selection
KeyF4                     → enter filter mode
KeyBackspace (filtering)  → remove last rune from filter
KeyEsc                    → deselect if selected; else exit filter mode + clear filter
KeyRunes (filtering)      → append to filter, reset cursor+offset to 0
```

**viewHeight:**
```go
func (m model) viewHeight() int {
    const fixedRows = 4 // header + top divider + bottom divider + help
    h := m.height - fixedRows
    if h < 1 { return 1 }
    return h
}
```

**buildDisplayList logic:**
1. Iterate `m.cumulative`, skip if in `m.excluded`
2. Skip if filter non-empty and name doesn't contain filter (case-insensitive)
3. Sort descending by cpu
4. Cap at `displayLimit = 60`

## Resume Instructions

1. Build: `go build -o top_cpu .`
2. Run: `./top_cpu`
3. Verify list populates within 2s with `name (pid)` format
4. Test selection: navigate with ↑↓, press Enter — row turns yellow with `★`, header shows `selected: name (pid)`
5. Test F1 exclude: with process selected, press F1 — process disappears, `./top_cpu_excluded.txt` created/appended
6. Test filter: press F4, type partial name — list narrows live; Esc exits filter
7. Restart app — excluded processes absent from list (loaded from file)

Run tests: `go test ./... -v` — expect 16 passing.

## Warnings

- `top_cpu_excluded.txt` stores `name (pid)` strings. If you change the name format in `poller.go`, existing exclusion entries won't match and excluded processes will reappear.
- `m.cumulative` is mutated in place (not copy-on-write) — only `m.excluded` uses CoW. Safe because Bubble Tea serializes updates and no code holds prior model copies.
- The `ps -eo %cpu,pid,args` `args` column can be empty for kernel threads — those lines get skipped by the `len(fields) < 3` guard in `parsePS`.
- `exec -a custom_name python` sets argv[0] = `custom_name`. This shows as `custom_name (pid)`. Full path args like `/usr/bin/python3 script.py` show as `python3 (pid)` via `filepath.Base`.

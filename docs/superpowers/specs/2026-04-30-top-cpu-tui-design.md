# top_cpu TUI — Design Spec
Date: 2026-04-30

## Overview

Go TUI application replicating `top_cpu.sh` behavior: aggregate cumulative CPU usage per process name via `ps`, refresh every 2s, display top 60 sorted descending. Adds interactive filtering and process exclusion via Bubble Tea.

## Architecture

Three layers:

**Model (Bubble Tea state)**
- `cumulative map[string]float64` — running CPU totals, never reset
- `excluded map[string]struct{}` — process names to hide, loaded from file at startup
- `filter string` — live search string, narrows display list
- `cursor int` — index into displayList for keyboard navigation
- `displayList []ProcEntry` — sorted, filtered, capped-at-60 view; rebuilt on every tick and every filter/exclude change

**Poller (tea.Cmd)**
- `tea.Tick(2 * time.Second)` fires a goroutine that runs `ps -eo %cpu,comm`
- Returns `tickMsg{entries []rawEntry}` to the update loop
- On receipt: merge into cumulative map, rebuild displayList

**File I/O**
- Path: `./top_cpu_excluded.txt` (same directory as binary)
- Format: newline-separated process names, one per line
- Load on startup; deduplicate entries into `excluded` map
- On each exclusion: append name + newline (no full rewrite)
- File created empty if missing; file errors logged to stderr, program continues

## Display Layout

```
top_cpu  [filter: ______]   excluded: 12
──────────────────────────────────────────
  1   45.2%  firefox
▶ 2   32.1%  node
  3   18.4%  postgres
  ...
──────────────────────────────────────────
↑↓ navigate  type to filter  Del exclude  q quit
```

- Filter box always visible (empty by default)
- Cursor arrow `▶` marks selected row
- Excluded count shown in header

## Key Bindings

| Key | Action |
|-----|--------|
| ↑ / ↓ | Move cursor |
| Any printable char | Append to filter; list narrows live |
| Backspace | Remove last filter char |
| Delete | Exclude process at cursor; append to file; rebuild list |
| Esc | Clear filter |
| q / Ctrl+C | Quit |

## Data Flow

**Aggregation (per tick):**
1. Parse `ps` output: each line → `(cpu float64, name string)`
2. `cumulative[name] += cpu`
3. Rebuild displayList:
   - Exclude names in `excluded` map
   - If filter non-empty, skip names not containing filter string (case-insensitive)
   - Sort by `cumulative[name]` descending
   - Cap at 60 entries

**Exclude action:**
1. `name = displayList[cursor].name`
2. `excluded[name] = struct{}{}`
3. Append `name + "\n"` to `./top_cpu_excluded.txt`
4. Rebuild displayList immediately
5. `cursor = min(cursor, len(displayList)-1)`

## Error Handling

- `ps` exec failure → skip tick, retain existing cumulative data
- File open/write error → log to stderr, continue (non-fatal)
- Empty `ps` output → skip tick

## File Structure

```
top_cpu/
  main.go              — entry point, loads exclusions, starts Bubble Tea
  model.go             — Model struct, Init/Update/View
  poller.go            — ps execution, tickMsg type
  exclusions.go        — file load/append logic
  top_cpu_excluded.txt — runtime exclusions (gitignored)
  go.mod
  go.sum
```

## Dependencies

- `github.com/charmbracelet/bubbletea` — TUI framework
- `github.com/charmbracelet/lipgloss` — styling (column alignment, cursor highlight)
- Standard library only for everything else

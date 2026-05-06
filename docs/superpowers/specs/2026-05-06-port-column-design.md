# Port Column on List Tab

## Goal

Show the listening TCP ports each process is bound to as a new column on the List tab, between CPU% and the process name. Make the F4 filter match against port numbers in addition to names.

## Scope

- macOS only (matches the rest of the app).
- TCP listening sockets only. No UDP. No established connections.
- All ports for a process, comma-joined, fixed-width column with ellipsis truncation.
- F4 filter substring-matches name OR any port number string.

Out of scope: changing the Timeline tab, port history persistence, sudo escalation for ports owned by other users (those simply won't appear).

## Architecture

### New file: `ports.go`

Two functions:

- `parseLsof(out string) map[string][]int` — pure parser for `lsof -F pn` record format. Records are `p<pid>\n` followed by one or more `n<host>:<port>\n` lines. Returns `pid (string) → sorted, deduplicated ports`.
- `fetchListeningPorts() map[string][]int` — runs `lsof -iTCP -sTCP:LISTEN -nP -P -F pn`, calls `parseLsof`. On error: log to stderr at most once per process lifetime (sync.Once), return empty map.

Parser handles:
- IPv4 `n127.0.0.1:3000` and IPv6 `n[::1]:3000` — port is the substring after the last `:`.
- Multiple `n` lines per `p` record — append all ports.
- Malformed records (no port, non-numeric) — skipped silently.
- Empty input — empty map.

### Modified: `poller.go`

`rawEntry` gains a field:

```go
type rawEntry struct {
    cpu   float64
    name  string
    pid   string
    ports []int
}
```

`fetchProcesses` runs `ps` and `lsof` concurrently via two goroutines and a `sync.WaitGroup`. After both return, it joins: for each `rawEntry` from ps, look up `ports[pid]` and assign. `parsePS` gains a `pid` field on the returned entries (already extracted internally; just expose it).

### Modified: `model.go`

`procEntry` gains `ports []int`.

`model` gains `latestPorts map[string][]int` keyed by display name. Updated wholesale on each `tickMsg` from the latest tick's `rawEntry` slice (CPU is cumulative; ports always reflect "now"). Stale entries naturally drop because the map is rebuilt each tick.

`buildDisplayList`:
- Reads `m.latestPorts[name]` to populate `procEntry.ports`.
- Filter check becomes: name matches lowercase substring OR any port's decimal string contains the filter substring.

### Render

`model.go:288` line format becomes:

```go
fmt.Sprintf("%3d  %6.1f%%  %-20s  %s", i+1, p.cpu, formatPorts(p.ports), p.name)
```

`formatPorts([]int) string`:
- Empty → `""` (column renders as 20 spaces).
- Comma-join all ports as decimal.
- If wider than 20, truncate to 19 chars and append `…`.

No header row added (matches existing headerless style).

### Error handling

- `lsof` not installed / errors out → empty map, stderr warning once, list still renders without ports.
- `lsof` slow → blocks the tick (acceptable; same as ps today). Bounded by lsof's own runtime, no internal timeout.
- Other-user processes whose ports require sudo simply don't appear. Acceptable.

## Data flow

```
tea.Tick (2s)
  → fetchProcesses()
       ├─ goroutine: ps -eo %cpu,pid,args  → parsePS  → []rawEntry (no ports)
       └─ goroutine: lsof -iTCP -sTCP:LISTEN -nP -P -F pn  → parseLsof  → map[pid][]int
     join: attach ports to entries by pid
  → tickMsg{entries}
  → Update: cumulative[name] += cpu;  latestPorts[name] = entry.ports
  → buildDisplayList: filter name|port, sort, cap, project ports
  → View: render port column
```

## Testing

New tests:

- `TestParseLsof` — fixtures:
  - single pid, single port
  - single pid, multiple ports (verify sorted, deduplicated)
  - multiple pids
  - IPv6 address `n[::1]:3000`
  - malformed record (no `n` line, non-numeric port) — skipped
  - empty input → empty map
- `TestBuildDisplayList_FilterByPort` — name `node (123)` with ports `[3000, 8080]`:
  - filter `"300"` → included
  - filter `"8080"` → included
  - filter `"node"` → included
  - filter `"9999"` → excluded
- `TestFormatPorts` — empty, single, multi, truncation past 20 chars.

Existing test updates:

- `TestParsePS` — assert `pid` field is populated alongside `name` and `cpu`.

No tests for `fetchListeningPorts` itself (shells out), `Update` (Bubble Tea integration), or `View` (rendering) — same policy as the rest of the codebase.

## File touch list

| File | Change |
|------|--------|
| `ports.go` | new — `parseLsof`, `fetchListeningPorts`, `formatPorts` |
| `ports_test.go` | new — `TestParseLsof`, `TestFormatPorts` |
| `poller.go` | `rawEntry` gains `pid`, `ports`; `fetchProcesses` parallel ps+lsof; `parsePS` exposes pid |
| `poller_test.go` | `TestParsePS` asserts pid |
| `model.go` | `procEntry` gains `ports`; `model.latestPorts`; `buildDisplayList` filter + projection; render line format |
| `model_test.go` | `TestBuildDisplayList_FilterByPort` |
| `CLAUDE.md` | add `ports.go` to file responsibilities table; mention port column in List flow |

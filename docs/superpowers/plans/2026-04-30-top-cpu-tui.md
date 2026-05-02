# top_cpu TUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go TUI app using Bubble Tea that aggregates cumulative CPU usage per process, displays top 60, and lets the user filter/exclude processes persistently.

**Architecture:** Bubble Tea model holds cumulative CPU map, excluded set, filter string, cursor, and display list. A `tea.Tick` command polls `ps -eo %cpu,comm` every 2s and sends results as a message. Business logic (parsing, filtering, sorting) is in pure functions for testability.

**Tech Stack:** Go 1.21+, github.com/charmbracelet/bubbletea, github.com/charmbracelet/lipgloss

---

## File Structure

| File | Responsibility |
|------|---------------|
| `main.go` | Entry point: load exclusions, create model, start Bubble Tea |
| `model.go` | Model struct, Init/Update/View, buildDisplayList, clamp |
| `poller.go` | rawEntry/tickMsg types, parsePS (pure), fetchProcesses, pollCmd |
| `exclusions.go` | loadExclusions, appendExclusion — file I/O only |
| `poller_test.go` | Tests for parsePS |
| `exclusions_test.go` | Tests for loadExclusions, appendExclusion |
| `model_test.go` | Tests for buildDisplayList |

---

### Task 1: Initialize Go module and install dependencies

**Files:**
- Create: `go.mod`, `go.sum`

- [ ] **Step 1: Initialize module**

```bash
cd /Users/grillermo/c/top_cpu
go mod init top_cpu
```

Expected output: creates `go.mod` with `module top_cpu`

- [ ] **Step 2: Add dependencies**

```bash
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/lipgloss@latest
```

Expected: `go.mod` and `go.sum` updated with bubbletea and lipgloss versions.

- [ ] **Step 3: Verify**

```bash
grep charmbracelet go.mod
```

Expected: two lines containing `charmbracelet/bubbletea` and `charmbracelet/lipgloss`.

- [ ] **Step 4: Commit**

```bash
git init
git add go.mod go.sum
git commit -m "chore: initialize Go module with bubbletea and lipgloss"
```

---

### Task 2: Implement exclusions.go with TDD

**Files:**
- Create: `exclusions.go`
- Create: `exclusions_test.go`

- [ ] **Step 1: Write the failing tests**

Create `exclusions_test.go`:

```go
package main

import (
	"os"
	"testing"
)

func TestLoadExclusionsFileNotExist(t *testing.T) {
	excluded, err := loadExclusions("/tmp/top_cpu_nonexistent_12345.txt")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if len(excluded) != 0 {
		t.Errorf("expected empty map, got %d entries", len(excluded))
	}
}

func TestLoadAndAppendExclusions(t *testing.T) {
	path := t.TempDir() + "/excluded.txt"

	excluded, err := loadExclusions(path)
	if err != nil {
		t.Fatalf("loadExclusions on empty: %v", err)
	}
	if len(excluded) != 0 {
		t.Errorf("expected empty, got %d", len(excluded))
	}

	if err := appendExclusion(path, "firefox"); err != nil {
		t.Fatalf("appendExclusion firefox: %v", err)
	}
	if err := appendExclusion(path, "node"); err != nil {
		t.Fatalf("appendExclusion node: %v", err)
	}

	excluded, err = loadExclusions(path)
	if err != nil {
		t.Fatalf("loadExclusions after append: %v", err)
	}
	if _, ok := excluded["firefox"]; !ok {
		t.Error("expected firefox in excluded")
	}
	if _, ok := excluded["node"]; !ok {
		t.Error("expected node in excluded")
	}
	if len(excluded) != 2 {
		t.Errorf("expected 2 entries, got %d", len(excluded))
	}
}

func TestLoadExclusionsDeduplicate(t *testing.T) {
	path := t.TempDir() + "/excluded.txt"
	os.WriteFile(path, []byte("firefox\nnode\nfirefox\n"), 0644)

	excluded, err := loadExclusions(path)
	if err != nil {
		t.Fatalf("loadExclusions: %v", err)
	}
	if len(excluded) != 2 {
		t.Errorf("expected 2 unique entries, got %d", len(excluded))
	}
}

func TestLoadExclusionsIgnoresBlankLines(t *testing.T) {
	path := t.TempDir() + "/excluded.txt"
	os.WriteFile(path, []byte("firefox\n\nnode\n\n"), 0644)

	excluded, err := loadExclusions(path)
	if err != nil {
		t.Fatalf("loadExclusions: %v", err)
	}
	if len(excluded) != 2 {
		t.Errorf("expected 2 entries, got %d", len(excluded))
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
go test ./... -run TestLoad -v
```

Expected: compile error — `loadExclusions` and `appendExclusion` undefined.

- [ ] **Step 3: Implement exclusions.go**

Create `exclusions.go`:

```go
package main

import (
	"bufio"
	"os"
	"strings"
)

func loadExclusions(path string) (map[string]struct{}, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]struct{}), nil
		}
		return nil, err
	}
	defer f.Close()

	excluded := make(map[string]struct{})
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name != "" {
			excluded[name] = struct{}{}
		}
	}
	return excluded, scanner.Err()
}

func appendExclusion(path string, name string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(name + "\n")
	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./... -run TestLoad -v
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add exclusions.go exclusions_test.go
git commit -m "feat: add exclusions file load/append with tests"
```

---

### Task 3: Implement poller.go with TDD

**Files:**
- Create: `poller.go`
- Create: `poller_test.go`

- [ ] **Step 1: Write the failing tests**

Create `poller_test.go`:

```go
package main

import (
	"testing"
)

func TestParsePSNormalOutput(t *testing.T) {
	input := `%CPU COMM
  1.5 firefox
 23.4 node
  0.0 ps`

	entries := parsePS(input)

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].cpu != 1.5 || entries[0].name != "firefox" {
		t.Errorf("entry 0: got cpu=%v name=%q, want cpu=1.5 name=firefox", entries[0].cpu, entries[0].name)
	}
	if entries[1].cpu != 23.4 || entries[1].name != "node" {
		t.Errorf("entry 1: got cpu=%v name=%q, want cpu=23.4 name=node", entries[1].cpu, entries[1].name)
	}
	if entries[2].cpu != 0.0 || entries[2].name != "ps" {
		t.Errorf("entry 2: got cpu=%v name=%q, want cpu=0.0 name=ps", entries[2].cpu, entries[2].name)
	}
}

func TestParsePSHeaderOnly(t *testing.T) {
	entries := parsePS("%CPU COMM\n")
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for header-only output, got %d", len(entries))
	}
}

func TestParsePSEmpty(t *testing.T) {
	entries := parsePS("")
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty output, got %d", len(entries))
	}
}

func TestParsePSSkipsMalformedLines(t *testing.T) {
	input := `%CPU COMM
  notanumber firefox
 10.0 node`

	entries := parsePS(input)
	if len(entries) != 1 {
		t.Fatalf("expected 1 valid entry, got %d", len(entries))
	}
	if entries[0].name != "node" {
		t.Errorf("expected node, got %q", entries[0].name)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
go test ./... -run TestParsePS -v
```

Expected: compile error — `parsePS` undefined.

- [ ] **Step 3: Implement poller.go**

Create `poller.go`:

```go
package main

import (
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type rawEntry struct {
	cpu  float64
	name string
}

type tickMsg struct {
	entries []rawEntry
}

func parsePS(output string) []rawEntry {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return nil
	}
	var entries []rawEntry
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		cpu, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			continue
		}
		name := strings.Join(fields[1:], " ")
		entries = append(entries, rawEntry{cpu: cpu, name: name})
	}
	return entries
}

func fetchProcesses() tickMsg {
	out, err := exec.Command("ps", "-eo", "%cpu,comm").Output()
	if err != nil {
		return tickMsg{}
	}
	return tickMsg{entries: parsePS(string(out))}
}

func pollCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return fetchProcesses()
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./... -run TestParsePS -v
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add poller.go poller_test.go
git commit -m "feat: add ps poller with pure parsePS function and tests"
```

---

### Task 4: Implement model.go with TDD

**Files:**
- Create: `model.go`
- Create: `model_test.go`

- [ ] **Step 1: Write the failing tests**

Create `model_test.go`:

```go
package main

import (
	"fmt"
	"testing"
)

func TestBuildDisplayListFiltersExcluded(t *testing.T) {
	m := newModel(map[string]struct{}{"firefox": {}}, "")
	m.cumulative = map[string]float64{
		"firefox": 50.0,
		"node":    30.0,
		"bash":    10.0,
	}
	list := m.buildDisplayList()

	for _, p := range list {
		if p.name == "firefox" {
			t.Error("firefox should be excluded from display list")
		}
	}
	if len(list) != 2 {
		t.Errorf("expected 2 entries, got %d", len(list))
	}
}

func TestBuildDisplayListSortedDescending(t *testing.T) {
	m := newModel(make(map[string]struct{}), "")
	m.cumulative = map[string]float64{
		"a": 10.0,
		"b": 50.0,
		"c": 30.0,
	}
	list := m.buildDisplayList()

	if len(list) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(list))
	}
	if list[0].name != "b" {
		t.Errorf("expected b first (highest cpu), got %q", list[0].name)
	}
	if list[1].name != "c" {
		t.Errorf("expected c second, got %q", list[1].name)
	}
	if list[2].name != "a" {
		t.Errorf("expected a third, got %q", list[2].name)
	}
}

func TestBuildDisplayListFilterCaseInsensitive(t *testing.T) {
	m := newModel(make(map[string]struct{}), "")
	m.cumulative = map[string]float64{
		"Firefox": 50.0,
		"node":    30.0,
	}
	m.filter = "fire"
	list := m.buildDisplayList()

	if len(list) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(list))
	}
	if list[0].name != "Firefox" {
		t.Errorf("expected Firefox, got %q", list[0].name)
	}
}

func TestBuildDisplayListCapsAt60(t *testing.T) {
	m := newModel(make(map[string]struct{}), "")
	m.cumulative = make(map[string]float64)
	for i := 0; i < 100; i++ {
		m.cumulative[fmt.Sprintf("proc%d", i)] = float64(i)
	}
	list := m.buildDisplayList()
	if len(list) != 60 {
		t.Errorf("expected 60 entries (display limit), got %d", len(list))
	}
}

func TestBuildDisplayListEmptyCumulative(t *testing.T) {
	m := newModel(make(map[string]struct{}), "")
	list := m.buildDisplayList()
	if len(list) != 0 {
		t.Errorf("expected 0 entries for empty cumulative, got %d", len(list))
	}
}

func TestClamp(t *testing.T) {
	if clamp(5, 0, 10) != 5 {
		t.Error("clamp(5,0,10) should be 5")
	}
	if clamp(-1, 0, 10) != 0 {
		t.Error("clamp(-1,0,10) should be 0")
	}
	if clamp(15, 0, 10) != 10 {
		t.Error("clamp(15,0,10) should be 10")
	}
	if clamp(5, 0, -1) != 0 {
		t.Error("clamp(5,0,-1) should be 0 (empty list case)")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
go test ./... -run "TestBuildDisplayList|TestClamp" -v
```

Expected: compile error — `newModel`, `procEntry`, `buildDisplayList`, `clamp` undefined.

- [ ] **Step 3: Implement model.go**

Create `model.go`:

```go
package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	displayLimit = 60
	pollInterval = 2 * time.Second
)

type procEntry struct {
	name string
	cpu  float64
}

type model struct {
	cumulative   map[string]float64
	excluded     map[string]struct{}
	excludedPath string
	filter       string
	cursor       int
	displayList  []procEntry
}

var (
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	headerStyle  = lipgloss.NewStyle().Bold(true)
	dividerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func newModel(excluded map[string]struct{}, excludedPath string) model {
	return model{
		cumulative:   make(map[string]float64),
		excluded:     excluded,
		excludedPath: excludedPath,
	}
}

func (m model) Init() tea.Cmd {
	return pollCmd(pollInterval)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		for _, e := range msg.entries {
			m.cumulative[e.name] += e.cpu
		}
		m.displayList = m.buildDisplayList()
		m.cursor = clamp(m.cursor, 0, len(m.displayList)-1)
		return m, pollCmd(pollInterval)

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit

		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}

		case tea.KeyDown:
			if m.cursor < len(m.displayList)-1 {
				m.cursor++
			}

		case tea.KeyDelete:
			if len(m.displayList) > 0 {
				name := m.displayList[m.cursor].name
				m.excluded[name] = struct{}{}
				_ = appendExclusion(m.excludedPath, name)
				m.displayList = m.buildDisplayList()
				m.cursor = clamp(m.cursor, 0, len(m.displayList)-1)
			}

		case tea.KeyEsc:
			m.filter = ""
			m.displayList = m.buildDisplayList()
			m.cursor = clamp(m.cursor, 0, len(m.displayList)-1)

		case tea.KeyBackspace:
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				m.displayList = m.buildDisplayList()
				m.cursor = clamp(m.cursor, 0, len(m.displayList)-1)
			}

		case tea.KeyRunes:
			if msg.String() == "q" && m.filter == "" {
				return m, tea.Quit
			}
			m.filter += msg.String()
			m.displayList = m.buildDisplayList()
			m.cursor = 0
		}
	}
	return m, nil
}

func (m model) View() string {
	var sb strings.Builder

	sb.WriteString(headerStyle.Render(fmt.Sprintf(
		"top_cpu  [filter: %s]   excluded: %d",
		m.filter+"_", len(m.excluded),
	)))
	sb.WriteString("\n")
	sb.WriteString(dividerStyle.Render(strings.Repeat("─", 52)))
	sb.WriteString("\n")

	for i, p := range m.displayList {
		prefix := "  "
		line := fmt.Sprintf("%3d  %6.1f%%  %s", i+1, p.cpu, p.name)
		if i == m.cursor {
			prefix = "▶ "
			line = cursorStyle.Render(line)
		}
		sb.WriteString(prefix + line + "\n")
	}

	sb.WriteString(dividerStyle.Render(strings.Repeat("─", 52)))
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("↑↓ navigate  type to filter  Del exclude  q quit"))

	return sb.String()
}

func (m model) buildDisplayList() []procEntry {
	type kv struct {
		name string
		cpu  float64
	}
	filterLower := strings.ToLower(m.filter)
	all := make([]kv, 0, len(m.cumulative))
	for name, cpu := range m.cumulative {
		if _, ok := m.excluded[name]; ok {
			continue
		}
		if filterLower != "" && !strings.Contains(strings.ToLower(name), filterLower) {
			continue
		}
		all = append(all, kv{name, cpu})
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].cpu > all[j].cpu
	})
	if len(all) > displayLimit {
		all = all[:displayLimit]
	}
	result := make([]procEntry, len(all))
	for i, kv := range all {
		result[i] = procEntry{name: kv.name, cpu: kv.cpu}
	}
	return result
}

func clamp(v, lo, hi int) int {
	if hi < 0 {
		return 0
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./... -run "TestBuildDisplayList|TestClamp" -v
```

Expected: all 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add model.go model_test.go
git commit -m "feat: add Bubble Tea model with display list logic and tests"
```

---

### Task 5: Implement main.go and wire everything together

**Files:**
- Create: `main.go`

- [ ] **Step 1: Create main.go**

```go
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

const excludedFilePath = "./top_cpu_excluded.txt"

func main() {
	excluded, err := loadExclusions(excludedFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load exclusions: %v\n", err)
		excluded = make(map[string]struct{})
	}

	m := newModel(excluded, excludedFilePath)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Build to verify no compile errors**

```bash
go build ./...
```

Expected: no output (success), binary `top_cpu` created.

- [ ] **Step 3: Run all tests**

```bash
go test ./... -v
```

Expected: all tests PASS. No failures.

- [ ] **Step 4: Commit**

```bash
git add main.go
git commit -m "feat: add main entry point — top_cpu TUI complete"
```

---

### Task 6: Add .gitignore and smoke test

**Files:**
- Create: `.gitignore`

- [ ] **Step 1: Create .gitignore**

```
top_cpu
top_cpu_excluded.txt
```

- [ ] **Step 2: Commit .gitignore**

```bash
git add .gitignore
git commit -m "chore: add gitignore for binary and exclusions file"
```

- [ ] **Step 3: Smoke test**

```bash
./top_cpu
```

Expected:
- Terminal switches to alt screen
- Header shows `top_cpu  [filter: _]   excluded: 0`
- List populates within 2s with process names and CPU%
- ↑/↓ moves cursor
- Typing narrows list live
- Esc clears filter
- Del on a process removes it from list and creates/appends `./top_cpu_excluded.txt`
- q quits and restores terminal
- Re-running the app: previously excluded processes absent from list

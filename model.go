package main

import (
	"fmt"
	"os"
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
	name  string
	cpu   float64
	ports []int
	pid   string
}

const (
	tabList     = 0
	tabTimeline = 1
)

type model struct {
	cumulative   map[string]float64
	latestPorts  map[string][]int
	latestPID    map[string]string
	excluded     map[string]struct{}
	excludedPath string
	filter       string
	filtering    bool   // true when F4 filter mode is active
	cursor       int
	offset       int    // first visible row index
	width        int    // terminal width in columns
	height       int    // terminal height in rows
	selected     string // process name locked for exclusion (empty = none)
	displayList  []procEntry
	activeTab    int
	timeline     timelineState
}

var (
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true)
	headerStyle   = lipgloss.NewStyle().Bold(true)
	dividerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func newModel(excluded map[string]struct{}, excludedPath string) model {
	return model{
		cumulative:   make(map[string]float64),
		latestPorts:  make(map[string][]int),
		latestPID:    make(map[string]string),
		excluded:     excluded,
		excludedPath: excludedPath,
		width:        80,
		height:       24,
		timeline:     newTimelineState(),
	}
}

func (m model) viewHeight() int {
	const fixedRows = 5 // tab bar + top divider + column header + bottom divider + help
	h := m.height - fixedRows
	if h < 1 {
		return 1
	}
	return h
}

func (m model) syncedOffset() int {
	vh := m.viewHeight()
	if m.cursor < m.offset {
		return m.cursor
	}
	if m.cursor >= m.offset+vh {
		return m.cursor - vh + 1
	}
	return m.offset
}

func (m model) Init() tea.Cmd {
	return pollCmd(pollInterval)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.offset = m.syncedOffset()
		return m, nil

	case tickMsg:
		nextPorts := make(map[string][]int, len(msg.entries))
		nextPID := make(map[string]string, len(msg.entries))
		for _, e := range msg.entries {
			m.cumulative[e.name] += e.cpu
			if len(e.ports) > 0 {
				nextPorts[e.name] = e.ports
			}
			nextPID[e.name] = e.pid
		}
		m.latestPorts = nextPorts
		m.latestPID = nextPID
		m.displayList = m.buildDisplayList()
		m.cursor = clamp(m.cursor, 0, len(m.displayList)-1)
		m.offset = m.syncedOffset()
		return m, pollCmd(pollInterval)

	case timelineDataMsg:
		if msg.rng == m.timeline.rng {
			m.timeline.loaded = true
			m.timeline.loading = false
			m.timeline.err = msg.err
			m.timeline.rankedNames = msg.rankedNames
			m.timeline.series = msg.series
			m.timeline.lastQuery = time.Now()
		}
		return m, nil

	case timelineRefreshTickMsg:
		if m.activeTab == tabTimeline {
			m.timeline.loading = true
			return m, tea.Batch(
				loadTimelineCmd(m.timeline.rng, m.timeline.dbPath, m.width),
				timelineRefreshTick(),
			)
		}
		return m, nil

	case tea.KeyMsg:
		// Filter mode consumes most keys.
		if m.filtering {
			return m.updateFiltering(msg)
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit

		case tea.KeyLeft:
			if m.activeTab > 0 {
				m.activeTab--
			}
			return m, nil

		case tea.KeyRight:
			if m.activeTab < tabTimeline {
				m.activeTab++
				if m.activeTab == tabTimeline && !m.timeline.loaded {
					m.timeline.loading = true
					return m, tea.Batch(
						loadTimelineCmd(m.timeline.rng, m.timeline.dbPath, m.width),
						timelineRefreshTick(),
					)
				}
			}
			return m, nil
		}

		if m.activeTab == tabTimeline {
			return m.updateTimeline(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
			m.offset = m.syncedOffset()
		}

	case tea.KeyDown:
		if m.cursor < len(m.displayList)-1 {
			m.cursor++
			m.offset = m.syncedOffset()
		}

	case tea.KeyEnter:
		if len(m.displayList) > 0 {
			m.selected = m.displayList[m.cursor].name
		}

	case tea.KeyF1:
		if m.selected != "" {
			next := make(map[string]struct{}, len(m.excluded)+1)
			for k, v := range m.excluded {
				next[k] = v
			}
			next[m.selected] = struct{}{}
			m.excluded = next
			if err := appendExclusion(m.excludedPath, m.selected); err != nil {
				fmt.Fprintf(os.Stderr, "top_cpu: failed to persist exclusion: %v\n", err)
			}
			m.selected = ""
			m.displayList = m.buildDisplayList()
			m.cursor = clamp(m.cursor, 0, len(m.displayList)-1)
			m.offset = m.syncedOffset()
		}

	case tea.KeyF4:
		m.filtering = true

	case tea.KeyEsc:
		if m.selected != "" {
			m.selected = ""
		}

	case tea.KeyRunes:
		if msg.String() == "q" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) updateFiltering(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit

	case tea.KeyEsc:
		m.filtering = false
		m.filter = ""
		m.displayList = m.buildDisplayList()
		m.cursor = clamp(m.cursor, 0, len(m.displayList)-1)
		m.offset = m.syncedOffset()

	case tea.KeyBackspace:
		if len(m.filter) > 0 {
			r := []rune(m.filter)
			m.filter = string(r[:len(r)-1])
			m.displayList = m.buildDisplayList()
			m.cursor = clamp(m.cursor, 0, len(m.displayList)-1)
			m.offset = m.syncedOffset()
		}

	case tea.KeyRunes:
		m.filter += msg.String()
		m.displayList = m.buildDisplayList()
		m.cursor = 0
		m.offset = 0
	}
	return m, nil
}

func (m model) updateTimeline(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyRunes:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "t":
			m.timeline.rng = m.timeline.rng.cycle()
			m.timeline.loading = true
			m.timeline.loaded = false
			return m, loadTimelineCmd(m.timeline.rng, m.timeline.dbPath, m.width)
		case "r":
			m.timeline.loading = true
			return m, loadTimelineCmd(m.timeline.rng, m.timeline.dbPath, m.width)
		}
	}
	return m, nil
}

func (m model) View() string {
	var sb strings.Builder
	sb.WriteString(m.renderTabBar())
	sb.WriteString("\n")
	sb.WriteString(dividerStyle.Render(strings.Repeat("─", 52)))
	sb.WriteString("\n")

	if m.activeTab == tabTimeline {
		sb.WriteString(renderTimeline(m.timeline, m.width, m.height))
		sb.WriteString("\n")
		sb.WriteString(dividerStyle.Render(strings.Repeat("─", 52)))
		sb.WriteString("\n")
		sb.WriteString(helpStyle.Render("←→ switch tabs  t cycle range  r refresh  q quit"))
		return sb.String()
	}

	header := fmt.Sprintf("  %-8s  %-9s  %-7s  %-20s  %s", "Position", "CPU Usage", "PID", "Port", "Process Name")
	sb.WriteString(headerStyle.Render(header))
	sb.WriteString("\n")

	end := m.offset + m.viewHeight()
	if end > len(m.displayList) {
		end = len(m.displayList)
	}
	for i := m.offset; i < end; i++ {
		p := m.displayList[i]
		prefix := "  "
		line := fmt.Sprintf("%-8d  %8.1f%%  %-7s  %-20s  %s", i+1, p.cpu, p.pid, formatPorts(p.ports), p.name)
		switch {
		case p.name == m.selected:
			prefix = "★ "
			line = selectedStyle.Render(line)
		case i == m.cursor:
			prefix = "▶ "
			line = cursorStyle.Render(line)
		}
		sb.WriteString(prefix + line + "\n")
	}

	sb.WriteString(dividerStyle.Render(strings.Repeat("─", 52)))
	sb.WriteString("\n")
	var help string
	switch {
	case m.selected != "":
		help = "↑↓ navigate  F1 exclude selected  Esc deselect  ←→ tabs  q quit"
	case m.filtering:
		help = "type to filter  Backspace  Esc exit filter"
	default:
		help = "↑↓ navigate  Enter select  F4 filter  ←→ tabs  q quit"
	}
	sb.WriteString(helpStyle.Render(help))

	return sb.String()
}

func (m model) renderTabBar() string {
	listTab := "  List  "
	timelineTab := "  Timeline  "
	if m.activeTab == tabList {
		listTab = "▶ List  "
	} else {
		timelineTab = "▶ Timeline  "
	}
	bar := fmt.Sprintf("[%s] [%s]", listTab, timelineTab)

	switch m.activeTab {
	case tabList:
		bar += fmt.Sprintf("   excluded: %d", len(m.excluded))
		if m.filtering {
			bar += fmt.Sprintf("   [filter: %s_]", m.filter)
		}
		if m.selected != "" {
			bar += fmt.Sprintf("   selected: %s", selectedStyle.Render(m.selected))
		}
	case tabTimeline:
		bar += fmt.Sprintf("   Range: %s", m.timeline.rng.label())
		if m.timeline.loading {
			bar += "   (loading…)"
		}
	}
	return headerStyle.Render(bar)
}

func (m model) buildDisplayList() []procEntry {
	type kv struct {
		name  string
		cpu   float64
		ports []int
		pid   string
	}
	filterLower := strings.ToLower(m.filter)
	all := make([]kv, 0, len(m.cumulative))
	for name, cpu := range m.cumulative {
		if _, ok := m.excluded[name]; ok {
			continue
		}
		ports := m.latestPorts[name]
		if filterLower != "" && !matchesFilter(name, ports, filterLower) {
			continue
		}
		all = append(all, kv{name, cpu, ports, m.latestPID[name]})
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].cpu > all[j].cpu
	})
	if len(all) > displayLimit {
		all = all[:displayLimit]
	}
	result := make([]procEntry, len(all))
	for i, kv := range all {
		result[i] = procEntry{name: kv.name, cpu: kv.cpu, ports: kv.ports, pid: kv.pid}
	}
	return result
}

func matchesFilter(name string, ports []int, filterLower string) bool {
	if strings.Contains(strings.ToLower(name), filterLower) {
		return true
	}
	for _, p := range ports {
		if strings.Contains(fmt.Sprintf("%d", p), filterLower) {
			return true
		}
	}
	return false
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

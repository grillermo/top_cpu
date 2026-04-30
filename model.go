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
	name string
	cpu  float64
}

type model struct {
	cumulative   map[string]float64
	excluded     map[string]struct{}
	excludedPath string
	filter       string
	cursor       int
	offset       int // first visible row index
	height       int // terminal height in rows
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
		height:       24,
	}
}

func (m model) viewHeight() int {
	const fixedRows = 4 // header + top divider + bottom divider + help
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
		m.height = msg.Height
		m.offset = m.syncedOffset()
		return m, nil

	case tickMsg:
		for _, e := range msg.entries {
			m.cumulative[e.name] += e.cpu
		}
		m.displayList = m.buildDisplayList()
		m.cursor = clamp(m.cursor, 0, len(m.displayList)-1)
		m.offset = m.syncedOffset()
		return m, pollCmd(pollInterval)

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit

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

		case tea.KeyF1:
			if len(m.displayList) > 0 {
				name := m.displayList[m.cursor].name
				next := make(map[string]struct{}, len(m.excluded)+1)
				for k, v := range m.excluded {
					next[k] = v
				}
				next[name] = struct{}{}
				m.excluded = next
				if err := appendExclusion(m.excludedPath, name); err != nil {
					fmt.Fprintf(os.Stderr, "top_cpu: failed to persist exclusion: %v\n", err)
				}
				m.displayList = m.buildDisplayList()
				m.cursor = clamp(m.cursor, 0, len(m.displayList)-1)
				m.offset = m.syncedOffset()
			}

		case tea.KeyEsc:
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
			if msg.String() == "q" && m.filter == "" {
				return m, tea.Quit
			}
			m.filter += msg.String()
			m.displayList = m.buildDisplayList()
			m.cursor = 0
			m.offset = 0
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

	end := m.offset + m.viewHeight()
	if end > len(m.displayList) {
		end = len(m.displayList)
	}
	for i := m.offset; i < end; i++ {
		p := m.displayList[i]
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
	sb.WriteString(helpStyle.Render("↑↓ navigate  type to filter  F1 exclude  q quit"))

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

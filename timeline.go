package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/guptarohit/asciigraph"
)

const (
	timelineSeriesCount  = 10
	timelineRefreshEvery = 60 * time.Second
)

type timeRange int

const (
	rangeToday timeRange = iota
	rangeWeek
	rangeAllTime
)

func (r timeRange) label() string {
	switch r {
	case rangeToday:
		return "Today"
	case rangeWeek:
		return "This Week"
	case rangeAllTime:
		return "All Time"
	}
	return "?"
}

func (r timeRange) window(now time.Time) (time.Time, time.Time) {
	switch r {
	case rangeToday:
		return now.Add(-24 * time.Hour), now
	case rangeWeek:
		return now.Add(-7 * 24 * time.Hour), now
	case rangeAllTime:
		return now.Add(-retentionDuration), now
	}
	return now.Add(-24 * time.Hour), now
}

func (r timeRange) cycle() timeRange {
	return (r + 1) % 3
}

type timelineState struct {
	rng         timeRange
	loaded      bool
	loading     bool
	err         error
	rankedNames []string
	series      map[string][]Sample
	lastQuery   time.Time
	dbPath      string
}

func newTimelineState() timelineState {
	path, _ := defaultDBPath()
	return timelineState{
		rng:    rangeWeek,
		dbPath: path,
	}
}

type timelineDataMsg struct {
	rng         timeRange
	rankedNames []string
	series      map[string][]Sample
	err         error
}

type timelineRefreshTickMsg struct{}

func loadTimelineCmd(rng timeRange, dbPath string, width int) tea.Cmd {
	return func() tea.Msg {
		store, err := OpenStore(dbPath)
		if err != nil {
			return timelineDataMsg{rng: rng, err: err}
		}
		defer store.Close()
		now := time.Now()
		start, end := rng.window(now)
		buckets := chartWidth(width)
		series, names, err := store.QueryTopN(start, end, timelineSeriesCount, buckets)
		return timelineDataMsg{rng: rng, rankedNames: names, series: series, err: err}
	}
}

func timelineRefreshTick() tea.Cmd {
	return tea.Tick(timelineRefreshEvery, func(time.Time) tea.Msg {
		return timelineRefreshTickMsg{}
	})
}

func chartWidth(termWidth int) int {
	w := termWidth - 15
	if w < 20 {
		return 20
	}
	if w > 200 {
		return 200
	}
	return w
}

var seriesColors = []asciigraph.AnsiColor{
	asciigraph.Red,
	asciigraph.Green,
	asciigraph.Yellow,
	asciigraph.Blue,
	asciigraph.Magenta,
	asciigraph.Cyan,
	asciigraph.White,
	asciigraph.Orange,
	asciigraph.SpringGreen,
	asciigraph.Pink,
}

func renderTimeline(state timelineState, width, height int) string {
	var sb strings.Builder

	if state.err != nil {
		sb.WriteString(helpStyle.Render(fmt.Sprintf("Error: %v", state.err)))
		return sb.String()
	}
	if !dbExists(state.dbPath) {
		sb.WriteString(helpStyle.Render("No database found.\n\n"))
		sb.WriteString("Run: ")
		sb.WriteString(lipgloss.NewStyle().Bold(true).Render("top_cpu --daemon"))
		sb.WriteString("\n\nThis records CPU samples in the background.\n")
		sb.WriteString(helpStyle.Render(fmt.Sprintf("DB path: %s", formatDBPath(state.dbPath))))
		return sb.String()
	}
	if !state.loaded && !state.loading {
		return helpStyle.Render("Press r to load…")
	}
	if state.loading && !state.loaded {
		return helpStyle.Render("Loading…")
	}
	if len(state.rankedNames) == 0 {
		return helpStyle.Render(fmt.Sprintf("No samples in window: %s", state.rng.label()))
	}

	chartH := height - 7
	if chartH < 5 {
		chartH = 5
	}
	cw := chartWidth(width)

	plotData := make([][]float64, 0, len(state.rankedNames))
	colors := make([]asciigraph.AnsiColor, 0, len(state.rankedNames))
	for i, name := range state.rankedNames {
		raw := state.series[name]
		points := make([]float64, len(raw))
		for j, s := range raw {
			points[j] = math.Log10(s.AvgCPU + 1)
		}
		plotData = append(plotData, points)
		colors = append(colors, seriesColors[i%len(seriesColors)])
	}

	graph := asciigraph.PlotMany(
		plotData,
		asciigraph.Height(chartH),
		asciigraph.Width(cw),
		asciigraph.Precision(1),
		asciigraph.SeriesColors(colors...),
	)
	sb.WriteString(graph)
	sb.WriteString("\n")
	sb.WriteString(renderXAxis(state, cw))
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("Y axis: log₁₀(cpu% + 1)"))
	sb.WriteString("\n")
	sb.WriteString(renderLegend(state.rankedNames, colors, width))
	return sb.String()
}

func renderXAxis(state timelineState, chartW int) string {
	now := time.Now()
	start, end := state.rng.window(now)
	const labelCount = 5
	var b strings.Builder
	pad := 14 // approximate offset for asciigraph y-axis labels
	b.WriteString(strings.Repeat(" ", pad))

	step := chartW / (labelCount - 1)
	for i := 0; i < labelCount; i++ {
		frac := float64(i) / float64(labelCount-1)
		ts := start.Add(time.Duration(float64(end.Sub(start)) * frac))
		var label string
		if state.rng == rangeAllTime {
			label = ts.Format("01/02")
		} else {
			label = ts.Format("15:04")
		}
		if i > 0 {
			used := i * step
			b.WriteString(strings.Repeat(" ", max(0, used-b.Len()+pad-len(label)/2)))
		}
		b.WriteString(label)
	}
	return helpStyle.Render(b.String())
}

func renderLegend(names []string, colors []asciigraph.AnsiColor, width int) string {
	if len(names) == 0 {
		return ""
	}
	maxPerLine := max(1, width/22)
	var b strings.Builder
	for i, name := range names {
		if i > 0 && i%maxPerLine == 0 {
			b.WriteString("\n")
		}
		entry := fmt.Sprintf("── %s", truncate(name, 18))
		colored := lipgloss.NewStyle().Foreground(ansiToLipgloss(colors[i])).Render(entry)
		b.WriteString(colored)
		b.WriteString("  ")
	}
	return b.String()
}

func ansiToLipgloss(c asciigraph.AnsiColor) lipgloss.Color {
	return lipgloss.Color(fmt.Sprintf("%d", int(c)))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return s[:n-1] + "…"
}

package main

import (
	"fmt"
	"os"
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
		fmt.Fprintf(os.Stderr, "top_cpu: ps error: %v\n", err)
		return tickMsg{}
	}
	return tickMsg{entries: parsePS(string(out))}
}

func pollCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return fetchProcesses()
	})
}

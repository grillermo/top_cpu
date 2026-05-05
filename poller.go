package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
		if len(fields) < 3 {
			continue
		}
		cpu, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			continue
		}
		pid := fields[1]
		argsStart := strings.Index(line, fields[2])
		if argsStart == -1 {
			continue
		}
		args := strings.TrimSpace(line[argsStart:])
		name := processDisplayName(args, pid)
		entries = append(entries, rawEntry{cpu: cpu, name: name})
	}
	return entries
}

func processDisplayName(args, pid string) string {
	if strings.HasSuffix(args, "]") {
		if i := strings.LastIndex(args, "["); i != -1 {
			title := strings.TrimSpace(args[i:])
			if len(title) >= 3 {
			return fmt.Sprintf("%s (%s)", title, pid)
			}
		}
	}

	argv := strings.Fields(args)
	if len(argv) == 0 {
		return fmt.Sprintf("unknown (%s)", pid)
	}
	return fmt.Sprintf("%s (%s)", filepath.Base(argv[0]), pid)
}

func fetchProcesses() tickMsg {
	out, err := exec.Command("ps", "-eo", "%cpu,pid,args").Output()
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

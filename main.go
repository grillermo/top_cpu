package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

const excludedFilePath = "./top_cpu_excluded.txt"

func main() {
	daemon := flag.Bool("daemon", false, "run as background recorder (writes CPU samples to SQLite)")
	flag.Parse()

	if *daemon {
		if err := runDaemon(); err != nil {
			fmt.Fprintf(os.Stderr, "top_cpu daemon: %v\n", err)
			os.Exit(1)
		}
		return
	}

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

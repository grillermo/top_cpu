package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

const (
	daemonPollInterval = 30 * time.Second
	purgeInterval      = 1 * time.Hour
	retentionDuration  = 30 * 24 * time.Hour
)

func runDaemon() error {
	dbPath, err := defaultDBPath()
	if err != nil {
		return fmt.Errorf("resolve db path: %w", err)
	}
	store, err := OpenStore(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Fprintf(os.Stderr, "top_cpu daemon started, db=%s, interval=%s\n", dbPath, daemonPollInterval)

	if err := store.Purge(time.Now().Add(-retentionDuration)); err != nil {
		fmt.Fprintf(os.Stderr, "top_cpu daemon: initial purge failed: %v\n", err)
	}

	pollTick := time.NewTicker(daemonPollInterval)
	defer pollTick.Stop()
	purgeTick := time.NewTicker(purgeInterval)
	defer purgeTick.Stop()

	if err := pollOnce(store); err != nil {
		fmt.Fprintf(os.Stderr, "top_cpu daemon: poll error: %v\n", err)
	}

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "top_cpu daemon: shutting down")
			return nil
		case <-pollTick.C:
			if err := pollOnce(store); err != nil {
				fmt.Fprintf(os.Stderr, "top_cpu daemon: poll error: %v\n", err)
			}
		case <-purgeTick.C:
			if err := store.Purge(time.Now().Add(-retentionDuration)); err != nil {
				fmt.Fprintf(os.Stderr, "top_cpu daemon: purge error: %v\n", err)
			}
		}
	}
}

func pollOnce(store *Store) error {
	out, err := exec.Command("ps", "-eo", "%cpu,pid,args").Output()
	if err != nil {
		return fmt.Errorf("ps: %w", err)
	}
	entries := parsePS(string(out))
	if len(entries) == 0 {
		return nil
	}
	return store.Insert(entries, time.Now())
}

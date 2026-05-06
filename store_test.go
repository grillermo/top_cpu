package main

import (
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestStoreInsertAndQuery(t *testing.T) {
	store := openTestStore(t)
	now := time.Now()

	for i := 0; i < 5; i++ {
		err := store.Insert([]rawEntry{
			{cpu: float64(10 + i), name: "high (1)"},
			{cpu: float64(1 + i), name: "low (2)"},
		}, now.Add(time.Duration(i)*time.Minute))
		if err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	series, names, err := store.QueryTopN(now.Add(-time.Hour), now.Add(time.Hour), 10, 5)
	if err != nil {
		t.Fatalf("QueryTopN: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 ranked names, got %d", len(names))
	}
	if names[0] != "high (1)" {
		t.Errorf("expected high (1) ranked first, got %q", names[0])
	}
	if len(series["high (1)"]) != 5 {
		t.Errorf("expected 5 buckets for high, got %d", len(series["high (1)"]))
	}
}

func TestStorePurgeRespectsCutoff(t *testing.T) {
	store := openTestStore(t)
	now := time.Now()

	old := now.Add(-40 * 24 * time.Hour)
	recent := now.Add(-1 * time.Hour)

	if err := store.Insert([]rawEntry{{cpu: 1.0, name: "old"}}, old); err != nil {
		t.Fatalf("Insert old: %v", err)
	}
	if err := store.Insert([]rawEntry{{cpu: 2.0, name: "recent"}}, recent); err != nil {
		t.Fatalf("Insert recent: %v", err)
	}
	if err := store.Purge(now.Add(-30 * 24 * time.Hour)); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	_, names, err := store.QueryTopN(now.Add(-50*24*time.Hour), now, 10, 10)
	if err != nil {
		t.Fatalf("QueryTopN: %v", err)
	}
	for _, n := range names {
		if n == "old" {
			t.Error("old row should have been purged")
		}
	}
}

func TestStoreEmptyWindow(t *testing.T) {
	store := openTestStore(t)
	now := time.Now()
	series, names, err := store.QueryTopN(now.Add(-time.Hour), now, 10, 10)
	if err != nil {
		t.Fatalf("QueryTopN: %v", err)
	}
	if len(names) != 0 || len(series) != 0 {
		t.Errorf("expected empty result, got names=%d series=%d", len(names), len(series))
	}
}

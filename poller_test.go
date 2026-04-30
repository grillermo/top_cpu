package main

import (
	"testing"
)

func TestParsePSNormalOutput(t *testing.T) {
	input := `%CPU   PID COMM
  1.5   101 firefox
 23.4   202 node
  0.0   303 ps`

	entries := parsePS(input)

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].cpu != 1.5 || entries[0].name != "firefox (101)" {
		t.Errorf("entry 0: got cpu=%v name=%q, want cpu=1.5 name=\"firefox (101)\"", entries[0].cpu, entries[0].name)
	}
	if entries[1].cpu != 23.4 || entries[1].name != "node (202)" {
		t.Errorf("entry 1: got cpu=%v name=%q, want cpu=23.4 name=\"node (202)\"", entries[1].cpu, entries[1].name)
	}
	if entries[2].cpu != 0.0 || entries[2].name != "ps (303)" {
		t.Errorf("entry 2: got cpu=%v name=%q, want cpu=0.0 name=\"ps (303)\"", entries[2].cpu, entries[2].name)
	}
}

func TestParsePSHeaderOnly(t *testing.T) {
	entries := parsePS("%CPU   PID COMM\n")
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
	input := `%CPU   PID COMM
  notanumber 101 firefox
 10.0   202 node`

	entries := parsePS(input)
	if len(entries) != 1 {
		t.Fatalf("expected 1 valid entry, got %d", len(entries))
	}
	if entries[0].name != "node (202)" {
		t.Errorf("expected \"node (202)\", got %q", entries[0].name)
	}
}

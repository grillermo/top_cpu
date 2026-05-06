# top_cpu

Real-time TUI showing cumulative CPU usage per process. Aggregates `ps` output every 2 seconds so short-lived spikes accumulate over time. Lets you exclude noisy processes permanently.

<img width="892" height="626" alt="CleanShot 2026-05-05 at 22 51 15@2x" src="https://github.com/user-attachments/assets/4fd24954-5378-4d3c-baf7-d726cedaf2f3" />


## Requirements

- macOS (uses `ps -eo %cpu,comm`)
- Go 1.21+

Install Go via Homebrew if needed:

```bash
brew install go
```

Verify:

```bash
go version
```

## Setup

```bash
git clone <repo-url>
cd top_cpu
go mod download
```

## Build

```bash
go build -o top_cpu .
```

## Run

```bash
./top_cpu
```

## Controls

| Key | Action |
|-----|--------|
| `↑` / `↓` | Move cursor |
| Type anything | Filter list live (case-insensitive) |
| `Backspace` | Remove last filter character |
| `Esc` | Clear filter |
| `F1` | Exclude process at cursor |
| `q` or `Ctrl+C` | Quit |

## Process Exclusion

Excluded processes are saved to `./top_cpu_excluded.txt` (one name per line, in the directory where you run the binary). They persist across restarts.

To un-exclude a process, edit the file and remove its line, then restart `top_cpu`.

## Development

Run tests:

```bash
go test ./...
```

Run with verbose test output:

```bash
go test ./... -v
```

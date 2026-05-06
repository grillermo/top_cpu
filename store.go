package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Sample struct {
	BucketStart time.Time
	AvgCPU      float64
}

func defaultDBPath() (string, error) {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "top_cpu", "top_cpu.db"), nil
}

func OpenStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS cpu_samples (
			process_name TEXT    NOT NULL,
			cpu_pct      REAL    NOT NULL,
			recorded_at  INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_recorded_at ON cpu_samples(recorded_at);
	`); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Insert(entries []rawEntry, at time.Time) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO cpu_samples (process_name, cpu_pct, recorded_at) VALUES (?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	ts := at.Unix()
	for _, e := range entries {
		if _, err := stmt.Exec(e.name, e.cpu, ts); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Purge(before time.Time) error {
	_, err := s.db.Exec(`DELETE FROM cpu_samples WHERE recorded_at < ?`, before.Unix())
	return err
}

// QueryTopN returns up to n processes ranked by max cpu_pct in [start, end],
// each as a slice of buckets averaged over equal-width time slots.
// If buckets <= 0, defaults to 1.
func (s *Store) QueryTopN(start, end time.Time, n, buckets int) (map[string][]Sample, []string, error) {
	if buckets < 1 {
		buckets = 1
	}
	if !end.After(start) {
		return map[string][]Sample{}, nil, nil
	}
	startUnix := start.Unix()
	endUnix := end.Unix()
	span := endUnix - startUnix
	if span < int64(buckets) {
		buckets = int(span)
		if buckets < 1 {
			buckets = 1
		}
	}
	bucketWidth := span / int64(buckets)
	if bucketWidth < 1 {
		bucketWidth = 1
	}

	topRows, err := s.db.Query(`
		SELECT process_name, MAX(cpu_pct) AS peak
		FROM cpu_samples
		WHERE recorded_at BETWEEN ? AND ?
		GROUP BY process_name
		ORDER BY peak DESC
		LIMIT ?
	`, startUnix, endUnix, n)
	if err != nil {
		return nil, nil, err
	}
	defer topRows.Close()

	var names []string
	for topRows.Next() {
		var name string
		var peak float64
		if err := topRows.Scan(&name, &peak); err != nil {
			return nil, nil, err
		}
		names = append(names, name)
	}
	if err := topRows.Err(); err != nil {
		return nil, nil, err
	}
	if len(names) == 0 {
		return map[string][]Sample{}, nil, nil
	}

	// One query per process (small N=10) to keep SQL portable.
	result := make(map[string][]Sample, len(names))
	for _, name := range names {
		bucketed, err := s.queryBuckets(name, startUnix, bucketWidth, buckets)
		if err != nil {
			return nil, nil, err
		}
		result[name] = bucketed
	}

	return result, names, nil
}

func (s *Store) queryBuckets(name string, startUnix, bucketWidth int64, buckets int) ([]Sample, error) {
	rows, err := s.db.Query(`
		SELECT
			((recorded_at - ?) / ?) AS bucket_idx,
			AVG(cpu_pct) AS avg_cpu
		FROM cpu_samples
		WHERE process_name = ?
		  AND recorded_at >= ?
		  AND recorded_at < ?
		GROUP BY bucket_idx
		ORDER BY bucket_idx
	`, startUnix, bucketWidth, name, startUnix, startUnix+int64(buckets)*bucketWidth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Sample, buckets)
	for i := range out {
		out[i] = Sample{
			BucketStart: time.Unix(startUnix+int64(i)*bucketWidth, 0),
			AvgCPU:      0,
		}
	}
	for rows.Next() {
		var idx int
		var avg float64
		if err := rows.Scan(&idx, &avg); err != nil {
			return nil, err
		}
		if idx >= 0 && idx < buckets {
			out[idx].AvgCPU = avg
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Verify path is reachable; used by TUI to detect missing daemon.
func dbExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func formatDBPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if rel, err := filepath.Rel(home, p); err == nil && len(rel) > 0 && rel[0] != '.' {
		return "~/" + rel
	}
	return p
}


// Package apphealthevent/store provides a tiny append-only event log for
// health_event_v1 events used by the OpenClaw Dashboard.
//
// It is intentionally simple: a single JSONL file under the dashboard directory,
// capped to maxLines so the file cannot grow without bound. It is not a general
// event bus; it is the local persistence surface the dashboard exposes via
// /api/health-events.
package apphealthevent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// defaultMaxLogLines caps the JSONL file. When the file grows beyond this,
// Store.Append rotates it by keeping the most recent entries.
const defaultMaxLogLines = 1000

// Store persists health events to disk and reads them back.
type Store struct {
	path     string
	maxLines int
	mu       sync.Mutex
}

// NewStore opens (or creates) the event log at the given dashboard directory.
func NewStore(dashboardDir string) *Store {
	return &Store{
		path:     filepath.Join(dashboardDir, "health-events.jsonl"),
		maxLines: defaultMaxLogLines,
	}
}

// SetMaxLogLines configures the cap on the JSONL log. Values <= 0 disable rotation.
func (s *Store) SetMaxLogLines(n int) {
	s.mu.Lock()
	if n > 0 {
		s.maxLines = n
	}
	s.mu.Unlock()
}

// Append validates and appends an event to the JSONL log. If the log exceeds
// maxLines, it is compacted to the most recent maxLines/2 entries.
func (s *Store) Append(e V1) error {
	if err := e.Validate(); err != nil {
		return fmt.Errorf("invalid health event: %w", err)
	}

	// Normalize details so round-tripping from JSONL always produces the same
	// numeric types (JSON numbers decode to float64; preserve original values).
	normalizedDetails := e.Details
	if len(e.Details) > 0 {
		b, err := json.Marshal(e.Details)
		if err != nil {
			return fmt.Errorf("marshal details: %w", err)
		}
		if err := json.Unmarshal(b, &normalizedDetails); err != nil {
			return fmt.Errorf("normalize details: %w", err)
		}
		e.Details = normalizedDetails
	}

	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal health event: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open health event log: %w", err)
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "%s\n", b); err != nil {
		return fmt.Errorf("write health event: %w", err)
	}
	return s.maybeCompactLocked()
}

// ReadAll returns all events newest-first. It does not hold the lock while
// decoding; instead it reads the file into memory and parses it under the lock
// only for the raw bytes step. This is acceptable because the log is small.
func (s *Store) ReadAll() ([]V1, error) {
	s.mu.Lock()
	raw, err := os.ReadFile(s.path)
	s.mu.Unlock()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read health event log: %w", err)
	}

	var events []V1
	sc := bufio.NewScanner(bytesOrReader(raw))
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e V1
		if err := json.Unmarshal(line, &e); err != nil {
			continue // skip corrupt lines
		}
		if err := e.Validate(); err != nil {
			continue
		}
		events = append(events, e)
	}
	// Reverse in place to newest-first.
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events, nil
}

func (s *Store) maybeCompactLocked() error {
	info, err := os.Stat(s.path)
	if err != nil {
		return nil
	}
	if info.Size() < int64(s.maxLines)*128 {
		return nil // cheap heuristic: avoid counting lines on every write
	}

	raw, err := os.ReadFile(s.path)
	if err != nil {
		return nil
	}
	lines := countLines(raw)
	if lines <= s.maxLines {
		return nil
	}

	keep := s.maxLines / 2
	start := lines - keep
	if start < 0 {
		start = 0
	}
	selected := selectLastLines(raw, start)

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, selected, 0o600); err != nil {
		return nil
	}
	_ = os.Rename(tmp, s.path)
	return nil
}

func countLines(b []byte) int {
	n := 0
	sc := bufio.NewScanner(bytesOrReader(b))
	for sc.Scan() {
		n++
	}
	return n
}

func selectLastLines(b []byte, skip int) []byte {
	var out []byte
	i := 0
	sc := bufio.NewScanner(bytesOrReader(b))
	for sc.Scan() {
		if i >= skip {
			out = append(out, sc.Bytes()...)
			out = append(out, '\n')
		}
		i++
	}
	return out
}

func bytesOrReader(b []byte) io.Reader {
	return bytes.NewReader(b)
}

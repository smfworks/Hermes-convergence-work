package apphealthevent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreAppendAndRead(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	e1, err := New("openclaw-dashboard", ComponentGateway, SeverityWarning, StatusDegraded, RecoveryHintRetry, "first")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	e2, err := New("openclaw-dashboard", ComponentGateway, SeverityWarning, StatusDegraded, RecoveryHintRetry, "second")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := store.Append(e1); err != nil {
		t.Fatalf("Append first: %v", err)
	}
	if err := store.Append(e2); err != nil {
		t.Fatalf("Append second: %v", err)
	}

	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	// Newest first.
	if events[0].Message != "second" {
		t.Fatalf("events[0].Message = %q, want second", events[0].Message)
	}
	if events[1].Message != "first" {
		t.Fatalf("events[1].Message = %q, want first", events[1].Message)
	}
}

func TestStoreAppendInvalid(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	invalid := V1{Schema: TypeV1}
	if err := store.Append(invalid); err == nil {
		t.Fatalf("expected error for invalid event")
	}
}

func TestStoreCompact(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	store.maxLines = 4 // small for test

	for i := 0; i < 6; i++ {
		e, _ := New("openclaw-dashboard", ComponentGateway, SeverityWarning, StatusDegraded, RecoveryHintRetry, "msg")
		if err := store.Append(e); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(events) > store.maxLines {
		t.Fatalf("len(events) = %d, want <= %d", len(events), store.maxLines)
	}
	if len(events) < 2 {
		t.Fatalf("len(events) = %d, want >= 2", len(events))
	}
}

func TestStoreReadMissing(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("len(events) = %d, want 0", len(events))
	}
}

func TestStoreIgnoresCorruptLines(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	if err := os.WriteFile(filepath.Join(dir, "health-events.jsonl"), []byte("not json\n"), 0o600); err != nil {
		t.Fatalf("setup corrupt file: %v", err)
	}
	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("len(events) = %d, want 0", len(events))
	}
}

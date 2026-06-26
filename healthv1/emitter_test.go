package apphealthevent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEmitterEmit(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	emit := NewEmitter(store, "openclaw-dashboard", nil)
	emit.Emit(ComponentGateway, SeverityCritical, StatusFailing, RecoveryHintRestartGateway, "gateway down", map[string]any{"port": float64(8080)})

	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if events[0].Component != ComponentGateway {
		t.Fatalf("component = %q, want %q", events[0].Component, ComponentGateway)
	}
	if events[0].Details["port"] != float64(8080) {
		t.Fatalf("details port = %v, want %v", events[0].Details["port"], float64(8080))
	}
}

func TestNilEmitter(t *testing.T) {
	var emit *Emitter
	emit.Emit(ComponentGateway, SeverityInfo, StatusHealthy, RecoveryHintNone, "noop", nil)
}

func TestEmitterEmitFromError(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	emit := NewEmitter(store, "openclaw-dashboard", nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	emit.EmitFromError(ComponentRefresh, ctx.Err(), "refresh context cancelled")

	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
}

func TestEmitterIgnoresMarshalFailure(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	emit := NewEmitter(store, "openclaw-dashboard", nil)
	emit.Emit(ComponentGateway, SeverityInfo, StatusHealthy, RecoveryHintNone, "ok", nil)
	// Write a line that decodes to V1 but fails validation, so it is skipped.
	if err := os.WriteFile(filepath.Join(dir, "health-events.jsonl"), []byte(`{"schema":"health_event_v1","id":"x","time":"2020-01-01T00:00:00Z","source":"x","component":"gateway","severity":"info","status":"healthy","recovery_hint":"none","message":"bad"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write invalid line: %v", err)
	}
	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1 (invalid line skipped)", len(events))
	}
}

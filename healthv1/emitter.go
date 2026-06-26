package apphealthevent

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// Emitter is the sink used by dashboard components to record health events.
// A nil Emitter is safe to use and silently drops events.
type Emitter struct {
	store  *Store
	source string
	logger *slog.Logger
}

// NewEmitter creates an Emitter backed by a Store.
func NewEmitter(store *Store, source string, logger *slog.Logger) *Emitter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Emitter{store: store, source: source, logger: logger}
}

// Emit writes a new health event. It never returns an error to callers; failures
// are logged but must not break the caller's main path.
func (e *Emitter) Emit(component Component, severity Severity, status Status, hint RecoveryHint, message string, details map[string]any) {
	if e == nil || e.store == nil {
		return
	}
	ev, err := New(e.source, component, severity, status, hint, message)
	if err != nil {
		e.logger.Warn("[health] failed to build health event", "error", err)
		return
	}
	if len(details) > 0 {
		ev.Details = details
	}
	if err := e.store.Append(ev); err != nil {
		e.logger.Warn("[health] failed to append health event", "error", err)
	}
}

// EmitFromError maps common dashboard errors to health events.
func (e *Emitter) EmitFromError(component Component, err error, fallbackMsg string) {
	if e == nil {
		return
	}
	severity := SeverityWarning
	status := StatusDegraded
	hint := RecoveryHintRetry
	msg := fallbackMsg
	if err != nil {
		msg = err.Error()
	}
	if msg == "" {
		msg = fallbackMsg
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		status = StatusDegraded
		hint = RecoveryHintRetry
	case isPermanentError(err):
		severity = SeverityCritical
		status = StatusFailing
		hint = RecoveryHintEscalateHuman
	}
	e.Emit(component, severity, status, hint, msg, nil)
}

// isPermanentError is a placeholder. In a real system this would consult the
// tool contract registry or a known error taxonomy.
func isPermanentError(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}

// Collector returns the most recent n events from the backing store.
func (e *Emitter) Collector() *Store {
	if e == nil {
		return nil
	}
	return e.store
}

// TimeOrderedID is a small helper to build deterministic IDs in tests.
func TimeOrderedID(prefix string, t time.Time) string {
	return prefix + "-" + t.Format("20060102-150405-000")
}

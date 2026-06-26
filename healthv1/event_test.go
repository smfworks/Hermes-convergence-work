package apphealthevent

import (
	"encoding/json"
	"testing"
	"time"
)

func TestV1Validate(t *testing.T) {
	base := V1{
		Schema:       TypeV1,
		ID:           "hev-1-2",
		Time:         time.Now().UTC().Format(time.RFC3339),
		Source:       "openclaw-dashboard",
		Component:    ComponentGateway,
		Severity:     SeverityWarning,
		Status:       StatusDegraded,
		RecoveryHint: RecoveryHintRetry,
		Message:      "gateway probe slow",
	}

	cases := []struct {
		name    string
		mutate  func(*V1)
		wantErr string
	}{
		{"valid", func(e *V1) {}, ""},
		{"missing id", func(e *V1) { e.ID = "" }, "id is required"},
		{"missing time", func(e *V1) { e.Time = "" }, "time is required"},
		{"bad time", func(e *V1) { e.Time = "not-rfc3339" }, "time must be RFC3339"},
		{"missing source", func(e *V1) { e.Source = "" }, "source is required"},
		{"missing component", func(e *V1) { e.Component = "" }, "component is required"},
		{"missing severity", func(e *V1) { e.Severity = "" }, "severity is required"},
		{"missing status", func(e *V1) { e.Status = "" }, "status is required"},
		{"missing hint", func(e *V1) { e.RecoveryHint = "" }, "recovery_hint is required"},
		{"missing message", func(e *V1) { e.Message = "" }, "message is required"},
		{"wrong schema", func(e *V1) { e.Schema = "other" }, "schema must be"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := base
			tc.mutate(&e)
			err := e.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !containsStr(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestNew(t *testing.T) {
	e, err := New("openclaw-dashboard", ComponentDashboard, SeverityInfo, StatusHealthy, RecoveryHintNone, "all good")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if e.Schema != TypeV1 {
		t.Fatalf("schema = %q, want %q", e.Schema, TypeV1)
	}
	if e.ID == "" {
		t.Fatalf("id not set")
	}
	if e.Time == "" {
		t.Fatalf("time not set")
	}
}

func TestToCloudEvent(t *testing.T) {
	e := V1{
		Schema:       TypeV1,
		ID:           "hev-123",
		Time:         "2026-06-26T12:00:00Z",
		Source:       "openclaw-dashboard",
		Component:    ComponentGateway,
		Severity:     SeverityWarning,
		Status:       StatusDegraded,
		RecoveryHint: RecoveryHintRetry,
		Message:      "gateway slow",
		Details:      map[string]any{"latency_ms": 5200},
	}
	ce, err := e.ToCloudEvent()
	if err != nil {
		t.Fatalf("ToCloudEvent failed: %v", err)
	}
	if ce.SpecVersion != "1.0" {
		t.Fatalf("specversion = %q, want 1.0", ce.SpecVersion)
	}
	if ce.Type != string(TypeV1) {
		t.Fatalf("type = %q, want %q", ce.Type, TypeV1)
	}
	var decoded V1
	if err := json.Unmarshal(ce.Data, &decoded); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if decoded.ID != e.ID {
		t.Fatalf("decoded id = %q, want %q", decoded.ID, e.ID)
	}
}

func containsStr(s, substr string) bool {
	return len(substr) <= len(s) && (s == substr || len(s) > 0 && containsStrHelper(s, substr))
}

func containsStrHelper(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

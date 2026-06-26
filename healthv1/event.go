// Package apphealthevent implements a small, versioned health-event contract
// for the OpenClaw Dashboard and its consumers.
//
// It intentionally does not import third-party libraries and uses only the
// standard library, matching the rest of the openclaw-dashboard repository.
package apphealthevent

import (
	"encoding/json"
	"fmt"
	"time"
)

// Type identifies the health event schema version. The only supported value
// today is "health_event_v1".
type Type string

const (
	TypeV1 Type = "health_event_v1"
)

// Severity is the human-readable impact level of a health event.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Status is the operational state of the component that emitted the event.
type Status string

const (
	StatusHealthy  Status = "healthy"
	StatusDegraded Status = "degraded"
	StatusFailing  Status = "failing"
	StatusRecovering Status = "recovering"
	StatusRecovered  Status = "recovered"
)

// Component is a namespaced source identifier for the event.
type Component string

const (
	ComponentGateway       Component = "openclaw.gateway"
	ComponentDashboard     Component = "openclaw.dashboard"
	ComponentSystemMetrics Component = "openclaw.system-metrics"
	ComponentRefresh       Component = "openclaw.refresh"
	ComponentChat          Component = "openclaw.chat"
)

// RecoveryHint is a machine-readable recommendation for downstream recovery
// engines. It is advisory only; execution policy lives outside this package.
type RecoveryHint string

const (
	RecoveryHintNone              RecoveryHint = "none"
	RecoveryHintRetry             RecoveryHint = "retry"
	RecoveryHintRestartGateway    RecoveryHint = "restart-gateway"
	RecoveryHintRefreshDashboard  RecoveryHint = "refresh-dashboard"
	RecoveryHintEscalateHuman     RecoveryHint = "escalate-human"
	RecoveryHintSwitchProvider    RecoveryHint = "switch-provider"
)

// V1 is the canonical health_event_v1 payload.
//
// It is intentionally flat and JSON-friendly so it can be embedded in a
// CloudEvents envelope, an OpenTelemetry log record, or consumed directly.
type V1 struct {
	// Schema identifies the contract version. Always "health_event_v1".
	Schema Type `json:"schema"`

	// ID is a unique event identifier. Prefer UUIDv7 or similar.
	ID string `json:"id"`

	// Time is the event timestamp in RFC3339/ISO8601.
	Time string `json:"time"`

	// Source is the emitting platform, e.g. "openclaw-dashboard".
	Source string `json:"source"`

	// Component identifies the failing or reporting subsystem.
	Component Component `json:"component"`

	// Severity is info/warning/critical.
	Severity Severity `json:"severity"`

	// Status is healthy/degraded/failing/recovering/recovered.
	Status Status `json:"status"`

	// RecoveryHint is a machine-readable recommendation.
	RecoveryHint RecoveryHint `json:"recovery_hint"`

	// CorrelationID ties related events (e.g. a degradation and its recovery).
	CorrelationID string `json:"correlation_id,omitempty"`

	// Message is a short human-readable description.
	Message string `json:"message"`

	// Details holds arbitrary structured context. Keep it serializable.
	Details map[string]any `json:"details,omitempty"`
}

// Validate returns a non-nil error if the event is malformed.
func (e V1) Validate() error {
	if e.Schema != TypeV1 {
		return fmt.Errorf("schema must be %q, got %q", TypeV1, e.Schema)
	}
	if e.ID == "" {
		return fmt.Errorf("id is required")
	}
	if e.Time == "" {
		return fmt.Errorf("time is required")
	}
	if _, err := time.Parse(time.RFC3339, e.Time); err != nil {
		return fmt.Errorf("time must be RFC3339: %w", err)
	}
	if e.Source == "" {
		return fmt.Errorf("source is required")
	}
	if e.Component == "" {
		return fmt.Errorf("component is required")
	}
	if e.Severity == "" {
		return fmt.Errorf("severity is required")
	}
	if e.Status == "" {
		return fmt.Errorf("status is required")
	}
	if e.RecoveryHint == "" {
		return fmt.Errorf("recovery_hint is required")
	}
	if e.Message == "" {
		return fmt.Errorf("message is required")
	}
	return nil
}

// CloudEvent wraps the health event in a CloudEvents v1.0 envelope.
// The event itself is placed in the "data" field.
type CloudEvent struct {
	SpecVersion string          `json:"specversion"`
	Type        string          `json:"type"`
	Source      string          `json:"source"`
	ID          string          `json:"id"`
	Time        string          `json:"time"`
	Data        json.RawMessage `json:"data"`
}

// ToCloudEvent returns a CloudEvents v1.0 envelope for this event.
func (e V1) ToCloudEvent() (CloudEvent, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return CloudEvent{}, fmt.Errorf("marshal health event: %w", err)
	}
	return CloudEvent{
		SpecVersion: "1.0",
		Type:        string(TypeV1),
		Source:      e.Source,
		ID:          e.ID,
		Time:        e.Time,
		Data:        b,
	}, nil
}

// New creates a validated health_event_v1 with the schema field pre-filled.
func New(source string, component Component, severity Severity, status Status, hint RecoveryHint, message string) (V1, error) {
	e := V1{
		Schema:       TypeV1,
		ID:           generateID(),
		Time:         time.Now().UTC().Format(time.RFC3339),
		Source:       source,
		Component:    component,
		Severity:     severity,
		Status:       status,
		RecoveryHint: hint,
		Message:      message,
	}
	if err := e.Validate(); err != nil {
		return V1{}, err
	}
	return e, nil
}

// generateID returns a simple time-ordered identifier. Callers that need UUIDs
// can overwrite ID after construction.
func generateID() string {
	return fmt.Sprintf("hev-%d-%06d", time.Now().UnixMilli(), time.Now().Nanosecond()%1_000_000)
}

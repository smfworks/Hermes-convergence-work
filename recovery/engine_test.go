package apprecovery

import (
	"context"
	"testing"
	"time"

	"github.com/smfworks/Hermes-convergence-work/healthv1"
)

func TestEngineProposeRetry(t *testing.T) {
	eng := NewEngine()
	ev := apphealthevent.V1{
		Schema:       apphealthevent.TypeV1,
		ID:           "evt-1",
		Time:         time.Now().Format(time.RFC3339),
		Source:       "openclaw-dashboard",
		Component:    apphealthevent.ComponentRefresh,
		Severity:     apphealthevent.SeverityWarning,
		Status:       apphealthevent.StatusDegraded,
		RecoveryHint: apphealthevent.RecoveryHintRetry,
		Message:      "refresh failed",
	}
	rctx := Context{SessionType: SessionBackgroundCron, UserPresent: false, InFlightMutations: 0}
	proposals, err := eng.Propose(context.Background(), []apphealthevent.V1{ev}, rctx)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if len(proposals) == 0 {
		t.Fatal("expected proposals")
	}
	if proposals[0].Action != ActionRetry {
		t.Fatalf("first action = %q, want retry", proposals[0].Action)
	}
	if proposals[0].Risk != RiskSafe {
		t.Fatalf("first risk = %q, want safe", proposals[0].Risk)
	}
	if !proposals[0].CanAutoExec {
		t.Fatal("safe retry should be auto-executable")
	}
}

func TestEngineHonorsInFlightMutations(t *testing.T) {
	eng := NewEngine()
	eng.now = func() time.Time { return time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC) }
	ev := apphealthevent.V1{
		Schema:       apphealthevent.TypeV1,
		ID:           "evt-1",
		Time:         eng.now().Format(time.RFC3339),
		Source:       "openclaw-dashboard",
		Component:    apphealthevent.ComponentGateway,
		Severity:     apphealthevent.SeverityCritical,
		Status:       apphealthevent.StatusFailing,
		RecoveryHint: apphealthevent.RecoveryHintRestartGateway,
		Message:      "gateway down",
	}
	rctx := Context{SessionType: SessionUserChat, UserPresent: true, InFlightMutations: 2, ActiveChat: true}
	proposals, err := eng.Propose(context.Background(), []apphealthevent.V1{ev}, rctx)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}

	var sawDangerous bool
	for _, p := range proposals {
		if p.Risk == RiskDangerous {
			sawDangerous = true
		}
		if p.Action == ActionRestartGateway && p.CanAutoExec {
			t.Fatal("restart-gateway must not be auto-executable with in-flight mutations")
		}
	}
	if !sawDangerous {
		t.Fatal("expected at least one dangerous proposal when in-flight mutations exist")
	}
}

func TestEngineRecentFailurePattern(t *testing.T) {
	eng := NewEngine()
	base := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	eng.now = func() time.Time { return base }

	events := make([]apphealthevent.V1, 4)
	for i := range events {
		events[i] = apphealthevent.V1{
			Schema:       apphealthevent.TypeV1,
			ID:           "evt-" + string(rune('a'+i)),
			Time:         base.Add(-time.Duration(i) * time.Minute).Format(time.RFC3339),
			Source:       "openclaw-dashboard",
			Component:    apphealthevent.ComponentGateway,
			Severity:     apphealthevent.SeverityWarning,
			Status:       apphealthevent.StatusDegraded,
			RecoveryHint: apphealthevent.RecoveryHintRetry,
			Message:      "transient failure",
		}
	}
	rctx := Context{SessionType: SessionBackgroundCron}
	proposals, err := eng.Propose(context.Background(), events, rctx)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	var sawEscalate bool
	for _, p := range proposals {
		if p.Action == ActionEscalateHuman {
			sawEscalate = true
		}
	}
	if !sawEscalate {
		t.Fatal("expected escalation proposal after repeated failures")
	}
}

func TestEngineSafeForNil(t *testing.T) {
	var eng *Engine
	proposals, err := eng.Propose(context.Background(), nil, Context{})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if len(proposals) != 0 {
		t.Fatalf("expected no proposals, got %d", len(proposals))
	}
}

func TestEngineEmptyEvents(t *testing.T) {
	eng := NewEngine()
	proposals, err := eng.Propose(context.Background(), nil, Context{})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if len(proposals) != 0 {
		t.Fatalf("expected no proposals, got %d", len(proposals))
	}
}

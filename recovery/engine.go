// Package apprecovery implements a context-aware recovery engine for the
// OpenClaw Dashboard. It is intentionally non-executing (shadow-mode): it reads
// health events and proposes recovery actions with a risk score, but the caller
// decides whether to execute them.
//
// This directly addresses convergence gap #3 from the Dr J diagnostic:
// recovery actions must consider session type, user presence, in-flight
// mutations, and recent failure history rather than applying static thresholds.
package apprecovery

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/smfworks/Hermes-convergence-work/healthv1"
)

// Action is a machine-readable recovery recommendation.
type Action string

const (
	ActionNone              Action = "none"
	ActionRetry             Action = "retry"
	ActionRefreshDashboard  Action = "refresh-dashboard"
	ActionRestartGateway    Action = "restart-gateway"
	ActionFlushCache        Action = "flush-cache"
	ActionRebuildIndex      Action = "rebuild-index"
	ActionSwitchProvider    Action = "switch-provider"
	ActionEscalateHuman     Action = "escalate-human"
	ActionTerminateSession  Action = "terminate-session"
)

// RiskLevel classifies how dangerous an action is to execute automatically.
type RiskLevel string

const (
	RiskSafe      RiskLevel = "safe"      // no user-visible side effects
	RiskCautious  RiskLevel = "cautious"  // may interrupt non-critical work
	RiskRisky     RiskLevel = "risky"     // may lose in-flight state
	RiskDangerous RiskLevel = "dangerous" // requires human review
)

// Context models the runtime situation in which a recovery decision is made.
type Context struct {
	// SessionType describes what kind of session is affected.
	SessionType SessionType `json:"session_type"`

	// UserPresent is true when a human is actively interacting with the session.
	UserPresent bool `json:"user_present"`

	// InFlightMutations is the number of uncommitted/unacked state-changing ops.
	InFlightMutations int `json:"in_flight_mutations"`

	// RecentFailureCount is failures in the trailing window (default 10 min).
	RecentFailureCount int `json:"recent_failure_count"`

	// RecentFailureWindow is the duration over which failures are counted.
	RecentFailureWindow time.Duration `json:"-"`

	// ActiveToolCalls is the number of currently running tool calls.
	ActiveToolCalls int `json:"active_tool_calls"`

	// ActiveChat is true when there is an active conversation thread.
	ActiveChat bool `json:"active_chat"`
}

// SessionType enumerates the session classes the engine distinguishes.
type SessionType string

const (
	SessionBackgroundCron SessionType = "background-cron"
	SessionUserChat       SessionType = "user-chat"
	SessionDelegatedTask  SessionType = "delegated-task"
	SessionToolOrchestration SessionType = "tool-orchestration"
)

// Proposal is one recommended recovery action with its rationale.
type Proposal struct {
	Action      Action    `json:"action"`
	Risk        RiskLevel `json:"risk"`
	Confidence  float64   `json:"confidence"` // 0.0-1.0
	Reason      string    `json:"reason"`
	CanAutoExec bool      `json:"can_auto_exec"`
}

// Engine evaluates health events and proposes context-aware recovery actions.
// A nil Engine is safe to use and returns empty proposals.
type Engine struct {
	now func() time.Time
}

// NewEngine creates a recovery engine.
func NewEngine() *Engine {
	return &Engine{now: time.Now}
}

// Propose takes the most recent health events and the current runtime context
// and returns zero or more ranked recovery proposals. Proposals are ordered
// from safest to most dangerous.
func (e *Engine) Propose(ctx context.Context, events []apphealthevent.V1, runtimeCtx Context) ([]Proposal, error) {
	if e == nil {
		return nil, nil
	}
	if len(events) == 0 {
		return nil, nil
	}

	latest := events[0]
	window := runtimeCtx.RecentFailureWindow
	if window <= 0 {
		window = 10 * time.Minute
	}
	recentCount := countRecent(events, e.now().Add(-window))
	if runtimeCtx.RecentFailureCount == 0 {
		runtimeCtx.RecentFailureCount = recentCount
	}

	return buildProposals(latest, runtimeCtx), nil
}

func countRecent(events []apphealthevent.V1, since time.Time) int {
	n := 0
	for _, ev := range events {
		ts, err := time.Parse(time.RFC3339, ev.Time)
		if err != nil {
			continue
		}
		if !ts.Before(since) {
			n++
		}
	}
	return n
}

func buildProposals(ev apphealthevent.V1, ctx Context) []Proposal {
	var proposals []Proposal

	// Map recovery hints from the event to concrete actions first.
	switch ev.RecoveryHint {
	case apphealthevent.RecoveryHintRetry:
		proposals = append(proposals, action(ActionRetry, RiskSafe,
			"event recommends retry"))
	case apphealthevent.RecoveryHintRestartGateway:
		proposals = append(proposals, action(ActionRestartGateway, RiskRisky,
			"event recommends gateway restart"))
	case apphealthevent.RecoveryHintRefreshDashboard:
		proposals = append(proposals, action(ActionRefreshDashboard, RiskCautious,
			"event recommends dashboard refresh"))
	case apphealthevent.RecoveryHintEscalateHuman:
		proposals = append(proposals, action(ActionEscalateHuman, RiskDangerous,
			"event recommends human escalation"))
	case apphealthevent.RecoveryHintSwitchProvider:
		proposals = append(proposals, action(ActionSwitchProvider, RiskCautious,
			"event recommends provider switch"))
	}

	// Add contextual override proposals based on runtime state.
	if ctx.InFlightMutations > 0 {
		proposals = append(proposals, action(ActionEscalateHuman, RiskDangerous,
			fmt.Sprintf("%d in-flight mutation(s); automated restart/flush could lose state", ctx.InFlightMutations)))
	}

	if ctx.UserPresent && ctx.ActiveChat {
		proposals = append(proposals, action(ActionNone, RiskSafe,
			"user is present in active chat; prefer transparent retry over disruptive action"))
	}

	if ctx.RecentFailureCount >= 3 {
		proposals = append(proposals, action(ActionEscalateHuman, RiskDangerous,
			fmt.Sprintf("%d recent failures in window; pattern suggests manual review", ctx.RecentFailureCount)))
	}

	if ctx.ActiveToolCalls > 0 && ev.Component == apphealthevent.ComponentGateway {
		proposals = append(proposals, action(ActionSwitchProvider, RiskCautious,
			"active tool calls depend on gateway; switching provider may preserve session"))
	}

	if len(proposals) == 0 {
		proposals = append(proposals, action(ActionNone, RiskSafe,
			"no specific recovery action indicated"))
	}

	// Score each proposal.
	for i := range proposals {
		scoreProposal(&proposals[i], ev, ctx)
	}

	// Sort safest-first, then by descending confidence.
	sort.SliceStable(proposals, func(i, j int) bool {
		ri, rj := riskWeight(proposals[i].Risk), riskWeight(proposals[j].Risk)
		if ri != rj {
			return ri < rj
		}
		return proposals[i].Confidence > proposals[j].Confidence
	})

	return proposals
}

func action(a Action, r RiskLevel, reason string) Proposal {
	return Proposal{Action: a, Risk: r, Reason: reason}
}

func riskWeight(r RiskLevel) int {
	switch r {
	case RiskSafe:
		return 0
	case RiskCautious:
		return 1
	case RiskRisky:
		return 2
	case RiskDangerous:
		return 3
	}
	return 4
}

func scoreProposal(p *Proposal, ev apphealthevent.V1, ctx Context) {
	base := 0.5

	// Severity drives urgency.
	switch ev.Severity {
	case apphealthevent.SeverityCritical:
		base += 0.3
	case apphealthevent.SeverityWarning:
		base += 0.1
	}

	// Risk level reduces confidence in auto-execution.
	switch p.Risk {
	case RiskSafe:
		base += 0.2
	case RiskCautious:
		base -= 0.1
	case RiskRisky:
		base -= 0.25
	case RiskDangerous:
		base -= 0.4
	}

	// Context modifiers.
	if ctx.UserPresent && (p.Action == ActionTerminateSession || p.Action == ActionRestartGateway) {
		base -= 0.3
	}
	if ctx.InFlightMutations > 0 && p.Risk != RiskSafe {
		base -= 0.3
	}
	if ctx.RecentFailureCount >= 3 && p.Action != ActionEscalateHuman {
		base -= 0.15
	}

	p.Confidence = clamp(base, 0.0, 1.0)
	p.CanAutoExec = p.Risk == RiskSafe || (p.Risk == RiskCautious && ctx.InFlightMutations == 0 && !ctx.UserPresent)
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

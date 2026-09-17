package policy

import (
	"testing"
	"time"
)

func TestDecisionEngine_ManualLock(t *testing.T) {
	engine := NewDecisionEngine()
	now := time.Now()

	p := DefaultSwitchPolicy()
	p.AutoSwitchEnabled = true
	p.LockedNode = "HK-VIP"

	state := &DecisionState{
		CurrentNode: "HK-01",
	}

	evals := []NodeEvaluation{
		{Name: "HK-01", Available: true, RTT: 50 * time.Millisecond},
		{Name: "HK-VIP", Available: true, RTT: 100 * time.Millisecond},
	}

	res := engine.Evaluate(now, p, state, evals)
	if !res.ShouldSwitch || res.TargetNode != "HK-VIP" || res.TriggerType != "manual_override" {
		t.Fatalf("expected manual override switch to HK-VIP, got %+v", res)
	}

	// Once switched, it should remain locked and not switch
	state.CurrentNode = "HK-VIP"
	res2 := engine.Evaluate(now, p, state, evals)
	if res2.ShouldSwitch {
		t.Fatalf("expected no switch when already on locked node, got %+v", res2)
	}
}

func TestDecisionEngine_UrgentFailover(t *testing.T) {
	engine := NewDecisionEngine()
	now := time.Now()

	p := DefaultSwitchPolicy()
	p.AutoSwitchEnabled = true
	p.MaxConsecutiveFailures = 3
	p.CooldownDuration = 10 * time.Minute

	// Even if in cooldown, emergency failover should ignore cooldown!
	state := &DecisionState{
		CurrentNode:         "HK-01",
		LastSwitchAt:        now.Add(-1 * time.Minute), // Inside cooldown
		ConsecutiveFailures: 3,
	}

	evals := []NodeEvaluation{
		{Name: "HK-01", Available: false, RTT: 0, TriageStatus: "failed"},
		{Name: "SG-01", Available: true, RTT: 65 * time.Millisecond, TriageStatus: "stable"},
		{Name: "JP-01", Available: true, RTT: 45 * time.Millisecond, TriageStatus: "stable"},
	}

	res := engine.Evaluate(now, p, state, evals)
	if !res.ShouldSwitch || res.TargetNode != "JP-01" || res.TriggerType != "failure_failover" {
		t.Fatalf("expected urgent failover to JP-01, got %+v", res)
	}
}

func TestDecisionEngine_CooldownAndThreshold(t *testing.T) {
	engine := NewDecisionEngine()
	now := time.Now()

	p := DefaultSwitchPolicy()
	p.AutoSwitchEnabled = true
	p.CooldownDuration = 5 * time.Minute
	p.MinImprovementRTT = 30 * time.Millisecond
	p.MinImprovementRatio = 0.20

	state := &DecisionState{
		CurrentNode:         "HK-01",
		LastSwitchAt:        now.Add(-2 * time.Minute), // 2 min ago < 5 min cooldown
		ConsecutiveFailures: 0,
	}

	evals := []NodeEvaluation{
		{Name: "HK-01", Available: true, RTT: 100 * time.Millisecond},
		{Name: "HK-FAST", Available: true, RTT: 50 * time.Millisecond}, // 50% faster, but blocked by cooldown
	}

	res := engine.Evaluate(now, p, state, evals)
	if res.ShouldSwitch {
		t.Fatalf("expected switch blocked by cooldown, got %+v", res)
	}

	// Advance time beyond cooldown
	nowAfterCooldown := now.Add(6 * time.Minute)

	// Case A: Insufficient improvement (HK-01 is 100ms, candidate is 85ms -> diff is 15ms < 30ms threshold)
	evalsMarginal := []NodeEvaluation{
		{Name: "HK-01", Available: true, RTT: 100 * time.Millisecond},
		{Name: "HK-SLIGHT", Available: true, RTT: 85 * time.Millisecond},
	}
	resMarginal := engine.Evaluate(nowAfterCooldown, p, state, evalsMarginal)
	if resMarginal.ShouldSwitch {
		t.Fatalf("expected marginal improvement to be rejected, got %+v", resMarginal)
	}

	// Case B: Substantial improvement (100ms vs 50ms -> diff 50ms >= 30ms, ratio 50% >= 20%)
	resSubstantial := engine.Evaluate(nowAfterCooldown, p, state, evals)
	if !resSubstantial.ShouldSwitch || resSubstantial.TargetNode != "HK-FAST" || resSubstantial.TriggerType != "latency_improvement" {
		t.Fatalf("expected latency improvement switch to HK-FAST, got %+v", resSubstantial)
	}
}

func TestDecisionEngine_RecordSwitchAndRollback(t *testing.T) {
	engine := NewDecisionEngine()
	state := &DecisionState{
		CurrentNode: "HK-01",
	}

	engine.RecordSwitch(state, "HK-01", "SG-01", "PROXY", "Latency improvement", "latency_improvement", 120*time.Millisecond, 60*time.Millisecond)

	if state.CurrentNode != "SG-01" || state.PreviousNode != "HK-01" {
		t.Fatalf("state mismatch after switch: %+v", state)
	}
	if len(state.AuditTrail) != 1 || state.AuditTrail[0].Status != "success" {
		t.Fatalf("audit trail mismatch: %+v", state.AuditTrail)
	}

	// Simulate immediate failure of SG-01 -> trigger rollback
	engine.RecordRollback(state, "PROXY", "SG-01", "DNS resolution failed")

	if state.CurrentNode != "HK-01" {
		t.Fatalf("expected current node to rollback to HK-01, got %s", state.CurrentNode)
	}
	if len(state.AuditTrail) != 2 || state.AuditTrail[1].Status != "rolled_back" {
		t.Fatalf("expected 2 audit events with rollback status, got %+v", state.AuditTrail)
	}
}

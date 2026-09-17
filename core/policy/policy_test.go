package policy

import (
	"testing"
	"time"
)

func TestDecisionEngine_Modes(t *testing.T) {
	engine := NewDecisionEngine()
	now := time.Now()

	state := &DecisionState{CurrentNode: "HK-01"}
	evals := []NodeEvaluation{
		{
			Name:              "HK-01",
			Available:         true,
			RTT:               100 * time.Millisecond,
			SampleCount:       5,
			LastSampleTime:    now,
			ObservationWindow: 2 * time.Minute,
		},
		{
			Name:              "HK-FAST",
			Available:         true,
			RTT:               40 * time.Millisecond,
			SampleCount:       5,
			LastSampleTime:    now,
			ObservationWindow: 2 * time.Minute,
		},
	}

	// 1. Default Mode: Monitor Only -> MUST NEVER SWITCH
	pDefault := DefaultSwitchPolicy()
	if pDefault.Mode != ModeMonitorOnly {
		t.Fatalf("expected default mode to be ModeMonitorOnly, got %s", pDefault.Mode)
	}
	resMon := engine.Evaluate(now, pDefault, state, evals)
	if resMon.ShouldSwitch {
		t.Fatalf("Monitor Only mode must NEVER switch")
	}

	// 2. Recommend Mode -> Must produce recommendation but NOT switch
	pRec := DefaultSwitchPolicy()
	pRec.Mode = ModeRecommend
	pRec.CooldownDuration = 0

	resRec := engine.Evaluate(now, pRec, state, evals)
	if resRec.ShouldSwitch {
		t.Fatalf("Recommend mode must NOT automatically switch")
	}
	if resRec.Recommendation == nil || resRec.Recommendation.TargetNode != "HK-FAST" {
		t.Fatalf("expected recommendation for HK-FAST, got %+v", resRec.Recommendation)
	}

	// 3. Auto Mode -> Executes switch
	pAuto := DefaultSwitchPolicy()
	pAuto.Mode = ModeAuto
	pAuto.CooldownDuration = 0

	resAuto := engine.Evaluate(now, pAuto, state, evals)
	if !resAuto.ShouldSwitch || resAuto.TargetNode != "HK-FAST" {
		t.Fatalf("Auto mode must switch to HK-FAST, got %+v", resAuto)
	}
}

func TestDecisionEngine_SampleFreshness(t *testing.T) {
	engine := NewDecisionEngine()
	now := time.Now()

	p := DefaultSwitchPolicy()
	p.Mode = ModeAuto
	p.CooldownDuration = 0
	p.MaxSampleAge = 5 * time.Minute
	p.MinSampleCount = 3

	state := &DecisionState{CurrentNode: "HK-01"}

	// Case 1: Candidate sample is too old (6 minutes old)
	evalsOld := []NodeEvaluation{
		{
			Name:              "HK-01",
			Available:         true,
			RTT:               100 * time.Millisecond,
			SampleCount:       5,
			LastSampleTime:    now,
			ObservationWindow: 5 * time.Minute,
		},
		{
			Name:              "HK-STALE",
			Available:         true,
			RTT:               40 * time.Millisecond,
			SampleCount:       5,
			LastSampleTime:    now.Add(-6 * time.Minute), // Stale!
			ObservationWindow: 5 * time.Minute,
		},
	}
	resOld := engine.Evaluate(now, p, state, evalsOld)
	if resOld.ShouldSwitch {
		t.Fatalf("should not switch to stale candidate")
	}
	if !resOld.RequiresFreshProbe {
		t.Fatalf("expected RequiresFreshProbe=true for stale candidate")
	}

	// Case 2: Candidate sample count is insufficient (only 1 sample < 3)
	evalsFew := []NodeEvaluation{
		{
			Name:              "HK-01",
			Available:         true,
			RTT:               100 * time.Millisecond,
			SampleCount:       5,
			LastSampleTime:    now,
			ObservationWindow: 5 * time.Minute,
		},
		{
			Name:              "HK-FEW",
			Available:         true,
			RTT:               40 * time.Millisecond,
			SampleCount:       1, // Insufficient!
			LastSampleTime:    now,
			ObservationWindow: 10 * time.Second,
		},
	}
	resFew := engine.Evaluate(now, p, state, evalsFew)
	if resFew.ShouldSwitch {
		t.Fatalf("should not switch to candidate with insufficient sample count")
	}
	if !resFew.RequiresFreshProbe {
		t.Fatalf("expected RequiresFreshProbe=true for sparse candidate")
	}
}

func TestDecisionEngine_VerificationAndRollback(t *testing.T) {
	engine := NewDecisionEngine()

	// Case 1: 1 transient failure out of 3 -> Should NOT rollback
	probesTransient := []VerificationProbe{
		{Index: 1, Success: false, Error: "connection timeout"},
		{Index: 2, Success: true, RTT: 45 * time.Millisecond},
		{Index: 3, Success: true, RTT: 48 * time.Millisecond},
	}
	resTransient := engine.EvaluateVerification(probesTransient, false, 2)
	if resTransient.ShouldRollback {
		t.Fatalf("single transient failure must NOT trigger rollback")
	}

	// Case 2: 2 failures out of 3 (reaches threshold 2) -> MUST rollback
	probesMajorityFail := []VerificationProbe{
		{Index: 1, Success: false, Error: "connection timeout"},
		{Index: 2, Success: true, RTT: 45 * time.Millisecond},
		{Index: 3, Success: false, Error: "connection reset"},
	}
	resMajority := engine.EvaluateVerification(probesMajorityFail, false, 2)
	if !resMajority.ShouldRollback {
		t.Fatalf("majority failure must trigger rollback")
	}

	// Case 3: Explicit region block -> MUST rollback immediately regardless of probe count
	resBlocked := engine.EvaluateVerification(nil, true, 2)
	if !resBlocked.ShouldRollback {
		t.Fatalf("explicit region block must trigger immediate rollback")
	}
}

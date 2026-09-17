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

func TestDecisionEngine_ModeBoundariesAndLocking(t *testing.T) {
	engine := NewDecisionEngine()
	now := time.Now()

	// Base evaluations where HK-01 is failing (unavailable), and HK-FAST is available
	evalsFailure := []NodeEvaluation{
		{
			Name:              "HK-01",
			Available:         false,
			TriageStatus:      "failed",
			SampleCount:       5,
			LastSampleTime:    now,
			ObservationWindow: 2 * time.Minute,
		},
		{
			Name:              "HK-FAST",
			Available:         true,
			RTT:               35 * time.Millisecond,
			SampleCount:       5,
			LastSampleTime:    now,
			ObservationWindow: 2 * time.Minute,
		},
	}

	tests := []struct {
		name                 string
		mode                 OrchestratorMode
		lockedNode           string
		stateFailures        int
		expectShouldSwitch   bool
		expectRecommendation bool
		expectTargetNode     string
	}{
		{
			name:                 "monitor_only + locked",
			mode:                 ModeMonitorOnly,
			lockedNode:           "HK-LOCKED",
			stateFailures:        0,
			expectShouldSwitch:   false,
			expectRecommendation: false,
		},
		{
			name:                 "recommend + locked",
			mode:                 ModeRecommend,
			lockedNode:           "HK-LOCKED",
			stateFailures:        0,
			expectShouldSwitch:   false,
			expectRecommendation: false,
		},
		{
			name:                 "auto + locked",
			mode:                 ModeAuto,
			lockedNode:           "HK-LOCKED",
			stateFailures:        0,
			expectShouldSwitch:   false,
			expectRecommendation: false,
		},
		{
			name:                 "monitor_only + failure",
			mode:                 ModeMonitorOnly,
			lockedNode:           "",
			stateFailures:        5,
			expectShouldSwitch:   false,
			expectRecommendation: false,
		},
		{
			name:                 "recommend + failure",
			mode:                 ModeRecommend,
			lockedNode:           "",
			stateFailures:        5,
			expectShouldSwitch:   false,
			expectRecommendation: true,
			expectTargetNode:     "HK-FAST",
		},
		{
			name:                 "auto + failure",
			mode:                 ModeAuto,
			lockedNode:           "",
			stateFailures:        5,
			expectShouldSwitch:   true,
			expectRecommendation: false,
			expectTargetNode:     "HK-FAST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := DefaultSwitchPolicy()
			p.Mode = tt.mode
			p.LockedNode = tt.lockedNode
			p.CooldownDuration = 0

			state := &DecisionState{
				CurrentNode:         "HK-01",
				ConsecutiveFailures: tt.stateFailures,
			}

			res := engine.Evaluate(now, p, state, evalsFailure)

			if res.ShouldSwitch != tt.expectShouldSwitch {
				t.Errorf("%s: ShouldSwitch = %v, expected %v", tt.name, res.ShouldSwitch, tt.expectShouldSwitch)
			}
			hasRec := res.Recommendation != nil
			if hasRec != tt.expectRecommendation {
				t.Errorf("%s: hasRecommendation = %v, expected %v", tt.name, hasRec, tt.expectRecommendation)
			}
			if tt.expectShouldSwitch && res.TargetNode != tt.expectTargetNode {
				t.Errorf("%s: TargetNode = %s, expected %s", tt.name, res.TargetNode, tt.expectTargetNode)
			}
			if tt.expectRecommendation && res.Recommendation.TargetNode != tt.expectTargetNode {
				t.Errorf("%s: Recommendation.TargetNode = %s, expected %s", tt.name, res.Recommendation.TargetNode, tt.expectTargetNode)
			}
		})
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

	// Case 3: Explicit region block under PurposeGeneral -> must NOT trigger rollback (not a hard failure for general traffic)
	resBlockedGeneral := engine.EvaluateVerification(nil, true, 2, PurposeGeneral)
	if resBlockedGeneral.ShouldRollback {
		t.Fatalf("explicit region block under PurposeGeneral must NOT trigger rollback")
	}

	// Case 4: Explicit region block under PurposeAI -> MUST trigger immediate rollback
	resBlockedAI := engine.EvaluateVerification(nil, true, 2, PurposeAI)
	if !resBlockedAI.ShouldRollback {
		t.Fatalf("explicit region block under PurposeAI must trigger immediate rollback")
	}
}

func TestDecisionEngine_PolicyPurpose(t *testing.T) {
	engine := NewDecisionEngine()
	now := time.Now()

	state := &DecisionState{CurrentNode: "HK-01"}

	// HK-01 is available for general internet, but blocked on Google/AI (e.g. Gemini HTTP 400)
	evals := []NodeEvaluation{
		{
			Name:              "HK-01",
			Available:         true,
			TriageStatus:      "blocked", // AI/Google region-blocked
			RTT:               50 * time.Millisecond,
			SampleCount:       5,
			LastSampleTime:    now,
			ObservationWindow: 2 * time.Minute,
		},
		{
			Name:              "HK-AI-PASS",
			Available:         true,
			TriageStatus:      "pass",
			RTT:               60 * time.Millisecond,
			SampleCount:       5,
			LastSampleTime:    now,
			ObservationWindow: 2 * time.Minute,
		},
		{
			Name:              "HK-AI-BLOCKED",
			Available:         true,
			TriageStatus:      "blocked",
			RTT:               30 * time.Millisecond, // Lower RTT but blocked on AI
			SampleCount:       5,
			LastSampleTime:    now,
			ObservationWindow: 2 * time.Minute,
		},
	}

	// 1. General Purpose Policy: AI regional block is NOT a hard failure
	pGeneral := DefaultSwitchPolicy()
	pGeneral.Mode = ModeAuto
	pGeneral.Purpose = PurposeGeneral
	pGeneral.CooldownDuration = 0

	resGeneral := engine.Evaluate(now, pGeneral, state, evals)
	if resGeneral.TriggerType == "failure_failover" {
		t.Fatalf("General policy must NOT trigger failure_failover for AI regional block")
	}

	// 2. AI Purpose Policy: AI regional block IS a hard failure -> triggers urgent failover
	pAI := DefaultSwitchPolicy()
	pAI.Mode = ModeAuto
	pAI.Purpose = PurposeAI
	pAI.CooldownDuration = 0

	resAI := engine.Evaluate(now, pAI, state, evals)
	if !resAI.ShouldSwitch || resAI.TriggerType != "failure_failover" {
		t.Fatalf("AI policy must trigger failure_failover when current node is region blocked on AI, got: %+v", resAI)
	}
	// HK-AI-BLOCKED has lower RTT (30ms) but is blocked on AI, so HK-AI-PASS (60ms) must be chosen!
	if resAI.TargetNode != "HK-AI-PASS" {
		t.Fatalf("AI policy must NOT select candidate that is blocked on AI (expected HK-AI-PASS, got %s)", resAI.TargetNode)
	}
}

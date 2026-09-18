package application

import (
	"context"
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/policy"
	"github.com/faceair/clash-speedtest/core/profiles"
)

// =============================================================================
// Final remediation regressions.
//
//  1. An unknown configured orchestrator mode must be rejected, never fail-open into an
//     evaluating mode.
//  5. A candidate_node_keys request that contains an unmatched key must be rejected, never
//     silently accepted with the unmatched key dropped.
// =============================================================================

// --- 1. unknown mode -------------------------------------------------------

// newPolicyTestService builds a minimal AppService for policy-boundary tests. It needs no
// store: UpdateSwitchPolicy only touches the policy and the emitter.
func newPolicyTestService() *AppService {
	return NewAppService(nil, profiles.Paths{}, NewMemoryEventEmitter())
}

// TestUpdateSwitchPolicy_RejectsUnknownMode pins the write-boundary rule: an unimplemented
// mode must not be storable. Before the fix, "Monitor_Only" was accepted and persisted, and
// every later evaluation silently ran under ModeRecommend.
func TestUpdateSwitchPolicy_RejectsUnknownMode(t *testing.T) {
	for _, bad := range []policy.OrchestratorMode{
		"Monitor_Only", "MONITOR-ONLY", "monitor", "monitor only", "auto_switch", "nonsense", "RECOMMEND",
	} {
		svc := newPolicyTestService()
		err := svc.UpdateSwitchPolicy(context.Background(), policy.SwitchPolicy{Mode: bad})
		if err == nil {
			t.Errorf("mode %q was accepted by UpdateSwitchPolicy; it must be rejected", bad)
			continue
		}
		if !monitor.IsValidationError(err) {
			t.Errorf("mode %q: expected a validation error (→HTTP 400), got %v", bad, err)
		}
		if !strings.Contains(err.Error(), string(bad)) {
			t.Errorf("mode %q: the error must name the offending value, got %q", bad, err.Error())
		}
	}
}

// TestUpdateSwitchPolicy_AcceptsImplementedModes is the positive control: the three real
// modes and the empty (unset) default must all be accepted, with empty normalized to the
// safe default monitor_only.
func TestUpdateSwitchPolicy_AcceptsImplementedModes(t *testing.T) {
	for _, mode := range []policy.OrchestratorMode{
		policy.ModeMonitorOnly, policy.ModeRecommend, policy.ModeAuto,
	} {
		svc := newPolicyTestService()
		if err := svc.UpdateSwitchPolicy(context.Background(), policy.SwitchPolicy{Mode: mode}); err != nil {
			t.Errorf("mode %q must be accepted, got %v", mode, err)
		}
		if svc.policy.Mode != mode {
			t.Errorf("mode %q was not stored verbatim, got %q", mode, svc.policy.Mode)
		}
	}

	svc := newPolicyTestService()
	if err := svc.UpdateSwitchPolicy(context.Background(), policy.SwitchPolicy{}); err != nil {
		t.Fatalf("an empty mode must be accepted (it is an unset default), got %v", err)
	}
	if svc.policy.Mode != policy.ModeMonitorOnly {
		t.Errorf("an empty mode must normalize to monitor_only, got %q", svc.policy.Mode)
	}
}

// TestNormalizeOrchestratorMode covers the shared normalizer used by both boundaries.
func TestNormalizeOrchestratorMode(t *testing.T) {
	got, err := policy.NormalizeOrchestratorMode("")
	if err != nil || got != policy.ModeMonitorOnly {
		t.Errorf(`NormalizeOrchestratorMode("") = (%q, %v), want (%q, nil)`, got, err, policy.ModeMonitorOnly)
	}
	for _, mode := range []policy.OrchestratorMode{
		policy.ModeMonitorOnly, policy.ModeRecommend, policy.ModeAuto,
	} {
		got, err := policy.NormalizeOrchestratorMode(mode)
		if err != nil || got != mode {
			t.Errorf("NormalizeOrchestratorMode(%q) = (%q, %v), want (%q, nil)", mode, got, err, mode)
		}
	}
	for _, bad := range []policy.OrchestratorMode{"Monitor_Only", "nonsense", "monitor only"} {
		if _, err := policy.NormalizeOrchestratorMode(bad); err == nil {
			t.Errorf("NormalizeOrchestratorMode(%q) must fail", bad)
		}
	}
}

// --- 5. candidate_node_keys ------------------------------------------------

func candidateNodes() []policy.EvidenceNode {
	return []policy.EvidenceNode{
		{ProfileID: "p", NodeKey: "nk_a", NodeIdentityKey: "nid_a", ConfigRevisionKey: "r", DisplayName: "A"},
		{ProfileID: "p", NodeKey: "nk_b", NodeIdentityKey: "nid_b", ConfigRevisionKey: "r", DisplayName: "B"},
	}
}

// TestResolveCandidateNames_RejectsPartiallyMatchedKeys is the exact scenario under review:
// candidate_node_keys = [valid_key, invalid_key]. It must be a validation error, NOT a 200
// that silently keeps the valid node and drops the invalid one.
func TestResolveCandidateNames_RejectsPartiallyMatchedKeys(t *testing.T) {
	p := policy.DefaultSwitchPolicy()

	names, err := resolveCandidateNames(p, candidateNodes(), []string{"nk_a", "nk_does_not_exist"})
	if err == nil {
		t.Fatalf("a request containing an unmatched key must be rejected; got names=%v", names)
	}
	if !monitor.IsValidationError(err) {
		t.Errorf("expected a validation error (→HTTP 400), got %v", err)
	}
	if !strings.Contains(err.Error(), "nk_does_not_exist") {
		t.Errorf("the error must name the unmatched key, got %q", err.Error())
	}
	if names != nil {
		t.Errorf("no partial result may be returned alongside the error, got %v", names)
	}
}

// TestResolveCandidateNames_AllKeysMustMatch: every key in the request must resolve, in any
// position.
func TestResolveCandidateNames_AllKeysMustMatch(t *testing.T) {
	p := policy.DefaultSwitchPolicy()

	for _, req := range [][]string{
		{"nk_a", "nope"},
		{"nope", "nk_a"},
		{"nk_a", "nk_b", "nope"},
		{"nope1", "nope2"},
	} {
		if _, err := resolveCandidateNames(p, candidateNodes(), req); err == nil {
			t.Errorf("request %v must be rejected", req)
		}
	}
}

// TestResolveCandidateNames_AcceptsFullyMatchedKeys is the positive control.
func TestResolveCandidateNames_AcceptsFullyMatchedKeys(t *testing.T) {
	p := policy.DefaultSwitchPolicy()

	names, err := resolveCandidateNames(p, candidateNodes(), []string{"nk_a"})
	if err != nil {
		t.Fatalf("a fully matched request must succeed, got %v", err)
	}
	if len(names) != 1 || names[0] != "A" {
		t.Errorf("expected [A], got %v", names)
	}

	// Identity keys are matched too.
	names, err = resolveCandidateNames(p, candidateNodes(), []string{"nid_b"})
	if err != nil {
		t.Fatalf("identity-key matching must work, got %v", err)
	}
	if len(names) != 1 || names[0] != "B" {
		t.Errorf("expected [B], got %v", names)
	}

	// No request at all means "no narrowing".
	names, err = resolveCandidateNames(p, candidateNodes(), nil)
	if err != nil || names != nil {
		t.Errorf("an empty request must return (nil, nil), got (%v, %v)", names, err)
	}
}

// TestResolveCandidateNames_WhitelistExclusionIsNotSilentlyWidened: a key that resolves to a
// real node which the policy whitelist excludes is not an "unmatched key", but it must still
// not degrade into "no narrowing" — returning (nil, nil) would widen the candidate set back
// to every whitelisted node, the opposite of the request. It is an error naming the cause.
func TestResolveCandidateNames_WhitelistExclusionIsNotSilentlyWidened(t *testing.T) {
	p := policy.DefaultSwitchPolicy()
	p.CandidateNodes = []string{"B"} // only B is whitelisted

	names, err := resolveCandidateNames(p, candidateNodes(), []string{"nk_a"})
	if err == nil {
		t.Fatalf("requesting a node the policy excludes must not widen back to all nodes; got names=%v", names)
	}
	if !monitor.IsValidationError(err) {
		t.Errorf("expected a validation error (→HTTP 400), got %v", err)
	}
	if names != nil {
		t.Errorf("no partial result may be returned alongside the error, got %v", names)
	}

	// Control: requesting the whitelisted node still works.
	names, err = resolveCandidateNames(p, candidateNodes(), []string{"nk_b"})
	if err != nil {
		t.Fatalf("requesting the whitelisted node must succeed, got %v", err)
	}
	if len(names) != 1 || names[0] != "B" {
		t.Errorf("expected [B], got %v", names)
	}
}

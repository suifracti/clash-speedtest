package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/policy"
)

// =============================================================================
// PR#7 — Monitor Evidence → Recommendation wiring.
//
// This file is the ONLY place where persisted monitor samples are read for the
// purpose of producing a recommendation. It is strictly read-only:
//
//   - it never calls controller.SelectNode / TestDelay
//   - it never mutates AppService.policy or AppService.decisionState
//   - it never starts, stops, pauses or triggers a monitor job
//
// The heavy lifting (freshness gate, purpose semantics, decision) lives in
// core/policy; this file only projects persistence into policy.EvidenceSample.
//
// Evidence isolation (all three keys must agree, see policy/evidence.go):
//
//	ProfileID         — required, otherwise the same physical endpoint observed under
//	                    two subscriptions would be aggregated (fingerprint.go R-01).
//	NodeIdentityKey   — the transport endpoint we query by (spans config revisions).
//	ConfigRevisionKey — the config/credential revision, filtered by the policy layer.
//
// The observation window is resolved by policy.DefaultEvidenceWindow and is NOT
// derived from MaxSampleAge: MaxSampleAge is a freshness gate on the newest sample,
// not a statement about how much history is meaningful.
// =============================================================================

const (
	// evidenceSampleLimitPerNode bounds how many raw samples per node are projected into
	// evidence. Newest-first, so the freshness window is always fully covered.
	evidenceSampleLimitPerNode = 2000

	// evidenceDiscoverySampleLimit bounds the node-discovery scan when no live monitor job
	// is available (e.g. after an application restart, since job definitions are in-memory).
	evidenceDiscoverySampleLimit = 10000

	// evidenceDiscoveryMaxWindow caps the node-discovery scan window independently of the
	// evidence window, so discovery stays cheap even with a long evidence window.
	evidenceDiscoveryMaxWindow = time.Hour
)

// MonitorRecommendationRequest selects the evidence window to evaluate.
type MonitorRecommendationRequest struct {
	// JobID identifies the monitor job whose node set defines the candidate universe.
	// When empty, the node set is discovered from persisted raw samples instead.
	JobID string `json:"job_id,omitempty"`

	// CurrentNodeKey / CurrentNodeIdentityKey explicitly identify the active node.
	// When both are empty the service falls back to the orchestrator's in-memory current
	// node, then to the external controller's current selector value (a read-only call).
	CurrentNodeKey         string `json:"current_node_key,omitempty"`
	CurrentNodeIdentityKey string `json:"current_node_identity_key,omitempty"`

	// Purpose selects the evaluation lens: "general" or "ai".
	// Empty means "use the configured policy purpose".
	Purpose string `json:"purpose,omitempty"`

	// CandidateNodeKeys optionally narrows the candidate set to specific node keys.
	// The current node is always evaluated.
	CandidateNodeKeys []string `json:"candidate_node_keys,omitempty"`

	// Preview explicitly requests a "what would the policy say" preview even when the
	// configured mode would suppress recommendations (i.e. monitor_only).
	//
	// Default false: the configured mode is respected and no recommendation is produced
	// under monitor_only. This is an explicit opt-in so the configured mode is never
	// silently bypassed.
	Preview bool `json:"preview,omitempty"`
}

// GetMonitorRecommendation produces a read-only, evidence-backed recommendation.
//
// It never switches anything: the returned recommendation always reports
// SelectNodeCalls == 0 and Executed == false.
func (s *AppService) GetMonitorRecommendation(
	ctx context.Context,
	req MonitorRecommendationRequest,
) (*policy.MonitorRecommendation, error) {
	if s.historyStore == nil {
		return nil, fmt.Errorf("monitor evidence store is not initialized")
	}

	// 1. Snapshot the policy and orchestrator state. Read-only: RLock, never mutated.
	s.ctrlMu.RLock()
	pol := s.policy
	state := s.decisionState
	s.ctrlMu.RUnlock()

	if pol.Mode == "" {
		pol.Mode = policy.ModeMonitorOnly
	}

	purpose := pol.Purpose
	if purpose == "" {
		purpose = policy.PurposeGeneral
	}
	if strings.TrimSpace(req.Purpose) != "" {
		parsed, err := parsePolicyPurpose(req.Purpose)
		if err != nil {
			return nil, monitor.WrapValidationError(err)
		}
		purpose = parsed
	}

	now := time.Now()
	// The observation window is resolved by the policy layer from the policy's own
	// observation requirements — deliberately NOT from MaxSampleAge.
	evidenceWindow := policy.DefaultEvidenceWindow(pol)
	since := now.Add(-evidenceWindow)

	// 2. Resolve the node universe (live job definition, or persisted samples).
	nodes, source, truncated, err := s.collectEvidenceNodes(ctx, req.JobID, since, now)
	if err != nil {
		return nil, err
	}

	// 3. Resolve the current node (request → orchestrator state → controller selection).
	current, hasCurrent := s.resolveEvidenceCurrentNode(ctx, req, nodes)

	// 4. Project raw samples into neutral evidence samples, one query per node identity.
	samplesByNode, rawTotal, samplesTruncated := s.collectEvidenceSamples(ctx, nodes, since)

	// 5. Build the evidence snapshot and hand it to the existing policy engine.
	snap := policy.BuildEvidenceSnapshot(policy.EvidenceInput{
		Now:                    now,
		Purpose:                purpose,
		Policy:                 pol,
		Source:                 source,
		EvidenceWindow:         evidenceWindow,
		CurrentNodeKey:         nodeKeyOf(current, hasCurrent),
		CurrentNodeIdentityKey: identityKeyOf(current, hasCurrent),
		Nodes:                  nodes,
		SamplesByNode:          samplesByNode,
		RawSampleCount:         rawTotal,
		Truncated:              truncated || samplesTruncated,
	})

	evalPolicy := pol
	candidateNames, err := resolveCandidateNames(pol, nodes, req.CandidateNodeKeys)
	if err != nil {
		return nil, err
	}
	if candidateNames != nil {
		evalPolicy.CandidateNodes = candidateNames
	}

	engine := s.decisionEngine
	if engine == nil {
		engine = policy.NewDecisionEngine()
	}

	rec := engine.RecommendFromEvidence(now, evalPolicy, &state, snap, policy.RecommendOptions{
		Preview: req.Preview,
	})
	return &rec, nil
}

func nodeKeyOf(node policy.EvidenceNode, ok bool) string {
	if !ok {
		return ""
	}
	return node.NodeKey
}

func identityKeyOf(node policy.EvidenceNode, ok bool) string {
	if !ok {
		return ""
	}
	return node.NodeIdentityKey
}

func parsePolicyPurpose(raw string) (policy.PolicyPurpose, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(policy.PurposeGeneral):
		return policy.PurposeGeneral, nil
	case string(policy.PurposeAI):
		return policy.PurposeAI, nil
	default:
		return "", fmt.Errorf("invalid purpose %q: must be %q or %q",
			raw, policy.PurposeGeneral, policy.PurposeAI)
	}
}

// collectEvidenceNodes resolves the node universe and its provenance.
//
// When jobID names a live monitor job, that job's node set is authoritative (it carries
// the exact ProfileID / NodeIdentityKey / ConfigRevisionKey derived from the raw proxy
// config). Otherwise the node set is discovered from persisted raw samples, grouped by
// (ProfileID, NodeIdentityKey) so the same endpoint under different profiles stays
// separate instead of being collapsed into one node.
func (s *AppService) collectEvidenceNodes(
	ctx context.Context,
	jobID string,
	since, now time.Time,
) ([]policy.EvidenceNode, policy.EvidenceSource, bool, error) {
	source := policy.EvidenceSource{
		LookbackSince: since,
		LookbackUntil: now,
	}

	if strings.TrimSpace(jobID) != "" {
		job, err := s.GetMonitorJob(jobID)
		if err != nil {
			// Job definitions live only in memory, so an unknown ID is a client-visible error
			// rather than silently-empty evidence.
			return nil, source, false, monitor.WrapValidationError(err)
		}

		nodes := make([]policy.EvidenceNode, 0, len(job.Nodes))
		for i := range job.Nodes {
			node := job.Nodes[i]
			monitor.PopulateNodeKeys(&node)
			nodes = append(nodes, policy.EvidenceNode{
				ProfileID:         job.ProfileID,
				NodeKey:           node.NodeKey,
				NodeIdentityKey:   node.NodeIdentityKey,
				ConfigRevisionKey: node.ConfigRevisionKey,
				DisplayName:       node.DisplayName,
			})
		}

		source.JobID = job.ID
		source.ProfileID = job.ProfileID
		source.ProbeSet = string(job.ProbeSet)
		source.NodeSetSource = "monitor_job"
		return nodes, source, false, nil
	}

	// Discovery path: enumerate node identities present in persisted raw samples.
	discoverySince := since
	if maxDiscovery := now.Add(-evidenceDiscoveryMaxWindow); discoverySince.Before(maxDiscovery) {
		discoverySince = maxDiscovery
	}

	samples, err := s.historyStore.QueryMonitorSamples(ctx, monitor.SampleFilter{
		Since:     &discoverySince,
		Limit:     evidenceDiscoverySampleLimit,
		OrderDesc: true,
	})
	if err != nil {
		return nil, source, false, fmt.Errorf("discover monitor evidence nodes: %w", err)
	}

	truncated := len(samples) >= evidenceDiscoverySampleLimit

	// Newest-first: the first sample seen for a (profile, identity) pair is the current
	// revision of that logical node.
	seen := map[string]struct{}{}
	profiles := map[string]struct{}{}
	var nodes []policy.EvidenceNode
	for _, sample := range samples {
		if sample == nil {
			continue
		}
		identity := sample.NodeIdentityKey
		if identity == "" {
			identity = sample.NodeKey
		}
		if identity == "" {
			continue
		}
		profile := strings.TrimSpace(sample.ProfileID)
		key := profile + "\x00" + identity
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if profile != "" {
			profiles[profile] = struct{}{}
		}
		nodes = append(nodes, policy.EvidenceNode{
			ProfileID:         profile,
			NodeKey:           sample.NodeKey,
			NodeIdentityKey:   sample.NodeIdentityKey,
			ConfigRevisionKey: sample.ConfigRevisionKey,
			DisplayName:       sample.DisplayNameSnapshot,
		})
	}

	if len(profiles) == 1 {
		for profile := range profiles {
			source.ProfileID = profile
		}
	}
	source.NodeSetSource = "persisted_samples"
	return nodes, source, truncated, nil
}

// collectEvidenceSamples projects persisted raw samples into policy.EvidenceSample.
//
// Samples are fetched by NodeIdentityKey (which deliberately spans config revisions) and
// qualified by ProfileID so a logical node's evidence can never be aggregated with the
// same physical endpoint observed under a different subscription/profile.
func (s *AppService) collectEvidenceSamples(
	ctx context.Context,
	nodes []policy.EvidenceNode,
	since time.Time,
) (map[string][]policy.EvidenceSample, int, bool) {
	out := make(map[string][]policy.EvidenceSample, len(nodes))
	truncated := false
	total := 0

	for _, node := range nodes {
		// Keyed by SampleSetKey(), not NodeKey: two logical nodes can share a NodeKey (the
		// same subscription node in two profiles with identical credentials), and keying by
		// NodeKey would make one silently read the other's evidence.
		if node.NodeIdentityKey == "" && node.NodeKey == "" {
			// Without an identity key the query would be unbounded and every sample in the
			// database would be attributed to this node. Refuse rather than fabricate.
			out[node.SampleSetKey()] = nil
			continue
		}

		filter := monitor.SampleFilter{
			Since:     &since,
			Limit:     evidenceSampleLimitPerNode,
			OrderDesc: true,
		}
		if node.NodeIdentityKey != "" {
			filter.NodeIdentityKey = node.NodeIdentityKey
		} else {
			filter.NodeKey = node.NodeKey
		}
		if node.ProfileID != "" {
			filter.ProfileID = node.ProfileID
		}

		samples, err := s.historyStore.QueryMonitorSamples(ctx, filter)
		if err != nil {
			// Evidence for one node failing to load must not fabricate a verdict for it:
			// the node simply carries no samples and will be gated as insufficient.
			continue
		}
		if len(samples) >= evidenceSampleLimitPerNode {
			truncated = true
		}

		projected := make([]policy.EvidenceSample, 0, len(samples))
		for _, sample := range samples {
			if sample == nil {
				continue
			}
			projected = append(projected, policy.EvidenceSample{
				ProfileID:         sample.ProfileID,
				NodeKey:           sample.NodeKey,
				NodeIdentityKey:   sample.NodeIdentityKey,
				ConfigRevisionKey: sample.ConfigRevisionKey,
				DisplayName:       sample.DisplayNameSnapshot,
				ProbeType:         sample.ProbeType,
				Target:            sample.Target,
				Timestamp:         sample.Timestamp,
				Success:           sample.Success,
				Latency:           sample.Latency,
				TTFB:              sample.TTFB,
				ErrorClass:        sample.ErrorClass,
				ErrorDetail:       sample.ErrorDetail,
				RegionBlocked:     regionBlockedFromMetadata(sample.Metadata),
			})
		}
		out[node.SampleSetKey()] = projected
		total += len(projected)
	}

	return out, total, truncated
}

// regionBlockedFromMetadata surfaces an explicit upstream region-block flag if present.
// The flag is only transported here; its interpretation belongs to core/policy.
func regionBlockedFromMetadata(metadata map[string]any) bool {
	if len(metadata) == 0 {
		return false
	}
	for _, key := range []string{"region_blocked", "geo_blocked", "blocked"} {
		if v, ok := metadata[key]; ok {
			if flag, isBool := v.(bool); isBool && flag {
				return true
			}
		}
	}
	return false
}

// resolveEvidenceCurrentNode determines the active node using only read-only sources.
func (s *AppService) resolveEvidenceCurrentNode(
	ctx context.Context,
	req MonitorRecommendationRequest,
	nodes []policy.EvidenceNode,
) (policy.EvidenceNode, bool) {
	if key := strings.TrimSpace(req.CurrentNodeKey); key != "" {
		for _, node := range nodes {
			if node.NodeKey == key {
				return node, true
			}
		}
	}
	if identity := strings.TrimSpace(req.CurrentNodeIdentityKey); identity != "" {
		for _, node := range nodes {
			if node.NodeIdentityKey == identity {
				return node, true
			}
		}
	}

	// Orchestrator's in-memory notion of the current node (DisplayName-based).
	s.ctrlMu.RLock()
	stateCurrent := s.decisionState.CurrentNode
	group := s.policy.TargetGroup
	ctrl := s.controller
	s.ctrlMu.RUnlock()

	if stateCurrent != "" {
		for _, node := range nodes {
			if node.DisplayName == stateCurrent {
				return node, true
			}
		}
	}

	// External controller selection — a strictly read-only call.
	if ctrl != nil {
		if group == "" {
			group = "PROXY"
		}
		if selection, err := ctrl.GetCurrentSelection(ctx, group); err == nil && selection != "" {
			for _, node := range nodes {
				if node.DisplayName == selection {
					return node, true
				}
			}
		}
	}

	return policy.EvidenceNode{}, false
}

// resolveCandidateNames narrows the policy candidate whitelist by the request's node keys.
//
// Returns (nil, nil) when no narrowing is requested. When the policy already carries a
// whitelist, the two are intersected rather than replaced. An empty intersection is a
// client error: silently widening to "all nodes" would be the opposite of the request.
func resolveCandidateNames(
	p policy.SwitchPolicy,
	nodes []policy.EvidenceNode,
	requested []string,
) ([]string, error) {
	if len(requested) == 0 {
		return nil, nil
	}

	wanted := map[string]struct{}{}
	for _, key := range requested {
		key = strings.TrimSpace(key)
		if key != "" {
			wanted[key] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return nil, monitor.WrapValidationError(fmt.Errorf("candidate_node_keys must not be empty"))
	}

	var names []string
	for _, node := range nodes {
		matched := false
		if _, ok := wanted[node.NodeKey]; ok {
			matched = true
		}
		if _, ok := wanted[node.NodeIdentityKey]; ok {
			matched = true
		}
		if !matched {
			continue
		}
		if len(p.CandidateNodes) > 0 && !containsName(p.CandidateNodes, node.DisplayName) {
			continue
		}
		names = append(names, node.DisplayName)
	}

	if len(names) == 0 {
		return nil, monitor.WrapValidationError(fmt.Errorf(
			"candidate_node_keys did not match any node in the evidence window"))
	}
	return names, nil
}

func containsName(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

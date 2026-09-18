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
// -----------------------------------------------------------------------------
// Candidate membership (B-02)
// -----------------------------------------------------------------------------
//
// The candidate universe comes from the CURRENT monitor job definition and nowhere else.
// History is evidence only: a NodeIdentityKey that merely appeared in recent samples must
// never be promoted back into the candidate set, because the node may have been removed from
// the subscription or replaced by a new config revision. A candidate therefore always carries
// the job's current NodeKey / NodeIdentityKey / ConfigRevisionKey, which is also the only
// place a current revision can be obtained safely.
//
// -----------------------------------------------------------------------------
// Evidence read budget and completeness (B-01)
// -----------------------------------------------------------------------------
//
// The observation window is read COMPLETELY by draining the keyset cursor until the window is
// exhausted. A single cursor page is capped at 1000 rows by the store, so any node with more
// than 1000 samples in the window necessarily spans several pages. Keyset (not OFFSET)
// pagination is required because the monitor subsystem writes samples continuously.
//
// Budget arithmetic, worst case per request:
//
//	rows  = job node count x maxEvidenceSamplesPerNode
//	pages = job node count x maxEvidencePagesPerNode
//
// Both factors are explicit and finite: the node count comes from the job definition (never
// from history) and the window itself is capped at 24h by policy.DefaultEvidenceWindow.
// Exceeding the per-node budget does not silently truncate — the node is gated as
// insufficient_evidence with reason evidence_budget_exceeded. Any page failing aborts the
// whole request, so a partial read can never be presented as complete evidence.
// =============================================================================

// Evidence read budget. See the file header for the worst-case arithmetic.
const (
	// evidenceCursorPageSize is the page size used while draining a node's raw samples.
	// The store rejects any limit above 1000.
	evidenceCursorPageSize = 1000

	// maxEvidenceSamplesPerNode is the total raw-sample budget for one node inside the
	// evidence window.
	maxEvidenceSamplesPerNode = 50000

	// maxEvidencePagesPerNode bounds the pagination loop independently of the row budget.
	maxEvidencePagesPerNode = 60
)

// MonitorRecommendationRequest selects the evidence window to evaluate.
type MonitorRecommendationRequest struct {
	// JobID identifies the monitor job whose CURRENT node set defines the candidate universe.
	//
	// It is required: the job definition is the only authoritative source of the current
	// candidate membership and of each node's current ConfigRevisionKey. Recent history is
	// evidence only and can never re-introduce a node that was removed from the job.
	JobID string `json:"job_id"`

	// CurrentNodeKey / CurrentNodeIdentityKey explicitly identify the active node.
	// When both are empty the service falls back to the orchestrator's in-memory current
	// node, then to the external controller's current selector value (a read-only call).
	// An explicitly supplied identifier that matches no job node is a validation error.
	CurrentNodeKey         string `json:"current_node_key,omitempty"`
	CurrentNodeIdentityKey string `json:"current_node_identity_key,omitempty"`

	// Purpose selects the evaluation lens: "general" or "ai".
	// Empty means "use the configured policy purpose".
	Purpose string `json:"purpose,omitempty"`

	// CandidateNodeKeys optionally narrows the candidate set to specific node keys of the
	// job. It can only RESTRICT the job's node set, never extend it.
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

	// 2. Resolve the candidate universe from the current job definition (B-02).
	nodes, source, err := s.collectEvidenceNodes(req.JobID, since, now)
	if err != nil {
		return nil, err
	}

	// 3. Resolve the current node (request → orchestrator state → controller selection).
	current, hasCurrent, err := s.resolveEvidenceCurrentNode(ctx, req, nodes)
	if err != nil {
		return nil, err
	}

	// 4. Read the raw samples for every node, draining the cursor until the window is
	//    exhausted (B-01).
	sets, err := collectEvidenceSamples(ctx, s.historyStore, nodes, since)
	if err != nil {
		return nil, err
	}

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
		SamplesByNode:          sets.samples,
		RawSampleCount:         sets.total,
		BudgetExceededNodes:    sets.budgetExceeded,
		PagesReadByNode:        sets.pagesRead,
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

// collectEvidenceNodes resolves the candidate universe and its provenance.
//
// B-02: the ONLY authoritative source is the current monitor job definition. It carries the
// current NodeKey / NodeIdentityKey / ConfigRevisionKey derived from the raw proxy config, so
// a node that was removed from the job simply is not a candidate, and a node whose config
// revision changed is evaluated against its NEW revision (never against the old one).
func (s *AppService) collectEvidenceNodes(
	jobID string,
	since, now time.Time,
) ([]policy.EvidenceNode, policy.EvidenceSource, error) {
	source := policy.EvidenceSource{
		LookbackSince: since,
		LookbackUntil: now,
	}

	if strings.TrimSpace(jobID) == "" {
		return nil, source, monitor.WrapValidationError(fmt.Errorf(
			"job_id is required: the candidate set must come from the current monitor job configuration; " +
				"recent history is evidence only and can never re-introduce a removed node"))
	}

	job, err := s.GetMonitorJob(jobID)
	if err != nil {
		// Job definitions live only in memory, so an unknown ID is a client-visible error
		// rather than silently-empty evidence.
		return nil, source, monitor.WrapValidationError(err)
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
	return nodes, source, nil
}

// evidenceSampleSets is the per-node raw-sample read result.
type evidenceSampleSets struct {
	samples        map[string][]policy.EvidenceSample
	budgetExceeded map[string]bool
	pagesRead      map[string]int
	total          int
}

// collectEvidenceSamples reads the complete raw-sample window for each node.
//
// It takes the store explicitly (rather than reading AppService.historyStore) so the failure
// path is injectable and testable: a failing query must surface as an error instead of being
// silently downgraded to "this node has no evidence".
func collectEvidenceSamples(
	ctx context.Context,
	store monitor.SampleStore,
	nodes []policy.EvidenceNode,
	since time.Time,
) (*evidenceSampleSets, error) {
	out := &evidenceSampleSets{
		samples:        make(map[string][]policy.EvidenceSample, len(nodes)),
		budgetExceeded: map[string]bool{},
		pagesRead:      make(map[string]int, len(nodes)),
	}

	for _, node := range nodes {
		// Keyed by SampleSetKey(), not NodeKey: two logical nodes can share a NodeKey (the
		// same subscription node in two profiles with identical credentials), and keying by
		// NodeKey would make one silently read the evidence of the other.
		key := node.SampleSetKey()
		if node.NodeIdentityKey == "" && node.NodeKey == "" {
			// Without an identity key the query would be unbounded and every sample in the
			// database would be attributed to this node. Refuse rather than fabricate.
			out.samples[key] = nil
			continue
		}

		// The filter is built once and only its Cursor advances between pages, so the
		// NodeIdentityKey / ProfileID / Since qualification is provably identical on every
		// page of the drain.
		filter := monitor.CursorFilter{
			Since:     &since,
			Limit:     evidenceCursorPageSize,
			OrderDesc: true,
		}
		// Legacy key bridge: history written before the NodeIdentityKey migration stores
		// node_identity_key = node_key, so it is only reachable when BOTH keys are supplied
		// (core/history/db.go applies
		// "((node_identity_key = ?) OR (node_identity_key = node_key AND node_key = ?))"
		// only when both are set). Supplying just the identity key silently under-reads that
		// history. Those rows also carry no config_revision_key and are excluded by the
		// revision gate anyway, so this keeps the read qualification consistent with the
		// documented bridge convention rather than changing the statistics.
		if node.NodeIdentityKey != "" {
			filter.NodeIdentityKey = node.NodeIdentityKey
			filter.LegacyNodeKey = node.NodeKey
		} else {
			filter.NodeKey = node.NodeKey
		}
		if node.ProfileID != "" {
			filter.ProfileID = node.ProfileID
		}

		projected, pages, budgetExceeded, err := drainNodeSamples(ctx, store, node, filter)
		if err != nil {
			// Any page failing must fail the whole request: using the pages that did succeed
			// would silently present a partial read as complete evidence.
			return nil, err
		}

		out.samples[key] = projected
		out.pagesRead[key] = pages
		if budgetExceeded {
			out.budgetExceeded[key] = true
		}
		out.total += len(projected)
	}

	return out, nil
}

// drainNodeSamples follows the keyset cursor until the observation window is exhausted.
//
// Keyset (not OFFSET) pagination is required here because the monitor subsystem writes samples
// continuously: an OFFSET-based loop would shift rows between pages and silently skip or
// duplicate samples. The filter is re-sent unchanged on every page — only Cursor is advanced —
// so the qualification cannot drift mid-drain.
func drainNodeSamples(
	ctx context.Context,
	store monitor.SampleStore,
	node policy.EvidenceNode,
	filter monitor.CursorFilter,
) ([]policy.EvidenceSample, int, bool, error) {
	var projected []policy.EvidenceSample
	pages := 0

	for {
		page, err := store.QueryMonitorSamplesCursor(ctx, filter)
		if err != nil {
			return nil, pages, false, fmt.Errorf(
				"load monitor evidence for node %s (page %d): %w", node.DisplayName, pages+1, err)
		}
		if page == nil {
			return nil, pages, false, fmt.Errorf(
				"load monitor evidence for node %s (page %d): store returned no page", node.DisplayName, pages+1)
		}
		pages++

		for _, sample := range page.Items {
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

		// Window exhausted, or the store has nothing more to give.
		if !page.HasMore || page.NextCursor == "" {
			return projected, pages, false, nil
		}

		if len(projected) >= maxEvidenceSamplesPerNode || pages >= maxEvidencePagesPerNode {
			// The window cannot be read completely within budget. Report it explicitly instead
			// of handing a partial read to the policy layer as if it were complete.
			return projected, pages, true, nil
		}

		filter.Cursor = page.NextCursor
	}
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
) (policy.EvidenceNode, bool, error) {
	// An explicit identifier that cannot be honoured must not be silently ignored: falling
	// back to a different node would answer a question the caller did not ask. This mirrors
	// the candidate_node_keys behaviour.
	if key := strings.TrimSpace(req.CurrentNodeKey); key != "" {
		for _, node := range nodes {
			if node.NodeKey == key {
				return node, true, nil
			}
		}
		return policy.EvidenceNode{}, false, monitor.WrapValidationError(fmt.Errorf(
			"current_node_key %q did not match any node of the monitor job", key))
	}
	if identity := strings.TrimSpace(req.CurrentNodeIdentityKey); identity != "" {
		for _, node := range nodes {
			if node.NodeIdentityKey == identity {
				return node, true, nil
			}
		}
		return policy.EvidenceNode{}, false, monitor.WrapValidationError(fmt.Errorf(
			"current_node_identity_key %q did not match any node of the monitor job", identity))
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
				return node, true, nil
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
					return node, true, nil
				}
			}
		}
	}

	return policy.EvidenceNode{}, false, nil
}

// resolveCandidateNames narrows the policy candidate whitelist by the request's node keys.
//
// B-02: it can only RESTRICT the job's node set — membership always originates from the job.
// Returns (nil, nil) when no narrowing is requested. When the policy already carries a
// whitelist, the two are intersected rather than replaced. An empty intersection is a client
// error: silently widening to "all nodes" would be the opposite of the request.
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
			"candidate_node_keys did not match any node of the monitor job"))
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

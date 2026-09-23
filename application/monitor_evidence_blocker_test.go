package application

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/policy"
)

// =============================================================================
// B-01 (end-to-end, real store) and B-02 (candidate membership / revision provenance).
// =============================================================================

// evidenceGroundTruth is computed independently from the generated sample list, so the
// assertions compare the recommendation against an oracle rather than against the
// implementation.
type evidenceGroundTruth struct {
	sampleCount         int
	successCount        int
	failureCount        int
	successRate         float64
	latencyP50          time.Duration
	latencyP95          time.Duration
	errorBreakdown      map[string]int
	observationWindow   time.Duration
	consecutiveFailures int
}

// buildPagedSeries generates `count` transport samples, index 0 being the newest, one second
// apart. It also returns the oracle statistics for the generated set.
func buildPagedSeries(
	node monitor.MonitoredNode,
	now time.Time,
	count int,
	successLatencyMs func(i int) int,
	fails func(i int) bool,
) ([]*monitor.MonitorSample, evidenceGroundTruth) {
	samples := make([]*monitor.MonitorSample, 0, count)
	truth := evidenceGroundTruth{errorBreakdown: map[string]int{}}
	var latencies []time.Duration

	newest := now.Add(-10 * time.Second)
	oldest := newest
	for i := 0; i < count; i++ {
		ts := newest.Add(-time.Duration(i) * time.Second)
		if ts.Before(oldest) {
			oldest = ts
		}
		success := !fails(i)
		latencyMs := successLatencyMs(i)
		if !success {
			latencyMs = 5000
		}
		sample := &monitor.MonitorSample{
			SampleID:            fmt.Sprintf("s_%s_%06d", node.NodeKey, i),
			RunID:               "run_paged",
			NodeKey:             node.NodeKey,
			NodeIdentityKey:     node.NodeIdentityKey,
			ConfigRevisionKey:   node.ConfigRevisionKey,
			ProfileID:           "prof-paged",
			DisplayNameSnapshot: node.DisplayName,
			ProbeType:           "rtt",
			Target:              "https://cp.cloudflare.com/generate_204",
			Timestamp:           ts,
			Success:             success,
			Latency:             time.Duration(latencyMs) * time.Millisecond,
			TTFB:                time.Duration(latencyMs) * time.Millisecond,
			ErrorClass:          "none",
		}
		if !success {
			sample.ErrorClass = "timeout"
			truth.failureCount++
			truth.errorBreakdown["timeout"]++
		} else {
			truth.successCount++
			latencies = append(latencies, sample.Latency)
		}
		samples = append(samples, sample)
	}

	truth.sampleCount = count
	truth.successRate = math.Round((float64(truth.successCount)/float64(count))*10000) / 10000
	truth.observationWindow = newest.Sub(oldest)

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	truth.latencyP50 = nearestRank(latencies, 0.50)
	truth.latencyP95 = nearestRank(latencies, 0.95)

	// Consecutive failures walking newest-first over transport samples.
	for _, sample := range samples {
		if sample.Success {
			break
		}
		truth.consecutiveFailures++
	}

	return samples, truth
}

func nearestRank(sortedAsc []time.Duration, p float64) time.Duration {
	if len(sortedAsc) == 0 {
		return 0
	}
	rank := int(math.Ceil(p*float64(len(sortedAsc)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sortedAsc) {
		rank = len(sortedAsc) - 1
	}
	return sortedAsc[rank]
}

// TestAppService_GetMonitorRecommendation_ReadsCompleteMultiPageEvidence is the B-01
// end-to-end regression: a node whose evidence window spans several cursor pages must be
// evaluated against the COMPLETE window, not the first page.
func TestAppService_GetMonitorRecommendation_ReadsCompleteMultiPageEvidence(t *testing.T) {
	svc, hStore := newEvidenceTestService(t)

	pol := policy.DefaultSwitchPolicy()
	pol.Mode = policy.ModeRecommend
	if err := svc.UpdateSwitchPolicy(context.Background(), pol); err != nil {
		t.Fatalf("UpdateSwitchPolicy: %v", err)
	}

	job, err := svc.CreateMonitorJob(monitor.MonitorJob{
		ID:        "job_paged",
		Name:      "Paged Evidence",
		ProfileID: "prof-paged",
		ProbeSet:  monitor.ProbeSetLight,
		Interval:  time.Hour,
		Nodes: []monitor.MonitoredNode{
			evidenceTestNode("HK-01", "10.80.0.1", 8388, "pw-hk"),
			evidenceTestNode("SG-01", "10.80.0.2", 8388, "pw-sg"),
		},
	})
	if err != nil {
		t.Fatalf("CreateMonitorJob: %v", err)
	}

	const sampleCount = 2500 // 2500 rows / 1000 per page => at least 3 pages

	now := time.Now()
	if err := hStore.SaveMonitorRun(context.Background(), &monitor.MonitorRun{
		RunID: "run_paged", JobID: job.ID, SamplingTier: monitor.SamplingTierRegular,
		TriggerType: monitor.SamplingTriggerScheduled, SamplingStrategyVersion: monitor.SamplingStrategyVersion,
		ScheduledAt: now, StartedAt: now, Status: monitor.RunStatusCompleted, TotalNodes: 2, SuccessNodes: 2,
	}); err != nil {
		t.Fatalf("SaveMonitorRun: %v", err)
	}
	hkSamples, hkTruth := buildPagedSeries(
		job.Nodes[0], now, sampleCount,
		func(i int) int { return 100 + (i%7)*10 },
		func(i int) bool { return i%10 == 9 },
	)
	sgSamples, sgTruth := buildPagedSeries(
		job.Nodes[1], now, sampleCount,
		func(i int) int { return 40 },
		func(i int) bool { return false },
	)
	if err := hStore.SaveMonitorSamples(context.Background(), append(append([]*monitor.MonitorSample{}, hkSamples...), sgSamples...)); err != nil {
		t.Fatalf("SaveMonitorSamples: %v", err)
	}

	rec, err := svc.GetMonitorRecommendation(context.Background(), MonitorRecommendationRequest{
		JobID:          "job_paged",
		CurrentNodeKey: job.Nodes[0].NodeKey,
	})
	if err != nil {
		t.Fatalf("GetMonitorRecommendation: %v", err)
	}

	if rec.Snapshot == nil || len(rec.Snapshot.Nodes) != 2 {
		t.Fatalf("expected 2 nodes in the snapshot, got %+v", rec.Snapshot)
	}

	byName := map[string]*policy.NodeEvidence{}
	for i := range rec.Snapshot.Nodes {
		byName[rec.Snapshot.Nodes[i].DisplayName] = &rec.Snapshot.Nodes[i]
	}

	// 1. Every node was read across multiple pages.
	for _, name := range []string{"HK-01", "SG-01"} {
		node := byName[name]
		if node == nil {
			t.Fatalf("expected node %s in the snapshot", name)
		}
		if node.PagesRead < 3 {
			t.Fatalf("%s: expected at least 3 cursor pages drained for %d samples, got %d",
				name, sampleCount, node.PagesRead)
		}
		if node.EvidenceBudgetExceeded {
			t.Fatalf("%s: 2500 samples must not exceed the per-node budget", name)
		}
	}

	// 2. The statistics must match the independently computed oracle exactly.
	hk := byName["HK-01"]
	if hk.SampleCount != hkTruth.sampleCount {
		t.Fatalf("HK SampleCount = %d, want %d", hk.SampleCount, hkTruth.sampleCount)
	}
	if hk.SuccessCount != hkTruth.successCount || hk.FailureCount != hkTruth.failureCount {
		t.Fatalf("HK success/failure = %d/%d, want %d/%d",
			hk.SuccessCount, hk.FailureCount, hkTruth.successCount, hkTruth.failureCount)
	}
	if hk.SuccessRate != hkTruth.successRate {
		t.Fatalf("HK SuccessRate = %v, want %v", hk.SuccessRate, hkTruth.successRate)
	}
	if hk.LatencyP50 != hkTruth.latencyP50 || hk.LatencyP95 != hkTruth.latencyP95 {
		t.Fatalf("HK P50/P95 = %s/%s, want %s/%s",
			hk.LatencyP50, hk.LatencyP95, hkTruth.latencyP50, hkTruth.latencyP95)
	}
	if hk.ObservationWindow != hkTruth.observationWindow {
		t.Fatalf("HK ObservationWindow = %s, want %s", hk.ObservationWindow, hkTruth.observationWindow)
	}
	if hk.ConsecutiveTransportFailures != hkTruth.consecutiveFailures {
		t.Fatalf("HK consecutive failures = %d, want %d", hk.ConsecutiveTransportFailures, hkTruth.consecutiveFailures)
	}
	if len(hk.ErrorBreakdown) != len(hkTruth.errorBreakdown) || hk.ErrorBreakdown["timeout"] != hkTruth.errorBreakdown["timeout"] {
		t.Fatalf("HK ErrorBreakdown = %+v, want %+v", hk.ErrorBreakdown, hkTruth.errorBreakdown)
	}

	// 3. No page lost rows to profile/revision filtering drift.
	if hk.ExcludedOtherProfileSamples != 0 || hk.ExcludedUnknownProfileSamples != 0 {
		t.Fatalf("HK profile filtering drifted across pages: %+v", hk)
	}
	if hk.ExcludedOtherRevisionSamples != 0 || hk.ExcludedUnknownRevisionSamples != 0 {
		t.Fatalf("HK revision filtering drifted across pages: %+v", hk)
	}

	// 4. The candidate's statistics come from its complete window too.
	sg := byName["SG-01"]
	if sg.SampleCount != sgTruth.sampleCount || sg.SuccessRate != sgTruth.successRate ||
		sg.LatencyP50 != sgTruth.latencyP50 || sg.LatencyP95 != sgTruth.latencyP95 {
		t.Fatalf("SG statistics = count %d rate %v p50 %s p95 %s, want count %d rate %v p50 %s p95 %s",
			sg.SampleCount, sg.SuccessRate, sg.LatencyP50, sg.LatencyP95,
			sgTruth.sampleCount, sgTruth.successRate, sgTruth.latencyP50, sgTruth.latencyP95)
	}

	// 5. The recommendation itself is consistent with the complete data.
	if rec.Decision != policy.DecisionRecommendSwitch {
		t.Fatalf("expected recommend_switch, got %s", rec.Decision)
	}
	if rec.RecommendedNode == nil || rec.RecommendedNode.DisplayName != "SG-01" {
		t.Fatalf("expected SG-01, got %+v", rec.RecommendedNode)
	}
	if rec.SampleCount != hkTruth.sampleCount {
		t.Fatalf("recommendation SampleCount = %d, want the full window %d", rec.SampleCount, hkTruth.sampleCount)
	}
	if rec.CurrentNode.LatencyP50 != hkTruth.latencyP50 {
		t.Fatalf("recommendation current P50 = %s, want %s", rec.CurrentNode.LatencyP50, hkTruth.latencyP50)
	}
	if rec.CurrentNode.SuccessRate != hkTruth.successRate {
		t.Fatalf("recommendation current SuccessRate = %v, want %v", rec.CurrentNode.SuccessRate, hkTruth.successRate)
	}
	if rec.CurrentNode.ErrorBreakdown["timeout"] != hkTruth.errorBreakdown["timeout"] {
		t.Fatalf("recommendation current ErrorBreakdown = %+v, want %+v",
			rec.CurrentNode.ErrorBreakdown, hkTruth.errorBreakdown)
	}
}

// TestAppService_GetMonitorRecommendation_RemovedNodeIsNeverACandidate is the B-02 regression:
// history is evidence only. A node with excellent history that is no longer part of the
// current job must never come back as a candidate.
func TestAppService_GetMonitorRecommendation_RemovedNodeIsNeverACandidate(t *testing.T) {
	svc, hStore := newEvidenceTestService(t)

	pol := policy.DefaultSwitchPolicy()
	pol.Mode = policy.ModeRecommend
	if err := svc.UpdateSwitchPolicy(context.Background(), pol); err != nil {
		t.Fatalf("UpdateSwitchPolicy: %v", err)
	}

	// The job knows only HK-01 (current) and SG-01 (candidate).
	job, err := svc.CreateMonitorJob(monitor.MonitorJob{
		ID:        "job_membership",
		Name:      "Membership",
		ProfileID: "prof-membership",
		ProbeSet:  monitor.ProbeSetLight,
		Interval:  time.Hour,
		Nodes: []monitor.MonitoredNode{
			evidenceTestNode("HK-01", "10.90.0.1", 8388, "pw-hk"),
			evidenceTestNode("SG-01", "10.90.0.2", 8388, "pw-sg"),
		},
	})
	if err != nil {
		t.Fatalf("CreateMonitorJob: %v", err)
	}

	// REMOVED-NODE has spectacular history but is not part of the job any more.
	removed := evidenceTestNode("REMOVED-NODE", "10.90.0.99", 8388, "pw-removed")
	monitor.PopulateNodeKeys(&removed)

	now := time.Now()
	insertEvidenceSamples(t, hStore, removed, "prof-membership", "removed", now, 10*time.Second, 30*time.Second, 30, true, 5, "")
	insertEvidenceSamples(t, hStore, job.Nodes[0], "prof-membership", "hk", now, 10*time.Second, 30*time.Second, 5, true, 200, "")
	insertEvidenceSamples(t, hStore, job.Nodes[1], "prof-membership", "sg", now, 10*time.Second, 30*time.Second, 5, true, 80, "")

	rec, err := svc.GetMonitorRecommendation(context.Background(), MonitorRecommendationRequest{
		JobID:          "job_membership",
		CurrentNodeKey: job.Nodes[0].NodeKey,
	})
	if err != nil {
		t.Fatalf("GetMonitorRecommendation: %v", err)
	}

	if len(rec.Snapshot.Nodes) != 2 {
		t.Fatalf("expected only the 2 job nodes, got %d", len(rec.Snapshot.Nodes))
	}
	for i := range rec.Snapshot.Nodes {
		node := &rec.Snapshot.Nodes[i]
		if node.NodeIdentityKey == removed.NodeIdentityKey {
			t.Fatalf("a node absent from the current job must never appear in the evidence snapshot")
		}
	}
	if rec.RecommendedNode == nil || rec.RecommendedNode.DisplayName != "SG-01" {
		t.Fatalf("expected the job candidate SG-01, got %+v", rec.RecommendedNode)
	}
	if rec.RecommendedNode.NodeKey == removed.NodeKey {
		t.Fatalf("a removed node must never be recommended")
	}

	// The removed node's identity must not leak anywhere into the output at all.
	blob := fmt.Sprintf("%+v", rec)
	if strings.Contains(blob, removed.NodeIdentityKey) || strings.Contains(blob, removed.NodeKey) {
		t.Fatalf("the removed node leaked into the recommendation output")
	}

	// candidate_node_keys can only restrict the job set, never extend it with a history key.
	_, err = svc.GetMonitorRecommendation(context.Background(), MonitorRecommendationRequest{
		JobID:             "job_membership",
		CurrentNodeKey:    job.Nodes[0].NodeKey,
		CandidateNodeKeys: []string{removed.NodeKey},
	})
	if err == nil {
		t.Fatalf("candidate_node_keys must not be able to add a node that is not in the job")
	}
	if !monitor.IsValidationError(err) {
		t.Fatalf("expected a validation error, got %v", err)
	}
}

// TestAppService_GetMonitorRecommendation_RevisionChangeDoesNotReuseOldEvidence is the B-02
// ConfigRevision regression: a candidate whose current revision has no samples yet must be
// rejected, and the excellent evidence from its previous revision must never be reused.
func TestAppService_GetMonitorRecommendation_RevisionChangeDoesNotReuseOldEvidence(t *testing.T) {
	svc, hStore := newEvidenceTestService(t)

	pol := policy.DefaultSwitchPolicy()
	pol.Mode = policy.ModeRecommend
	if err := svc.UpdateSwitchPolicy(context.Background(), pol); err != nil {
		t.Fatalf("UpdateSwitchPolicy: %v", err)
	}

	// Same physical endpoint and same credentials, but the job now carries a NEW revision
	// (the password rotated), so R2 != R1.
	server, port := "10.95.0.1", 8388
	previousRevisionNode := evidenceTestNode("ROTATED", server, port, "password-r1")
	monitor.PopulateNodeKeys(&previousRevisionNode)

	job, err := svc.CreateMonitorJob(monitor.MonitorJob{
		ID:        "job_revision",
		Name:      "Revision",
		ProfileID: "prof-rev",
		ProbeSet:  monitor.ProbeSetLight,
		Interval:  time.Hour,
		Nodes: []monitor.MonitoredNode{
			evidenceTestNode("HK-01", "10.95.0.9", 8388, "pw-hk"),
			evidenceTestNode("ROTATED", server, port, "password-r2"),
		},
	})
	if err != nil {
		t.Fatalf("CreateMonitorJob: %v", err)
	}

	if job.Nodes[1].NodeIdentityKey != previousRevisionNode.NodeIdentityKey {
		t.Fatalf("test precondition failed: expected the same transport identity")
	}
	if job.Nodes[1].ConfigRevisionKey == previousRevisionNode.ConfigRevisionKey {
		t.Fatalf("test precondition failed: expected a different ConfigRevisionKey")
	}

	now := time.Now()
	// The OLD revision has plenty of excellent history...
	insertEvidenceSamples(t, hStore, previousRevisionNode, "prof-rev", "r1", now, 10*time.Second, 30*time.Second, 30, true, 5, "")
	// ...and the current revision has no samples at all yet.
	insertEvidenceSamples(t, hStore, job.Nodes[0], "prof-rev", "hk", now, 10*time.Second, 30*time.Second, 5, true, 200, "")

	rec, err := svc.GetMonitorRecommendation(context.Background(), MonitorRecommendationRequest{
		JobID:          "job_revision",
		CurrentNodeKey: job.Nodes[0].NodeKey,
	})
	if err != nil {
		t.Fatalf("GetMonitorRecommendation: %v", err)
	}

	rotated := rec.Snapshot.Nodes[1]
	if rotated.SampleCount != 0 {
		t.Fatalf("the previous revision's evidence must not be attributed to the current revision, got %d samples",
			rotated.SampleCount)
	}
	if rotated.ExcludedOtherRevisionSamples != 30 {
		t.Fatalf("expected the 30 old-revision samples to be excluded, got %d", rotated.ExcludedOtherRevisionSamples)
	}
	if rotated.Sufficiency.Sufficient {
		t.Fatalf("a candidate with no current-revision evidence must be insufficient")
	}

	rejected := false
	for _, rejection := range rec.RejectedCandidates {
		if rejection.NodeName == "ROTATED" && rejection.Code == policy.RejectOtherConfigRevision {
			rejected = true
		}
	}
	if !rejected {
		t.Fatalf("expected ROTATED to be rejected with %s, got %+v", policy.RejectOtherConfigRevision, rec.RejectedCandidates)
	}
	if rec.RecommendedNode != nil {
		t.Fatalf("the old revision must never be used to recommend the current revision, got %+v", rec.RecommendedNode)
	}
	if rec.Decision != policy.DecisionStay {
		t.Fatalf("expected stay (healthy current node, no eligible candidate), got %s", rec.Decision)
	}
}

// TestAppService_GetMonitorRecommendation_RequiresAuthoritativeCandidateSource is the B-02
// guard: without the current job definition there is no authoritative candidate set, so the
// endpoint refuses instead of inventing one from recent history.
func TestAppService_GetMonitorRecommendation_RequiresAuthoritativeCandidateSource(t *testing.T) {
	svc, hStore := newEvidenceTestService(t)

	pol := policy.DefaultSwitchPolicy()
	pol.Mode = policy.ModeRecommend
	if err := svc.UpdateSwitchPolicy(context.Background(), pol); err != nil {
		t.Fatalf("UpdateSwitchPolicy: %v", err)
	}

	node := evidenceTestNode("HISTORY-ONLY", "10.96.0.1", 8388, "pw-h")
	monitor.PopulateNodeKeys(&node)
	now := time.Now()
	insertEvidenceSamples(t, hStore, node, "prof-h", "h", now, 10*time.Second, 30*time.Second, 30, true, 10, "")

	// Rich recent history exists, but there is no job: it must NOT become a candidate set.
	_, err := svc.GetMonitorRecommendation(context.Background(), MonitorRecommendationRequest{
		CurrentNodeKey: node.NodeKey,
	})
	if err == nil {
		t.Fatalf("a request without job_id must be rejected: history alone is not a candidate source")
	}
	if !monitor.IsValidationError(err) {
		t.Fatalf("expected a validation error, got %v", err)
	}
}

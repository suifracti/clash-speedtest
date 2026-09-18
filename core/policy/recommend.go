package policy

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// =============================================================================
// PR#7 — Recommendation output model.
//
// This is the read-only half of the orchestrator. It consumes an EvidenceSnapshot
// and produces an explainable recommendation. It never executes anything:
//
//	SelectNodeCalls == 0   (by construction — no controller is even reachable here)
//	Executed        == false
//	AdvisoryOnly    == true
//	AutoImplemented == false
//
// The stay/switch core decision is delegated to the existing DecisionEngine.Evaluate
// so there is exactly ONE policy model in the codebase.
// =============================================================================

// RecommendationDecision is the top-level verdict of an evidence-backed evaluation.
type RecommendationDecision string

const (
	// DecisionStay means: keep the current node. The current node is healthy and no
	// candidate clears the evidence gate plus the anti-flapping thresholds.
	DecisionStay RecommendationDecision = "stay"

	// DecisionRecommendSwitch means: a candidate clears the evidence gate and the
	// policy thresholds. It is a *recommendation only* — nothing is executed.
	DecisionRecommendSwitch RecommendationDecision = "recommend_switch"

	// DecisionInsufficientEvidence means: the available evidence is not sufficient to
	// justify any verdict. Stale or thin data must never drive a recommendation.
	DecisionInsufficientEvidence RecommendationDecision = "insufficient_evidence"
)

// Reason severities.
const (
	SeverityInfo    = "info"
	SeverityWarning = "warning"
	SeverityBlocker = "blocker"
)

// Recommendation reason codes (stable, machine-readable).
const (
	ReasonCurrentNodeUnresolved   = "current_node_unresolved"
	ReasonCurrentFreshness        = "current_freshness"
	ReasonCurrentSuccessRate      = "current_success_rate"
	ReasonCurrentLatency          = "current_latency"
	ReasonCurrentConsecutiveFails = "current_consecutive_transport_failures"
	ReasonCurrentTransportDown    = "current_transport_unhealthy"
	ReasonCurrentServiceOnlyFail  = "current_service_failure_not_transport"
	ReasonCurrentAIBlock          = "current_ai_region_blocked"
	ReasonInsufficientEvidence    = "insufficient_evidence"
	ReasonLockedNodePins          = "locked_node_pins_selection"
	ReasonEngineDecision          = "policy_engine_decision"
	ReasonFreshProbeRequired      = "fresh_probe_required"
	ReasonStaleCandidates         = "stale_candidates"
	ReasonRecommendedSuccessRate  = "candidate_success_rate"
	ReasonRecommendedP95          = "candidate_p95_improvement"
	ReasonRecommendedP50          = "candidate_p50_improvement"
	ReasonRecommendedFreshness    = "candidate_freshness"
	ReasonStayCurrentStable       = "stay_current_stable"
	ReasonNoEligibleCandidate     = "no_eligible_candidate"
	ReasonNoCandidateNodes        = "no_candidate_nodes"
	ReasonEvidenceConfidence      = "evidence_confidence"
	ReasonConfiguredModeNotice    = "configured_mode_notice"
)

// Candidate rejection codes (stable, machine-readable).
const (
	RejectNotInWhitelist                = "not_in_candidate_whitelist"
	RejectNoEvidence                    = "no_evidence"
	RejectStaleEvidence                 = "stale_evidence"
	RejectInsufficientSampleCount       = "insufficient_sample_count"
	RejectInsufficientObservationWindow = "insufficient_observation_window"
	RejectOtherConfigRevision           = "config_revision_mismatch"
	RejectUnknownConfigRevision         = "unknown_config_revision"
	RejectTransportUnhealthy            = "transport_unhealthy"
	RejectAIRegionBlocked               = "ai_region_blocked"
	RejectNoLatencyBaseline             = "no_latency_baseline"
	RejectNoSignificantImprovement      = "no_significant_latency_improvement"
	RejectHysteresisNotMet              = "hysteresis_not_met"
	RejectNotBestCandidate              = "not_best_candidate"
)

// RecommendationReason is one explainable statement backed by real evidence.
type RecommendationReason struct {
	Code     string         `json:"code"`
	Severity string         `json:"severity"`
	Message  string         `json:"message"`
	NodeKey  string         `json:"node_key,omitempty"`
	NodeName string         `json:"node_name,omitempty"`
	Evidence map[string]any `json:"evidence,omitempty"`
}

// CandidateRejection explains why a specific node was excluded from consideration.
type CandidateRejection struct {
	NodeKey           string         `json:"node_key"`
	NodeIdentityKey   string         `json:"node_identity_key,omitempty"`
	ConfigRevisionKey string         `json:"config_revision_key,omitempty"`
	NodeName          string         `json:"node_name"`
	Code              string         `json:"code"`
	Reason            string         `json:"reason"`
	Evidence          map[string]any `json:"evidence,omitempty"`
}

// NodeRecommendationRef is a self-contained evidence summary for one node.
type NodeRecommendationRef struct {
	NodeKey           string `json:"node_key"`
	NodeIdentityKey   string `json:"node_identity_key,omitempty"`
	ConfigRevisionKey string `json:"config_revision_key,omitempty"`
	DisplayName       string `json:"display_name"`
	IsCurrent         bool   `json:"is_current"`

	Freshness         FreshnessStatus `json:"freshness"`
	TransportHealthy  bool            `json:"transport_healthy"`
	BlockedForPurpose bool            `json:"blocked_for_purpose"`
	ServiceDegraded   bool            `json:"service_degraded"`
	TriageStatus      string          `json:"triage_status"`

	SampleCount         int     `json:"sample_count"`
	SuccessCount        int     `json:"success_count"`
	FailureCount        int     `json:"failure_count"`
	SuccessRate         float64 `json:"success_rate"`
	ConsecutiveFailures int     `json:"consecutive_failures"`

	TransportFailureCount int `json:"transport_failure_count"`
	ServiceFailureCount   int `json:"service_failure_count"`
	AIBlockCount          int `json:"ai_block_count"`

	LatencySampleCount int           `json:"latency_sample_count"`
	LatencyP50         time.Duration `json:"latency_p50"`
	LatencyP95         time.Duration `json:"latency_p95"`
	TTFBP50            time.Duration `json:"ttfb_p50"`
	TTFBP95            time.Duration `json:"ttfb_p95"`

	ObservationWindow time.Duration `json:"observation_window"`
	FirstSampleAt     time.Time     `json:"first_sample_at"`
	LastSampleAt      time.Time     `json:"last_sample_at"`
	SampleAge         time.Duration `json:"sample_age"`

	ErrorBreakdown map[string]int `json:"error_breakdown,omitempty"`
}

// ConfidenceBasis documents exactly how Confidence was computed. A bare score is
// never emitted without its derivation.
type ConfidenceBasis struct {
	Score           float64 `json:"score"`
	SampleDepth     float64 `json:"sample_depth"`
	WindowAdequacy  float64 `json:"window_adequacy"`
	FreshnessFactor float64 `json:"freshness_factor"`
	Formula         string  `json:"formula"`
	BoundingNode    string  `json:"bounding_node,omitempty"`
	Detail          string  `json:"detail"`
}

// MonitorRecommendation is the complete, explainable, non-executing output of PR#7.
type MonitorRecommendation struct {
	GeneratedAt time.Time `json:"generated_at"`

	JobID     string `json:"job_id,omitempty"`
	ProfileID string `json:"profile_id,omitempty"`

	Purpose PolicyPurpose `json:"purpose"`
	// ConfiguredMode is the user's configured orchestrator mode (recorded, never acted upon).
	ConfiguredMode OrchestratorMode `json:"configured_mode"`
	// EvaluationMode is the mode the advisory path evaluated under. It is always
	// ModeRecommend, whose existing definition is "produce a recommendation with full
	// rationale but require explicit user confirmation before executing".
	EvaluationMode OrchestratorMode `json:"evaluation_mode"`

	// AdvisoryOnly and Executed are structural guarantees of this PR.
	AdvisoryOnly bool `json:"advisory_only"`
	Executed     bool `json:"executed"`
	// SelectNodeCalls is always 0: this path holds no controller reference at all.
	SelectNodeCalls int `json:"select_node_calls"`
	// AutoImplemented is always false: automatic execution is out of scope for PR#7.
	AutoImplemented bool `json:"auto_implemented"`

	CurrentNode     *NodeRecommendationRef `json:"current_node,omitempty"`
	RecommendedNode *NodeRecommendationRef `json:"recommended_node,omitempty"`

	Decision RecommendationDecision `json:"decision"`

	Confidence          float64             `json:"confidence"`
	ConfidenceBasis     ConfidenceBasis     `json:"confidence_basis"`
	EvidenceSufficiency EvidenceSufficiency `json:"evidence_sufficiency"`

	Freshness         FreshnessStatus `json:"freshness"`
	ObservationWindow time.Duration   `json:"observation_window"`
	SampleCount       int             `json:"sample_count"`

	Reasons            []RecommendationReason `json:"reasons"`
	RejectedCandidates []CandidateRejection   `json:"rejected_candidates"`

	Gate EvidenceGate `json:"gate"`

	// Snapshot is the full evidence bundle, preserved so every statement above can be
	// traced back to its provenance and observation window.
	Snapshot *EvidenceSnapshot `json:"snapshot,omitempty"`
}

func (r *MonitorRecommendation) addReason(code, severity, message string, ev *NodeEvidence, extra map[string]any) {
	reason := RecommendationReason{
		Code:     code,
		Severity: severity,
		Message:  message,
		Evidence: extra,
	}
	if ev != nil {
		reason.NodeKey = ev.NodeKey
		reason.NodeName = ev.DisplayName
	}
	r.Reasons = append(r.Reasons, reason)
}

// RecommendFromEvidence is the read-only evidence → recommendation entry point.
//
// It is a pure function of (now, policy, state, snapshot): it does not mutate the
// passed DecisionState, does not touch the controller, and cannot switch anything.
//
// The stay/switch core decision is produced by the existing DecisionEngine.Evaluate;
// this function only (a) builds the evidence-gated candidate set, and (b) turns the
// engine's verdict into a fully explainable recommendation.
func (e *DecisionEngine) RecommendFromEvidence(
	now time.Time,
	p SwitchPolicy,
	state *DecisionState,
	snap EvidenceSnapshot,
) MonitorRecommendation {
	rec := MonitorRecommendation{
		GeneratedAt:        now,
		JobID:              snap.Source.JobID,
		ProfileID:          snap.Source.ProfileID,
		Purpose:            snap.Purpose,
		ConfiguredMode:     p.Mode,
		EvaluationMode:     ModeRecommend,
		AdvisoryOnly:       true,
		Executed:           false,
		SelectNodeCalls:    0,
		AutoImplemented:    false,
		Reasons:            []RecommendationReason{},
		RejectedCandidates: []CandidateRejection{},
		Gate:               snap.Gate,
		Snapshot:           &snap,
	}

	// The snapshot's purpose is authoritative for the whole evaluation: it is the lens
	// the evidence was classified under. The engine is handed the same value so purpose
	// semantics can never diverge between evidence building and decision making.
	purpose := snap.Purpose
	if purpose == "" {
		purpose = PurposeGeneral
	}
	rec.Purpose = purpose

	pEval := p
	pEval.Mode = ModeRecommend // advisory: "produce a recommendation, execute nothing"
	pEval.Purpose = purpose

	if p.Mode == ModeMonitorOnly {
		rec.addReason(ReasonConfiguredModeNotice, SeverityInfo,
			"当前编排模式为 monitor_only：本结果仅为只读评估，任何模式下都不会被自动执行", nil,
			map[string]any{"configured_mode": string(p.Mode)})
	}

	cur := snap.CurrentNode()
	if cur == nil {
		rec.Decision = DecisionInsufficientEvidence
		rec.Freshness = FreshnessNoEvidence
		rec.EvidenceSufficiency = EvidenceSufficiency{
			Sufficient: false,
			Reasons:    []string{ReasonCurrentNodeUnresolved},
			Details:    []string{"无法在证据快照中定位当前节点（未提供当前节点标识，且未能从控制器当前选择解析）"},
		}
		rec.ConfidenceBasis = ConfidenceBasis{Score: 0, Detail: "当前节点未知，不计算置信度"}
		rec.addReason(ReasonCurrentNodeUnresolved, SeverityBlocker,
			"无法确定当前节点，无法进行任何比较", nil, nil)
		return rec
	}

	rec.CurrentNode = refFromEvidence(cur)
	rec.Freshness = cur.Freshness
	rec.ObservationWindow = cur.ObservationWindow
	rec.SampleCount = cur.SampleCount
	rec.EvidenceSufficiency = cur.Sufficiency

	appendCurrentNodeReasons(&rec, cur, pEval)

	// --- Locked node: reuse the existing policy semantic (pinning suspends switching). ---
	if p.LockedNode != "" {
		rec.Decision = DecisionStay
		rec.ConfidenceBasis = ConfidenceBasis{Score: 0, Detail: "节点已锁定，未进入候选比较"}
		rec.addReason(ReasonLockedNodePins, SeverityInfo,
			fmt.Sprintf("策略已锁定节点 %s，按既有语义暂停所有切换建议", p.LockedNode), cur,
			map[string]any{"locked_node": p.LockedNode})
		if !cur.Sufficiency.Sufficient {
			appendInsufficientEvidenceReasons(&rec, cur)
		}
		return rec
	}

	// --- Freshness / evidence gate on the current node. ---
	// Without trustworthy current-node evidence there is no basis for any verdict.
	if !cur.Sufficiency.Sufficient {
		rec.Decision = DecisionInsufficientEvidence
		rec.ConfidenceBasis = ConfidenceBasis{Score: 0, Detail: "当前节点证据不足，不计算置信度"}
		appendInsufficientEvidenceReasons(&rec, cur)
		return rec
	}

	// --- Candidate selection: apply the same gate to every candidate. ---
	curIndex := indexOfNode(&snap, cur)
	nodeNames := assignEvaluationNames(&snap)

	var evals []NodeEvaluation
	evals = append(evals, toNodeEvaluation(nodeNames[curIndex], cur))
	passedGate := map[int]struct{}{curIndex: {}}

	whitelistActive := len(p.CandidateNodes) > 0
	whitelist := map[string]struct{}{}
	for _, name := range p.CandidateNodes {
		whitelist[strings.TrimSpace(name)] = struct{}{}
	}

	for i := range snap.Nodes {
		if i == curIndex {
			continue
		}
		node := &snap.Nodes[i]

		if whitelistActive {
			if _, ok := whitelist[node.DisplayName]; !ok {
				rec.reject(node, RejectNotInWhitelist,
					fmt.Sprintf("候选 %s 不在策略候选白名单 CandidateNodes 内", node.DisplayName), nil)
				continue
			}
		}

		if !node.Sufficiency.Sufficient {
			code, reason := sufficiencyRejection(node)
			rec.reject(node, code, reason, map[string]any{
				"sample_count":       node.SampleCount,
				"sample_age":         node.SampleAge.String(),
				"observation_window": node.ObservationWindow.String(),
			})
			continue
		}

		// Purpose semantics: under AI purpose a confirmed region/service block is
		// disqualifying. Under general purpose it is explicitly NOT a node failure.
		if purpose == PurposeAI && node.BlockedForPurpose {
			rec.reject(node, RejectAIRegionBlocked,
				fmt.Sprintf("候选 %s 在 AI 用途下捕获地区/服务阻断证据（AI 阻断 %d 次，服务阻断 %d 次），被淘汰",
					node.DisplayName, node.AIBlockCount, node.ServiceFailureCount),
				map[string]any{
					"ai_block_count":        node.AIBlockCount,
					"service_failure_count": node.ServiceFailureCount,
					"purpose":               string(purpose),
				})
			continue
		}

		if !node.TransportHealthy {
			rec.reject(node, RejectTransportUnhealthy,
				fmt.Sprintf("候选 %s 传输层失败 %d/%d（连续 %d 次），判定为节点故障",
					node.DisplayName, node.TransportFailureCount, node.TransportSampleCount, node.ConsecutiveTransportFailures),
				map[string]any{
					"transport_failure_count": node.TransportFailureCount,
					"transport_sample_count":  node.TransportSampleCount,
					"consecutive_failures":    node.ConsecutiveTransportFailures,
					"error_breakdown":         node.ErrorBreakdown,
				})
			continue
		}

		if node.LatencyP50 <= 0 {
			rec.reject(node, RejectNoLatencyBaseline,
				fmt.Sprintf("候选 %s 传输层健康但缺少可用延迟基线（成功样本 %d 条）",
					node.DisplayName, node.SuccessCount), nil)
			continue
		}

		passedGate[i] = struct{}{}
		evals = append(evals, toNodeEvaluation(nodeNames[i], node))
	}

	if len(evals) <= 1 {
		rec.addReason(ReasonNoCandidateNodes, SeverityWarning,
			"候选池为空或全部候选被证据门槛淘汰，仅能评估当前节点", cur, nil)
	}

	// --- Delegate the core stay/switch decision to the existing policy engine. ---
	stateCopy := DecisionState{}
	if state != nil {
		stateCopy = *state
	}
	stateCopy.CurrentNode = nodeNames[curIndex]
	stateCopy.ConsecutiveFailures = cur.ConsecutiveFailures

	res := e.Evaluate(now, pEval, &stateCopy, evals)

	// In ModeRecommend the engine reports the trigger on the recommendation itself
	// (DecisionResult.TriggerType is only populated on the executing path).
	engineTrigger := res.TriggerType
	if engineTrigger == "" && res.Recommendation != nil {
		engineTrigger = res.Recommendation.TriggerType
	}

	rec.addReason(ReasonEngineDecision, SeverityInfo,
		fmt.Sprintf("策略引擎结论（%s）：%s", engineTrigger, res.Reason), nil,
		map[string]any{
			"trigger_type":  engineTrigger,
			"should_switch": res.ShouldSwitch,
			"engine_reason": res.Reason,
		})

	if res.RequiresFreshProbe && len(res.StaleNodes) > 0 {
		rec.addReason(ReasonFreshProbeRequired, SeverityWarning,
			fmt.Sprintf("以下节点样本过旧或样本数不足，需轻量复测后方可参评：%s",
				strings.Join(res.StaleNodes, ", ")), nil,
			map[string]any{"stale_nodes": res.StaleNodes})
	}

	if res.Recommendation != nil {
		if targetIdx, ok := indexOfEvaluationName(nodeNames, res.Recommendation.TargetNode); ok {
			target := &snap.Nodes[targetIdx]
			rec.Decision = DecisionRecommendSwitch
			rec.RecommendedNode = refFromEvidence(target)
			appendCandidateReasons(&rec, cur, target, pEval, res)
			appendLosingCandidates(&rec, &snap, passedGate, curIndex, targetIdx, nodeNames, cur, pEval)
			rec.ConfidenceBasis = computeConfidence(cur, target, snap.Gate)
			rec.Confidence = rec.ConfidenceBasis.Score
			rec.addReason(ReasonEvidenceConfidence, SeverityInfo,
				rec.ConfidenceBasis.Detail, nil, map[string]any{
					"score":            rec.ConfidenceBasis.Score,
					"sample_depth":     rec.ConfidenceBasis.SampleDepth,
					"window_adequacy":  rec.ConfidenceBasis.WindowAdequacy,
					"freshness_factor": rec.ConfidenceBasis.FreshnessFactor,
					"formula":          rec.ConfidenceBasis.Formula,
				})
			return rec
		}
		// Defensive: the engine named a target we cannot map back. Never fabricate.
		rec.addReason(ReasonNoEligibleCandidate, SeverityBlocker,
			fmt.Sprintf("策略引擎给出的目标节点 %s 无法映射回证据快照，拒绝输出该建议", res.Recommendation.TargetNode), nil, nil)
	}

	// No recommendation produced by the engine.
	if !cur.TransportHealthy || cur.BlockedForPurpose {
		rec.Decision = DecisionInsufficientEvidence
		rec.ConfidenceBasis = ConfidenceBasis{Score: 0, Detail: "当前节点已判定故障或受阻，但无可用替代节点，不计算置信度"}
		rec.addReason(ReasonNoEligibleCandidate, SeverityBlocker,
			"当前节点已判定故障或受阻，但候选池中没有通过证据门槛的可用替代节点，无法给出切换建议", cur,
			map[string]any{
				"transport_healthy":   cur.TransportHealthy,
				"blocked_for_purpose": cur.BlockedForPurpose,
			})
		return rec
	}

	rec.Decision = DecisionStay
	rec.addReason(ReasonStayCurrentStable, SeverityInfo,
		"当前节点传输层健康，且没有候选节点满足证据门槛与防抖阈值，维持当前选择", cur, nil)
	rec.ConfidenceBasis = computeConfidence(cur, nil, snap.Gate)
	rec.Confidence = rec.ConfidenceBasis.Score
	rec.addReason(ReasonEvidenceConfidence, SeverityInfo,
		rec.ConfidenceBasis.Detail, nil, map[string]any{
			"score":            rec.ConfidenceBasis.Score,
			"sample_depth":     rec.ConfidenceBasis.SampleDepth,
			"window_adequacy":  rec.ConfidenceBasis.WindowAdequacy,
			"freshness_factor": rec.ConfidenceBasis.FreshnessFactor,
			"formula":          rec.ConfidenceBasis.Formula,
		})
	return rec
}

func (r *MonitorRecommendation) reject(node *NodeEvidence, code, reason string, extra map[string]any) {
	r.RejectedCandidates = append(r.RejectedCandidates, CandidateRejection{
		NodeKey:           node.NodeKey,
		NodeIdentityKey:   node.NodeIdentityKey,
		ConfigRevisionKey: node.ConfigRevisionKey,
		NodeName:          node.DisplayName,
		Code:              code,
		Reason:            reason,
		Evidence:          extra,
	})
}

// sufficiencyRejection maps a failed evidence gate onto a stable rejection code.
func sufficiencyRejection(node *NodeEvidence) (string, string) {
	detail := ""
	if len(node.Sufficiency.Details) > 0 {
		detail = node.Sufficiency.Details[0]
	}
	for _, code := range node.Sufficiency.Reasons {
		switch code {
		case GateReasonStaleEvidence:
			return RejectStaleEvidence, detail
		case GateReasonInsufficientSampleCount:
			return RejectInsufficientSampleCount, detail
		case GateReasonInsufficientObservationWindow:
			return RejectInsufficientObservationWindow, detail
		case GateReasonOtherRevisionSamplesExcluded:
			return RejectOtherConfigRevision, detail
		case GateReasonUnknownRevisionSamplesExcluded:
			return RejectUnknownConfigRevision, detail
		case GateReasonNoEvidence:
			return RejectNoEvidence, detail
		}
	}
	if detail == "" {
		detail = fmt.Sprintf("候选 %s 未通过证据门槛", node.DisplayName)
	}
	return RejectNoEvidence, detail
}

func appendInsufficientEvidenceReasons(rec *MonitorRecommendation, cur *NodeEvidence) {
	rec.addReason(ReasonInsufficientEvidence, SeverityBlocker,
		fmt.Sprintf("当前节点 %s 证据不足，按保守门槛输出 insufficient_evidence，拒绝给出硬推荐", cur.DisplayName),
		cur, map[string]any{
			"gate_reasons":       cur.Sufficiency.Reasons,
			"sample_count":       cur.SampleCount,
			"sample_age":         cur.SampleAge.String(),
			"observation_window": cur.ObservationWindow.String(),
			"freshness":          string(cur.Freshness),
		})
	for i, detail := range cur.Sufficiency.Details {
		code := ReasonInsufficientEvidence
		if i < len(cur.Sufficiency.Reasons) {
			code = cur.Sufficiency.Reasons[i]
		}
		rec.addReason(code, SeverityBlocker, detail, cur, nil)
	}
}

// appendCurrentNodeReasons explains the current node's state from real evidence.
func appendCurrentNodeReasons(rec *MonitorRecommendation, cur *NodeEvidence, p SwitchPolicy) {
	switch cur.Freshness {
	case FreshnessFresh:
		rec.addReason(ReasonCurrentFreshness, SeverityInfo,
			fmt.Sprintf("当前节点 %s 最新样本距今 %s，处于新鲜窗口内（MaxSampleAge=%s）",
				cur.DisplayName, roundDuration(cur.SampleAge), roundDuration(rec.Gate.MaxSampleAge)),
			cur, map[string]any{
				"freshness":          string(cur.Freshness),
				"sample_age":         cur.SampleAge.String(),
				"last_sample_at":     cur.LastSampleAt,
				"max_sample_age":     rec.Gate.MaxSampleAge.String(),
				"observation_window": cur.ObservationWindow.String(),
			})
	case FreshnessStale:
		rec.addReason(ReasonCurrentFreshness, SeverityBlocker,
			fmt.Sprintf("当前节点 %s 最新样本距今 %s，已超过 MaxSampleAge=%s，陈旧数据不得驱动推荐",
				cur.DisplayName, roundDuration(cur.SampleAge), roundDuration(rec.Gate.MaxSampleAge)),
			cur, map[string]any{
				"freshness":      string(cur.Freshness),
				"sample_age":     cur.SampleAge.String(),
				"last_sample_at": cur.LastSampleAt,
				"max_sample_age": rec.Gate.MaxSampleAge.String(),
			})
	default:
		rec.addReason(ReasonCurrentFreshness, SeverityBlocker,
			fmt.Sprintf("当前节点 %s 在观察回看窗口内没有任何原始样本", cur.DisplayName),
			cur, map[string]any{"freshness": string(cur.Freshness)})
	}

	rec.addReason(ReasonCurrentSuccessRate, SeverityInfo,
		fmt.Sprintf("当前节点 %s 观察窗口内成功率 %.2f%%（成功 %d / 失败 %d，共 %d 条样本）",
			cur.DisplayName, cur.SuccessRate*100, cur.SuccessCount, cur.FailureCount, cur.SampleCount),
		cur, map[string]any{
			"success_rate":  cur.SuccessRate,
			"success_count": cur.SuccessCount,
			"failure_count": cur.FailureCount,
			"sample_count":  cur.SampleCount,
		})

	rec.addReason(ReasonCurrentLatency, SeverityInfo,
		fmt.Sprintf("当前节点 %s 延迟 P50=%s / P95=%s，TTFB P50=%s / P95=%s",
			cur.DisplayName, roundDuration(cur.LatencyP50), roundDuration(cur.LatencyP95),
			roundDuration(cur.TTFBP50), roundDuration(cur.TTFBP95)),
		cur, map[string]any{
			"latency_p50": cur.LatencyP50.String(),
			"latency_p95": cur.LatencyP95.String(),
			"ttfb_p50":    cur.TTFBP50.String(),
			"ttfb_p95":    cur.TTFBP95.String(),
		})

	if cur.ConsecutiveTransportFailures > 0 {
		severity := SeverityWarning
		if !cur.TransportHealthy {
			severity = SeverityBlocker
		}
		rec.addReason(ReasonCurrentConsecutiveFails, severity,
			fmt.Sprintf("当前节点 %s 最近连续 %d 次传输探测失败（阈值 MaxConsecutiveFailures=%d）",
				cur.DisplayName, cur.ConsecutiveTransportFailures, p.MaxConsecutiveFailures),
			cur, map[string]any{
				"consecutive_transport_failures": cur.ConsecutiveTransportFailures,
				"max_consecutive_failures":       p.MaxConsecutiveFailures,
				"error_breakdown":                cur.ErrorBreakdown,
			})
	}

	if !cur.TransportHealthy {
		rec.addReason(ReasonCurrentTransportDown, SeverityBlocker,
			fmt.Sprintf("当前节点 %s 传输层失败 %d/%d，判定为传输层故障",
				cur.DisplayName, cur.TransportFailureCount, cur.TransportSampleCount),
			cur, map[string]any{
				"transport_failure_count": cur.TransportFailureCount,
				"transport_sample_count":  cur.TransportSampleCount,
				"error_breakdown":         cur.ErrorBreakdown,
			})
	}

	// Purpose semantics: service-only blocking is NOT a transport failure under general purpose.
	if cur.ServiceFailureCount > 0 {
		if p.Purpose == PurposeAI {
			rec.addReason(ReasonCurrentAIBlock, SeverityBlocker,
				fmt.Sprintf("当前节点 %s 在 AI 用途下捕获服务/地区阻断 %d 次（AI 阻断 %d 次），按 AI 语义视为硬失败",
					cur.DisplayName, cur.ServiceFailureCount, cur.AIBlockCount),
				cur, map[string]any{
					"service_failure_count": cur.ServiceFailureCount,
					"ai_block_count":        cur.AIBlockCount,
					"purpose":               string(p.Purpose),
				})
		} else {
			rec.addReason(ReasonCurrentServiceOnlyFail, SeverityInfo,
				fmt.Sprintf("当前节点 %s 存在 %d 次服务级阻断（Google/AI 等），但传输层正常；general 用途下不计为节点故障",
					cur.DisplayName, cur.ServiceFailureCount),
				cur, map[string]any{
					"service_failure_count": cur.ServiceFailureCount,
					"transport_healthy":     cur.TransportHealthy,
					"purpose":               string(p.Purpose),
				})
		}
	}
}

// appendCandidateReasons explains why the recommended node won.
func appendCandidateReasons(
	rec *MonitorRecommendation,
	cur *NodeEvidence,
	target *NodeEvidence,
	p SwitchPolicy,
	res DecisionResult,
) {
	rec.addReason(ReasonRecommendedSuccessRate, SeverityInfo,
		fmt.Sprintf("候选 %s 观察窗口内成功率 %.2f%%（成功 %d / 共 %d 条样本，窗口 %s）",
			target.DisplayName, target.SuccessRate*100, target.SuccessCount, target.SampleCount,
			roundDuration(target.ObservationWindow)),
		target, map[string]any{
			"success_rate":       target.SuccessRate,
			"success_count":      target.SuccessCount,
			"sample_count":       target.SampleCount,
			"observation_window": target.ObservationWindow.String(),
		})

	if cur.LatencyP95 > 0 && target.LatencyP95 > 0 {
		delta := cur.LatencyP95 - target.LatencyP95
		message := fmt.Sprintf("候选 %s P95=%s 与当前节点 P95=%s 相差 %s",
			target.DisplayName, roundDuration(target.LatencyP95), roundDuration(cur.LatencyP95), roundDuration(delta))
		if delta > 0 {
			message = fmt.Sprintf("候选 %s P95=%s 比当前节点 P95=%s 低 %s",
				target.DisplayName, roundDuration(target.LatencyP95), roundDuration(cur.LatencyP95), roundDuration(delta))
		}
		rec.addReason(ReasonRecommendedP95, SeverityInfo, message, target, map[string]any{
			"target_p95":  target.LatencyP95.String(),
			"current_p95": cur.LatencyP95.String(),
			"delta":       delta.String(),
		})
	}

	if cur.LatencyP50 > 0 && target.LatencyP50 > 0 {
		delta := cur.LatencyP50 - target.LatencyP50
		ratio := 0.0
		if cur.LatencyP50 > 0 {
			ratio = float64(delta) / float64(cur.LatencyP50)
		}
		rec.addReason(ReasonRecommendedP50, SeverityInfo,
			fmt.Sprintf("候选 %s P50=%s 相对当前节点 P50=%s 改善 %s（%.1f%%），满足 MinImprovementRTT=%s / MinImprovementRatio=%.2f",
				target.DisplayName, roundDuration(target.LatencyP50), roundDuration(cur.LatencyP50),
				roundDuration(delta), ratio*100,
				roundDuration(p.MinImprovementRTT), p.MinImprovementRatio),
			target, map[string]any{
				"target_p50":            target.LatencyP50.String(),
				"current_p50":           cur.LatencyP50.String(),
				"delta":                 delta.String(),
				"improvement_ratio":     ratio,
				"min_improvement_rtt":   p.MinImprovementRTT.String(),
				"min_improvement_ratio": p.MinImprovementRatio,
				"trigger_type":          res.TriggerType,
			})
	}

	rec.addReason(ReasonRecommendedFreshness, SeverityInfo,
		fmt.Sprintf("候选 %s 最新样本距今 %s，处于新鲜窗口内", target.DisplayName, roundDuration(target.SampleAge)),
		target, map[string]any{
			"sample_age":     target.SampleAge.String(),
			"last_sample_at": target.LastSampleAt,
			"freshness":      string(target.Freshness),
		})
}

// appendLosingCandidates records why every other gate-passing candidate lost.
func appendLosingCandidates(
	rec *MonitorRecommendation,
	snap *EvidenceSnapshot,
	passedGate map[int]struct{},
	curIndex int,
	targetIndex int,
	nodeNames []string,
	cur *NodeEvidence,
	p SwitchPolicy,
) {
	for i := range snap.Nodes {
		if i == curIndex || i == targetIndex {
			continue
		}
		if _, ok := passedGate[i]; !ok {
			continue
		}
		node := &snap.Nodes[i]

		delta := cur.LatencyP50 - node.LatencyP50
		ratio := 0.0
		if cur.LatencyP50 > 0 {
			ratio = float64(delta) / float64(cur.LatencyP50)
		}

		code := RejectNotBestCandidate
		reason := fmt.Sprintf("候选 %s 通过证据门槛但未被选中：P50=%s 未优于被推荐节点的 %s",
			node.DisplayName, roundDuration(node.LatencyP50), roundDuration(snap.Nodes[targetIndex].LatencyP50))

		switch {
		case p.MinImprovementRTT > 0 && delta < p.MinImprovementRTT:
			code = RejectNoSignificantImprovement
			reason = fmt.Sprintf("候选 %s 相对当前节点 P50 改善 %s，低于 MinImprovementRTT=%s",
				node.DisplayName, roundDuration(delta), roundDuration(p.MinImprovementRTT))
		case p.MinImprovementRatio > 0 && ratio < p.MinImprovementRatio:
			code = RejectNoSignificantImprovement
			reason = fmt.Sprintf("候选 %s 相对当前节点 P50 改善比例 %.1f%%，低于 MinImprovementRatio=%.2f",
				node.DisplayName, ratio*100, p.MinImprovementRatio)
		case p.HysteresisBuffer > 0 && node.LatencyP50 >= cur.LatencyP50-p.HysteresisBuffer:
			code = RejectHysteresisNotMet
			reason = fmt.Sprintf("候选 %s P50=%s 未低于当前节点 P50=%s 减去防抖缓冲 HysteresisBuffer=%s",
				node.DisplayName, roundDuration(node.LatencyP50), roundDuration(cur.LatencyP50),
				roundDuration(p.HysteresisBuffer))
		}

		rec.reject(node, code, reason, map[string]any{
			"latency_p50":       node.LatencyP50.String(),
			"current_p50":       cur.LatencyP50.String(),
			"delta":             delta.String(),
			"improvement_ratio": ratio,
		})
	}
}

// computeConfidence derives the confidence score from real evidence, and documents it.
// When a recommendation exists, the weaker of (current, recommended) bounds the score.
func computeConfidence(cur, target *NodeEvidence, gate EvidenceGate) ConfidenceBasis {
	curScore, curDetail, curDepth, curWindow, curFresh := confidenceFor(cur, gate)
	basis := ConfidenceBasis{
		Score:           curScore,
		SampleDepth:     curDepth,
		WindowAdequacy:  curWindow,
		FreshnessFactor: curFresh,
		Formula:         "0.40*sample_depth + 0.30*window_adequacy + 0.30*freshness_factor",
		BoundingNode:    cur.DisplayName,
		Detail:          curDetail,
	}

	if target != nil {
		tScore, tDetail, tDepth, tWindow, tFresh := confidenceFor(target, gate)
		if tScore < basis.Score {
			basis.Score = tScore
			basis.SampleDepth = tDepth
			basis.WindowAdequacy = tWindow
			basis.FreshnessFactor = tFresh
			basis.BoundingNode = target.DisplayName
			basis.Detail = fmt.Sprintf("取当前节点与推荐节点中证据质量较低者：%s", tDetail)
		}
	}
	basis.Score = math.Round(basis.Score*10000) / 10000
	return basis
}

func confidenceFor(ev *NodeEvidence, gate EvidenceGate) (score float64, detail string, depth, window, fresh float64) {
	depth = 1.0
	depthDetail := "MinSampleCount<=0，样本深度按 1.00 计"
	if gate.MinSampleCount > 0 {
		depth = math.Min(1, float64(ev.SampleCount)/float64(2*gate.MinSampleCount))
		depthDetail = fmt.Sprintf("min(1, 样本数 %d / (2*MinSampleCount %d)) = %.2f",
			ev.SampleCount, 2*gate.MinSampleCount, depth)
	}

	window = 1.0
	windowDetail := "MinObservationWindow<=0，窗口充足度按 1.00 计"
	if gate.MinObservationWindow > 0 {
		window = math.Min(1, float64(ev.ObservationWindow)/float64(2*gate.MinObservationWindow))
		windowDetail = fmt.Sprintf("min(1, 观察窗口 %s / (2*MinObservationWindow %s)) = %.2f",
			roundDuration(ev.ObservationWindow), roundDuration(2*gate.MinObservationWindow), window)
	}

	fresh = 1.0
	freshDetail := "MaxSampleAge<=0，新鲜度按 1.00 计"
	if gate.MaxSampleAge > 0 {
		fresh = 1 - float64(ev.SampleAge)/float64(gate.MaxSampleAge)
		if fresh < 0 {
			fresh = 0
		}
		if fresh > 1 {
			fresh = 1
		}
		freshDetail = fmt.Sprintf("1 - 样本年龄 %s / MaxSampleAge %s = %.2f",
			roundDuration(ev.SampleAge), roundDuration(gate.MaxSampleAge), fresh)
	}

	score = 0.40*depth + 0.30*window + 0.30*fresh
	detail = fmt.Sprintf("节点 %s：%s；%s；%s；置信度 = 0.40*%.2f + 0.30*%.2f + 0.30*%.2f = %.4f",
		ev.DisplayName, depthDetail, windowDetail, freshDetail, depth, window, fresh, score)
	return score, detail, depth, window, fresh
}

// assignEvaluationNames gives every node a stable, unique name for the policy engine.
// Duplicate display names are disambiguated so two nodes can never collapse into one.
func assignEvaluationNames(snap *EvidenceSnapshot) []string {
	names := make([]string, len(snap.Nodes))
	taken := map[string]struct{}{}
	for i := range snap.Nodes {
		base := strings.TrimSpace(snap.Nodes[i].DisplayName)
		if base == "" {
			base = snap.Nodes[i].NodeKey
		}
		if base == "" {
			base = fmt.Sprintf("node-%d", i)
		}
		name := base
		for n := 2; ; n++ {
			if _, exists := taken[name]; !exists {
				break
			}
			name = fmt.Sprintf("%s#%d", base, n)
		}
		taken[name] = struct{}{}
		names[i] = name
	}
	return names
}

func indexOfEvaluationName(names []string, name string) (int, bool) {
	for i, n := range names {
		if n == name {
			return i, true
		}
	}
	return 0, false
}

func indexOfNode(snap *EvidenceSnapshot, node *NodeEvidence) int {
	for i := range snap.Nodes {
		if &snap.Nodes[i] == node {
			return i
		}
	}
	for i := range snap.Nodes {
		if snap.Nodes[i].NodeKey == node.NodeKey {
			return i
		}
	}
	return 0
}

// toNodeEvaluation projects evidence onto the existing NodeEvaluation contract so the
// legacy DecisionEngine can be reused verbatim instead of reimplemented.
func toNodeEvaluation(name string, ev *NodeEvidence) NodeEvaluation {
	return NodeEvaluation{
		Name:              name,
		Available:         ev.TransportHealthy,
		RTT:               ev.LatencyP50,
		Loss:              0,
		Bandwidth:         0,
		TriageStatus:      ev.TriageStatus,
		SampleCount:       ev.SampleCount,
		FirstSampleTime:   ev.FirstSampleAt,
		LastSampleTime:    ev.LastSampleAt,
		ObservationWindow: ev.ObservationWindow,
	}
}

func refFromEvidence(ev *NodeEvidence) *NodeRecommendationRef {
	if ev == nil {
		return nil
	}
	return &NodeRecommendationRef{
		NodeKey:               ev.NodeKey,
		NodeIdentityKey:       ev.NodeIdentityKey,
		ConfigRevisionKey:     ev.ConfigRevisionKey,
		DisplayName:           ev.DisplayName,
		IsCurrent:             ev.IsCurrent,
		Freshness:             ev.Freshness,
		TransportHealthy:      ev.TransportHealthy,
		BlockedForPurpose:     ev.BlockedForPurpose,
		ServiceDegraded:       ev.ServiceDegraded,
		TriageStatus:          ev.TriageStatus,
		SampleCount:           ev.SampleCount,
		SuccessCount:          ev.SuccessCount,
		FailureCount:          ev.FailureCount,
		SuccessRate:           ev.SuccessRate,
		ConsecutiveFailures:   ev.ConsecutiveFailures,
		TransportFailureCount: ev.TransportFailureCount,
		ServiceFailureCount:   ev.ServiceFailureCount,
		AIBlockCount:          ev.AIBlockCount,
		LatencySampleCount:    ev.LatencySampleCount,
		LatencyP50:            ev.LatencyP50,
		LatencyP95:            ev.LatencyP95,
		TTFBP50:               ev.TTFBP50,
		TTFBP95:               ev.TTFBP95,
		ObservationWindow:     ev.ObservationWindow,
		FirstSampleAt:         ev.FirstSampleAt,
		LastSampleAt:          ev.LastSampleAt,
		SampleAge:             ev.SampleAge,
		ErrorBreakdown:        ev.ErrorBreakdown,
	}
}

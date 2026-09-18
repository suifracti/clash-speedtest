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

	// DecisionNoEligibleCandidate means: the current node's own evidence WAS sufficient
	// (it cleared every gate), but the node is judged failed or blocked and no alternative
	// candidate cleared the evidence gate either.
	//
	// This is deliberately NOT DecisionInsufficientEvidence. Reporting it as
	// "insufficient_evidence" would contradict the very same payload, which carries
	// EvidenceSufficiency.Sufficient == true, and would describe an operational outage
	// ("the node I am on is down and there is nothing to move to") as a data problem
	// ("come back when there are more samples") — the opposite of what an operator needs
	// to act on.
	DecisionNoEligibleCandidate RecommendationDecision = "no_eligible_candidate"
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

	// Mode semantics. The configured mode is never silently bypassed: either it is
	// respected (monitor_only suppresses recommendations), or the caller explicitly
	// asked for a preview.
	ReasonMonitorOnlySuppresses = "monitor_only_suppresses_recommendation"
	ReasonPreviewNotice         = "preview_mode_notice"
	ReasonAutoNotImplemented    = "auto_execution_not_implemented"
	// ReasonInvalidMode is emitted when the configured orchestrator mode is not one of
	// ValidOrchestratorModes. The evaluation is fail-closed (no candidate comparison, no
	// recommendation) and the boundary surfaces this as HTTP 400.
	ReasonInvalidMode = "invalid_orchestrator_mode"

	// Evidence isolation provenance.
	ReasonProfileIsolation = "profile_isolation"
)

// Candidate rejection codes (stable, machine-readable).
const (
	RejectNotInWhitelist          = "not_in_candidate_whitelist"
	RejectNoEvidence              = "no_evidence"
	RejectStaleEvidence           = "stale_evidence"
	RejectInsufficientSampleCount = "insufficient_sample_count"
	// RejectInsufficientLatencySamples means the node had enough samples in total but too few
	// transport-scope latency observations for its P50 to be ranked on.
	RejectInsufficientLatencySamples = "insufficient_latency_samples"
	// RejectEvidenceBudgetExceeded means the node's raw-sample window could not be drained
	// completely, so its statistics are partial. Distinct from no_evidence: evidence exists,
	// it just could not all be read.
	RejectEvidenceBudgetExceeded        = "evidence_budget_exceeded"
	RejectInsufficientObservationWindow = "insufficient_observation_window"
	RejectOtherConfigRevision           = "config_revision_mismatch"
	RejectUnknownConfigRevision         = "unknown_config_revision"
	RejectOtherProfile                  = "profile_mismatch"
	RejectUnknownProfile                = "unknown_profile"
	RejectProfileIsolationAmbiguous     = "profile_isolation_unavailable"
	RejectTransportUnhealthy            = "transport_unhealthy"
	RejectAIRegionBlocked               = "ai_region_blocked"
	RejectNoLatencyBaseline             = "no_latency_baseline"
	RejectNoSignificantImprovement      = "no_significant_latency_improvement"
	RejectHysteresisNotMet              = "hysteresis_not_met"
	RejectNotBestCandidate              = "not_best_candidate"
)

// Recommendation suppression codes (why no recommendation was produced by design).
const (
	SuppressReasonMonitorOnly = "configured_mode_monitor_only"
	// SuppressReasonInvalidMode means the configured mode is not an implemented mode. It is
	// fail-closed to monitor_only semantics and surfaced as a validation error (400) by the
	// boundary, never as a normal suppressed result.
	SuppressReasonInvalidMode = "configured_mode_invalid"
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
	ProfileID         string         `json:"profile_id,omitempty"`
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
	ProfileID         string `json:"profile_id,omitempty"`
	ProfileIDInferred bool   `json:"profile_id_inferred,omitempty"`
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

// RecommendOptions controls how an evidence evaluation is scoped.
type RecommendOptions struct {
	// Preview explicitly requests a "what would the policy say" preview even when the
	// configured mode would suppress recommendations.
	//
	// This exists so the configured mode is NEVER silently bypassed:
	//   - Preview == false (default): the configured mode is respected. Under
	//     ModeMonitorOnly the result is a suppressed stay (no recommendation), exactly
	//     matching the existing "仅收集遥测数据，不执行或推荐切换" semantic.
	//   - Preview == true: the caller explicitly opts into a preview. The evaluation
	//     then runs under ModeRecommend semantics, and the result is hard-marked
	//     Preview == true / AdvisoryOnly == true / Executed == false.
	//
	// Neither value can execute anything.
	Preview bool
}

// MonitorRecommendation is the complete, explainable, non-executing output of PR#7.
type MonitorRecommendation struct {
	GeneratedAt time.Time `json:"generated_at"`

	JobID     string `json:"job_id,omitempty"`
	ProfileID string `json:"profile_id,omitempty"`

	Purpose PolicyPurpose `json:"purpose"`
	// ConfiguredMode is the user's configured orchestrator mode. It is always
	// recorded, and it is always respected unless Preview was explicitly requested.
	ConfiguredMode OrchestratorMode `json:"configured_mode"`
	// EvaluationMode is the mode the evaluation actually ran under:
	//   ModeMonitorOnly — the configured mode was respected and no evaluation ran
	//                     (RecommendationSuppressed == true).
	//   ModeRecommend   — a recommendation evaluation ran. This is either the user's
	//                     own configured mode, or an explicitly requested preview.
	EvaluationMode OrchestratorMode `json:"evaluation_mode"`
	// Preview reports whether this result came from an explicitly requested preview
	// that overrode a mode which would otherwise suppress recommendations.
	Preview bool `json:"preview"`
	// RecommendationSuppressed is true when the configured mode itself forbids
	// producing a recommendation (ModeMonitorOnly without an explicit preview).
	RecommendationSuppressed bool   `json:"recommendation_suppressed"`
	SuppressedReason         string `json:"suppressed_reason,omitempty"`

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
// It is a pure function of (now, policy, state, snapshot, opts): it does not mutate the
// passed DecisionState, does not touch the controller, and cannot switch anything.
//
// The configured mode is always honored. A recommendation is only produced when the
// configured mode allows it (ModeRecommend / ModeAuto) or when the caller explicitly
// requested a preview (RecommendOptions.Preview). Under ModeMonitorOnly without a
// preview the result is an explicit, reasoned suppression — never a silent override.
//
// The stay/switch core decision is produced by the existing DecisionEngine.Evaluate;
// this function only (a) builds the evidence-gated candidate set, and (b) turns the
// engine's verdict into a fully explainable recommendation.
func (e *DecisionEngine) RecommendFromEvidence(
	now time.Time,
	p SwitchPolicy,
	state *DecisionState,
	snap EvidenceSnapshot,
	opts RecommendOptions,
) MonitorRecommendation {
	rec := MonitorRecommendation{
		GeneratedAt:        now,
		JobID:              snap.Source.JobID,
		ProfileID:          snap.Source.ProfileID,
		Purpose:            snap.Purpose,
		ConfiguredMode:     p.Mode,
		EvaluationMode:     ModeMonitorOnly,
		Preview:            opts.Preview,
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
	pEval.Purpose = purpose

	// An unset mode must not fall through to "evaluate": the secure default is
	// ModeMonitorOnly (matching DefaultSwitchPolicy). An UNRECOGNISED mode is not a mode
	// at all — it is a configuration error, and this engine's fail-closed duty is to
	// treat it exactly like monitor_only rather than silently evaluating under
	// ModeRecommend and emitting a switch recommendation for a typo.
	//
	// The caller-facing 400 for this case is produced at the boundaries
	// (NormalizeOrchestratorMode in application.GetMonitorRecommendation /
	// UpdateSwitchPolicy). This branch is the last line of defence for a bad value that
	// somehow reached the policy layer, so it must never be the *only* handling.
	configuredMode := p.Mode
	modeInvalid := false
	if normalized, err := NormalizeOrchestratorMode(configuredMode); err != nil {
		modeInvalid = true
		configuredMode = ModeMonitorOnly
	} else {
		configuredMode = normalized
	}
	rec.ConfiguredMode = configuredMode

	// Mode semantics: respect the configured mode unless a preview was explicitly asked for.
	// An invalid mode can NEVER be overridden by preview: a malformed configuration is not
	// a "what if" question, and answering it would emit a recommendation for a policy the
	// operator never validly expressed.
	suppressRecommendation := (configuredMode == ModeMonitorOnly && !opts.Preview) || modeInvalid
	if modeInvalid {
		rec.RecommendationSuppressed = true
		rec.SuppressedReason = SuppressReasonInvalidMode
		rec.EvaluationMode = ModeMonitorOnly
		rec.addReason(ReasonInvalidMode, SeverityBlocker,
			fmt.Sprintf("配置的编排模式 %q 不是已实现的模式之一（%q / %q / %q），为避免把拼写错误当成有效策略，本次不进行任何候选比较与推荐",
				string(p.Mode), string(ModeMonitorOnly), string(ModeRecommend), string(ModeAuto)),
			nil, map[string]any{
				"configured_mode": string(p.Mode),
				"valid_modes": []string{
					string(ModeMonitorOnly), string(ModeRecommend), string(ModeAuto),
				},
			})
	} else if suppressRecommendation {
		rec.RecommendationSuppressed = true
		rec.SuppressedReason = SuppressReasonMonitorOnly
		rec.EvaluationMode = ModeMonitorOnly
		rec.addReason(ReasonMonitorOnlySuppresses, SeverityInfo,
			"当前编排模式为 monitor_only：按既有语义本模式不产生切换建议，因此不输出推荐。若需查看\"策略会怎么判\"，请显式请求 preview",
			nil, map[string]any{
				"configured_mode": string(configuredMode),
				"preview":         false,
			})
	} else {
		rec.EvaluationMode = ModeRecommend
		pEval.Mode = ModeRecommend
		if opts.Preview {
			// Preview is always explained, whether or not it actually overrode a suppression,
			// so a result flagged preview=true is never left without a reason.
			overrode := configuredMode == ModeMonitorOnly
			message := "preview 模式（显式请求）：仅为预览评估；任何模式下都不会执行切换"
			if overrode {
				message = "preview 模式（显式请求）：已越过 monitor_only 的建议抑制，仅为预览评估；任何模式下都不会执行切换"
			}
			rec.addReason(ReasonPreviewNotice, SeverityWarning, message, nil, map[string]any{
				"configured_mode":      string(configuredMode),
				"preview":              true,
				"overrode_suppression": overrode,
			})
		}
		if configuredMode == ModeAuto {
			rec.addReason(ReasonAutoNotImplemented, SeverityInfo,
				"当前编排模式为 auto：自动执行未在本 PR 实现，本结果仅为建议，不会执行任何切换",
				nil, map[string]any{"configured_mode": string(configuredMode)})
		}
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
	appendProfileIsolationReason(&rec, cur)

	// --- Configured mode suppression (ModeMonitorOnly without an explicit preview). ---
	// Telemetry above is still reported; no candidate is compared and no recommendation
	// is produced, because the configured mode forbids it.
	if suppressRecommendation {
		rec.Decision = DecisionStay
		rec.ConfidenceBasis = ConfidenceBasis{Score: 0, Detail: "monitor_only 抑制推荐，未进入候选比较"}
		if !cur.Sufficiency.Sufficient {
			appendInsufficientEvidenceReasons(&rec, cur)
		}
		return rec
	}

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

	// The policy whitelist is expressed with display names, but the engine matches it against
	// the disambiguated evaluation names. Remap it, otherwise two logical nodes sharing a
	// display name would cause the second one ("DUP#2") to be silently excluded by the
	// whitelist even though it is the better candidate.
	if len(pEval.CandidateNodes) > 0 {
		allowed := make(map[string]struct{}, len(pEval.CandidateNodes))
		for _, name := range pEval.CandidateNodes {
			allowed[strings.TrimSpace(name)] = struct{}{}
		}
		remapped := make([]string, 0, len(pEval.CandidateNodes))
		for i := range snap.Nodes {
			if _, ok := allowed[snap.Nodes[i].DisplayName]; ok {
				remapped = append(remapped, nodeNames[i])
			}
		}
		if len(remapped) > 0 {
			pEval.CandidateNodes = remapped
		}
	}

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
				"profile_id":         node.ProfileID,
				"sample_count":       node.SampleCount,
				"sample_age":         node.SampleAge.String(),
				"observation_window": node.ObservationWindow.String(),
				"evidence_window":    rec.Gate.EvidenceWindow.String(),
				"gate_reasons":       node.Sufficiency.Reasons,
			})
			continue
		}

		// Latency-ranking depth: a candidate may only be chosen on its speed if enough
		// transport-scope latency observations back the P50 it would be chosen for. This is
		// checked here (a candidate property) rather than in per-node sufficiency, so that an
		// incumbent whose probes succeed without a measurable latency is still correctly
		// reported as healthy instead of "insufficient evidence".
		if !node.LatencyRankingEligible(p.MinLatencySampleCount) {
			rec.reject(node, RejectInsufficientLatencySamples,
				fmt.Sprintf("候选 %s 有效的传输层延迟样本数 %d 少于 MinLatencySampleCount=%d，其 P50=%s 不足以作为排序依据（总样本数 %d 不计入，因为延迟仅由传输层成功探测构成）",
					node.DisplayName, node.LatencySampleCount, p.MinLatencySampleCount,
					roundDuration(node.LatencyP50), node.SampleCount),
				map[string]any{
					"latency_sample_count":     node.LatencySampleCount,
					"min_latency_sample_count": p.MinLatencySampleCount,
					"sample_count":             node.SampleCount,
					"latency_p50":              node.LatencyP50.String(),
					"transport_success_count":  node.TransportSuccessCount,
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
				fmt.Sprintf("候选 %s 传输层探测失败 %d/%d（连续 %d 次），判定为节点故障",
					node.DisplayName, node.TransportScopeFailureCount(), node.TransportSampleCount,
					node.ConsecutiveTransportFailures),
				map[string]any{
					"transport_scope_failure_count": node.TransportScopeFailureCount(),
					"transport_sample_count":        node.TransportSampleCount,
					"transport_attributed_failures": node.TransportFailureCount,
					"consecutive_failures":          node.ConsecutiveTransportFailures,
					"error_breakdown":               node.ErrorBreakdown,
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
	//
	// The current node's evidence was sufficient (checked above), so this is not an
	// evidence problem: the node is failed/blocked and the candidate pool is empty after
	// the gate. It gets its own verdict so the payload cannot contradict its own
	// EvidenceSufficiency.
	if !cur.TransportHealthy || cur.BlockedForPurpose {
		rec.Decision = DecisionNoEligibleCandidate
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

	// The reason must state the cause that actually applied. The engine returns
	// ShouldSwitch == false for several distinct reasons, and only one of them is
	// "no candidate beat the thresholds" — the others never compared candidates at all,
	// so asserting that cause there would be a fabricated explanation.
	switch {
	case !stateCopy.LastSwitchAt.IsZero() && now.Sub(stateCopy.LastSwitchAt) < pEval.CooldownDuration:
		remaining := pEval.CooldownDuration - now.Sub(stateCopy.LastSwitchAt)
		rec.addReason(ReasonStayCurrentStable, SeverityInfo,
			fmt.Sprintf("处于切换冷却期中（剩余 %s），本轮未进行候选比较，维持当前选择",
				roundDuration(remaining)), cur,
			map[string]any{
				"cooldown_duration":  pEval.CooldownDuration.String(),
				"cooldown_remaining": remaining.String(),
				"last_switch_at":     stateCopy.LastSwitchAt,
			})
	case cur.LatencyP50 <= 0:
		rec.addReason(ReasonStayCurrentStable, SeverityInfo,
			"当前节点尚无可用的传输层延迟基线（P50=0），引擎不做切换决策，维持当前选择", cur,
			map[string]any{"latency_sample_count": cur.LatencySampleCount})
	default:
		rec.addReason(ReasonStayCurrentStable, SeverityInfo,
			"当前节点传输层健康，且没有候选节点满足证据门槛与防抖阈值，维持当前选择", cur, nil)
	}

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
		ProfileID:         node.ProfileID,
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
		case GateReasonInsufficientLatencySamples:
			return RejectInsufficientLatencySamples, detail
		case GateReasonInsufficientObservationWindow:
			return RejectInsufficientObservationWindow, detail
		case GateReasonEvidenceBudgetExceeded:
			// Must have its own code: reporting a partial read as "no evidence" tells the
			// caller the opposite of what happened (there WAS evidence, we could not read
			// all of it) and hides the one condition a retry/larger budget would fix.
			return RejectEvidenceBudgetExceeded, detail
		case GateReasonOtherRevisionSamplesExcluded:
			return RejectOtherConfigRevision, detail
		case GateReasonUnknownRevisionSamplesExcluded:
			return RejectUnknownConfigRevision, detail
		case GateReasonOtherProfileSamplesExcluded:
			return RejectOtherProfile, detail
		case GateReasonUnknownProfileSamplesExcluded:
			return RejectUnknownProfile, detail
		case GateReasonProfileIsolationAmbiguous:
			return RejectProfileIsolationAmbiguous, detail
		case GateReasonNoEvidence:
			return RejectNoEvidence, detail
		}
	}
	if detail == "" {
		detail = fmt.Sprintf("候选 %s 未通过证据门槛", node.DisplayName)
	}
	return RejectNoEvidence, detail
}

// appendProfileIsolationReason surfaces which ProfileID the evidence was attributed to,
// so a reviewer can verify no cross-profile mixing happened.
func appendProfileIsolationReason(rec *MonitorRecommendation, ev *NodeEvidence) {
	if ev == nil {
		return
	}
	evidence := map[string]any{
		"profile_id":                       ev.ProfileID,
		"profile_id_inferred":              ev.ProfileIDInferred,
		"profile_isolation_unknown":        ev.ProfileIsolationUnknown,
		"node_identity_key":                ev.NodeIdentityKey,
		"config_revision_key":              ev.ConfigRevisionKey,
		"excluded_other_profile_samples":   ev.ExcludedOtherProfileSamples,
		"excluded_unknown_profile_samples": ev.ExcludedUnknownProfileSamples,
		"excluded_other_revision_samples":  ev.ExcludedOtherRevisionSamples,
		"profile_isolation_ambiguous":      ev.ProfileIsolationAmbiguous,
	}
	message := fmt.Sprintf("证据归属：ProfileID=%s", ev.ProfileID)
	if ev.ProfileIDInferred {
		message += "（未显式提供，由观察窗口内唯一样本 Profile 推断）"
	}
	if ev.ProfileIsolationUnknown {
		message += "（未知：观察窗口内没有任何带 Profile 归属的样本，无法证明跨 Profile 隔离）"
	} else if ev.ProfileID == "" {
		message += "（未知）"
	}
	message += fmt.Sprintf("；已排除 %d 条属于其他 Profile、%d 条缺少 Profile 归属的样本",
		ev.ExcludedOtherProfileSamples, ev.ExcludedUnknownProfileSamples)
	if ev.ProfileIsolationAmbiguous {
		message += "；同一传输端点存在多个 Profile，归属不可判定"
	}
	rec.addReason(ReasonProfileIsolation, SeverityInfo, message, ev, evidence)
}

func appendInsufficientEvidenceReasons(rec *MonitorRecommendation, cur *NodeEvidence) {
	rec.addReason(ReasonInsufficientEvidence, SeverityBlocker,
		fmt.Sprintf("当前节点 %s 证据不足，按保守门槛输出 insufficient_evidence，拒绝给出硬推荐", cur.DisplayName),
		cur, map[string]any{
			"gate_reasons":       cur.Sufficiency.Reasons,
			"sample_count":       cur.SampleCount,
			"sample_age":         cur.SampleAge.String(),
			"observation_window": cur.ObservationWindow.String(),
			"evidence_window":    rec.Gate.EvidenceWindow.String(),
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
			fmt.Sprintf("当前节点 %s 最新样本距今 %s，满足新鲜度门槛（MaxSampleAge=%s）；观察窗口为 %s（扫描窗口 %s）",
				cur.DisplayName, roundDuration(cur.SampleAge), roundDuration(rec.Gate.MaxSampleAge),
				roundDuration(cur.ObservationWindow), roundDuration(rec.Gate.EvidenceWindow)),
			cur, map[string]any{
				"freshness":          string(cur.Freshness),
				"sample_age":         cur.SampleAge.String(),
				"last_sample_at":     cur.LastSampleAt,
				"max_sample_age":     rec.Gate.MaxSampleAge.String(),
				"observation_window": cur.ObservationWindow.String(),
				"evidence_window":    rec.Gate.EvidenceWindow.String(),
			})
	case FreshnessStale:
		rec.addReason(ReasonCurrentFreshness, SeverityBlocker,
			fmt.Sprintf("当前节点 %s 最新样本距今 %s，已超过 MaxSampleAge=%s，陈旧数据不得驱动推荐",
				cur.DisplayName, roundDuration(cur.SampleAge), roundDuration(rec.Gate.MaxSampleAge)),
			cur, map[string]any{
				"freshness":          string(cur.Freshness),
				"sample_age":         cur.SampleAge.String(),
				"last_sample_at":     cur.LastSampleAt,
				"max_sample_age":     rec.Gate.MaxSampleAge.String(),
				"observation_window": cur.ObservationWindow.String(),
				"evidence_window":    rec.Gate.EvidenceWindow.String(),
			})
	default:
		rec.addReason(ReasonCurrentFreshness, SeverityBlocker,
			fmt.Sprintf("当前节点 %s 在观察窗口（%s）内没有任何原始样本",
				cur.DisplayName, roundDuration(rec.Gate.EvidenceWindow)),
			cur, map[string]any{
				"freshness":       string(cur.Freshness),
				"evidence_window": rec.Gate.EvidenceWindow.String(),
				"window_since":    rec.Gate.WindowSince,
			})
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
			fmt.Sprintf("当前节点 %s 传输层探测失败 %d/%d，判定为传输层故障",
				cur.DisplayName, cur.TransportScopeFailureCount(), cur.TransportSampleCount),
			cur, map[string]any{
				"transport_scope_failure_count": cur.TransportScopeFailureCount(),
				"transport_sample_count":        cur.TransportSampleCount,
				"transport_attributed_failures": cur.TransportFailureCount,
				"error_breakdown":               cur.ErrorBreakdown,
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

		trigger := res.TriggerType
		if trigger == "" && res.Recommendation != nil {
			trigger = res.Recommendation.TriggerType
		}

		// The threshold claim is only true on the optimisation path. The urgent-failover path
		// does NOT apply MinImprovementRTT / MinImprovementRatio (it takes the lowest-RTT
		// candidate among the fresh, available ones), so the recommended node can even be
		// slower than the current one — claiming the thresholds were met there would be false.
		thresholdsApplied := trigger != "failure_failover"
		message := fmt.Sprintf("候选 %s P50=%s 相对当前节点 P50=%s 改善 %s（%.1f%%），满足 MinImprovementRTT=%s / MinImprovementRatio=%.2f",
			target.DisplayName, roundDuration(target.LatencyP50), roundDuration(cur.LatencyP50),
			roundDuration(delta), ratio*100,
			roundDuration(p.MinImprovementRTT), p.MinImprovementRatio)
		if !thresholdsApplied {
			message = fmt.Sprintf("候选 %s P50=%s 与当前节点 P50=%s 相差 %s：本建议由故障切换触发，引擎按“通过证据门槛的可用候选中 RTT 最低者”选取，不适用 MinImprovementRTT / MinImprovementRatio",
				target.DisplayName, roundDuration(target.LatencyP50), roundDuration(cur.LatencyP50),
				roundDuration(delta))
		}

		rec.addReason(ReasonRecommendedP50, SeverityInfo, message, target, map[string]any{
			"target_p50":            target.LatencyP50.String(),
			"current_p50":           cur.LatencyP50.String(),
			"delta":                 delta.String(),
			"improvement_ratio":     ratio,
			"trigger_type":          trigger,
			"thresholds_applied":    thresholdsApplied,
			"min_improvement_rtt":   p.MinImprovementRTT.String(),
			"min_improvement_ratio": p.MinImprovementRatio,
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
		target := &snap.Nodes[targetIndex]

		delta := cur.LatencyP50 - node.LatencyP50
		ratio := 0.0
		if cur.LatencyP50 > 0 {
			ratio = float64(delta) / float64(cur.LatencyP50)
		}
		// Positive means this candidate is genuinely FASTER than the node that was recommended.
		deltaVsTarget := target.LatencyP50 - node.LatencyP50

		code := RejectNotBestCandidate
		reason := ""

		switch {
		case p.MinImprovementRTT > 0 && delta < p.MinImprovementRTT:
			code = RejectNoSignificantImprovement
			reason = fmt.Sprintf("候选 %s 相对当前节点 P50 改善 %s，低于 MinImprovementRTT=%s，引擎不予采纳",
				node.DisplayName, roundDuration(delta), roundDuration(p.MinImprovementRTT))
		case p.MinImprovementRatio > 0 && ratio < p.MinImprovementRatio:
			code = RejectNoSignificantImprovement
			reason = fmt.Sprintf("候选 %s 相对当前节点 P50 改善比例 %.1f%%，低于 MinImprovementRatio=%.2f，引擎不予采纳",
				node.DisplayName, ratio*100, p.MinImprovementRatio)
		case deltaVsTarget > 0:
			// The candidate really is faster than the recommended node, yet the engine never
			// promoted it. That happens because the engine's running best only advances when
			// the improvement clears HysteresisBuffer, so a candidate that beats the eventual
			// winner by less than the buffer is skipped purely because of scan order.
			//
			// This case MUST NOT fall through to the "not better than the recommended node"
			// wording below: that sentence would state the opposite of the evidence in the
			// same object (the candidate is faster, and the numbers say so).
			code = RejectHysteresisNotMet
			reason = fmt.Sprintf("候选 %s P50=%s 确实优于被推荐的 %s P50=%s（快 %s），但未超过防抖缓冲 HysteresisBuffer=%s，引擎的 running-best 规则因此未改选",
				node.DisplayName, roundDuration(node.LatencyP50), target.DisplayName,
				roundDuration(target.LatencyP50), roundDuration(deltaVsTarget), roundDuration(p.HysteresisBuffer))
		default:
			// Only reachable when the candidate is not faster than the recommended node, so
			// this statement is always true of the numbers it quotes.
			reason = fmt.Sprintf("候选 %s 通过证据门槛但未被选中：P50=%s 未优于被推荐节点的 %s",
				node.DisplayName, roundDuration(node.LatencyP50), roundDuration(target.LatencyP50))
		}

		rec.reject(node, code, reason, map[string]any{
			"latency_p50":          node.LatencyP50.String(),
			"current_p50":          cur.LatencyP50.String(),
			"delta":                delta.String(),
			"improvement_ratio":    ratio,
			"recommended_node":     target.DisplayName,
			"recommended_p50":      target.LatencyP50.String(),
			"delta_vs_recommended": deltaVsTarget.String(),
			"hysteresis_buffer":    p.HysteresisBuffer.String(),
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
		Name:               name,
		Available:          ev.TransportHealthy,
		RTT:                ev.LatencyP50,
		Loss:               0,
		Bandwidth:          0,
		TriageStatus:       ev.TriageStatus,
		SampleCount:        ev.SampleCount,
		FirstSampleTime:    ev.FirstSampleAt,
		LastSampleTime:     ev.LastSampleAt,
		ObservationWindow:  ev.ObservationWindow,
		LatencySampleCount: ev.LatencySampleCount,
	}
}

func refFromEvidence(ev *NodeEvidence) *NodeRecommendationRef {
	if ev == nil {
		return nil
	}
	return &NodeRecommendationRef{
		ProfileID:             ev.ProfileID,
		ProfileIDInferred:     ev.ProfileIDInferred,
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

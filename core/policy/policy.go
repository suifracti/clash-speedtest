package policy

import (
	"fmt"
	"time"
)

// OrchestratorMode defines the operational mode for proxy auto-switching.
type OrchestratorMode string

const (
	// ModeMonitorOnly monitors and probes candidate nodes, but NEVER alters external proxy settings.
	// This is the required secure default.
	ModeMonitorOnly OrchestratorMode = "monitor_only"

	// ModeRecommend evaluates policies and produces switch recommendations with full rationale,
	// but requires explicit user confirmation before executing.
	ModeRecommend OrchestratorMode = "recommend"

	// ModeAuto automatically executes node switching in the external proxy controller
	// according to configured thresholds and anti-flapping rules.
	ModeAuto OrchestratorMode = "auto"
)

// PolicyPurpose defines the intended routing purpose or capability required for candidate nodes.
type PolicyPurpose string

const (
	// PurposeGeneral routes general internet traffic. Google/AI regional restrictions
	// are NOT treated as hard failures, and will not cause auto-switching on an otherwise functional node.
	PurposeGeneral PolicyPurpose = "general"

	// PurposeAI routes AI/LLM specialized traffic (e.g. OpenAI, Google Gemini, Anthropic).
	// Confirmed regional blocking (e.g. HTTP 400 FAILED_PRECONDITION / Geo-Blocked) is treated
	// as a hard failure, triggering failover or rollback to an AI-capable node.
	PurposeAI PolicyPurpose = "ai"
)

// SwitchPolicy defines user-configured criteria for automatic proxy node switching.
type SwitchPolicy struct {
	// Purpose defines the target routing domain / required capability ("general" or "ai").
	// Defaults to "general". Regional Google/AI blocks are ONLY hard failures if Purpose == "ai".
	Purpose PolicyPurpose `json:"purpose"`

	// Mode specifies the operational mode: "monitor_only" (default), "recommend", or "auto".
	Mode OrchestratorMode `json:"mode"`

	// TargetGroup specifies the proxy selector group in the external core (e.g. "PROXY", "节点选择").
	TargetGroup string `json:"target_group"`

	// TargetGroupType enforces that only Selector groups can be modified. Defaults to "Selector".
	TargetGroupType string `json:"target_group_type"`

	// CandidateNodes is an optional whitelist of node names to select from.
	// If empty, all available nodes in the target group are considered.
	CandidateNodes []string `json:"candidate_nodes,omitempty"`

	// Interval is the periodic evaluation frequency (e.g. 30s, 1m, 5m).
	Interval time.Duration `json:"interval"`

	// MaxConsecutiveFailures triggers an immediate failover when the current active node
	// fails consecutive health/probe checks (e.g. 3).
	MaxConsecutiveFailures int `json:"max_consecutive_failures"`

	// MinImprovementRTT requires an alternate node to be at least this much faster than the
	// current node to trigger a switch (e.g. 30ms), preventing thrashing.
	MinImprovementRTT time.Duration `json:"min_improvement_rtt"`

	// MinImprovementRatio requires an alternate node to have a relative RTT improvement
	// (e.g. 0.20 = 20% lower latency) to justify switching.
	MinImprovementRatio float64 `json:"min_improvement_ratio"`

	// CooldownDuration is the minimum quiet time after a switch before another non-critical
	// switch can occur (e.g. 5m).
	CooldownDuration time.Duration `json:"cooldown_duration"`

	// HysteresisBuffer provides anti-flapping dampening when comparing fluctuating RTTs.
	HysteresisBuffer time.Duration `json:"hysteresis_buffer"`

	// LockedNode, when non-empty, pins the proxy to this specific node.
	// Auto-switching is completely suspended while locked.
	LockedNode string `json:"locked_node,omitempty"`

	// RollbackOnFailure indicates whether to revert to the previous working node
	// if the newly selected node fails verification after a grace period.
	RollbackOnFailure bool `json:"rollback_on_failure"`

	// --- Sample Freshness & Evidence Thresholds ---

	// MaxSampleAge specifies the maximum allowed age of probe samples.
	// Stale candidate data older than this requires a fresh probe before entering decision.
	MaxSampleAge time.Duration `json:"max_sample_age"`

	// MinSampleCount is the minimum number of distinct probe samples required
	// before a candidate node is eligible for selection.
	MinSampleCount int `json:"min_sample_count"`

	// MinObservationWindow is the minimum timespan between first and latest sample.
	MinObservationWindow time.Duration `json:"min_observation_window"`

	// --- Post-Switch Verification ---

	// VerificationGracePeriod is the settling delay after switching before running verification probes.
	VerificationGracePeriod time.Duration `json:"verification_grace_period"`

	// VerificationProbeCount is the number of lightweight probes run to verify a newly selected node.
	VerificationProbeCount int `json:"verification_probe_count"`

	// VerificationFailureThreshold is the number of failed verification probes (e.g. 2 out of 3)
	// required to trigger a rollback. A single transient failure does NOT trigger rollback.
	VerificationFailureThreshold int `json:"verification_failure_threshold"`
}

// DefaultSwitchPolicy returns sensible, secure production defaults (Monitor Only by default).
func DefaultSwitchPolicy() SwitchPolicy {
	return SwitchPolicy{
		Purpose:                      PurposeGeneral,
		Mode:                         ModeMonitorOnly,
		TargetGroup:                  "PROXY",
		TargetGroupType:              "Selector",
		CandidateNodes:               nil,
		Interval:                     1 * time.Minute,
		MaxConsecutiveFailures:       3,
		MinImprovementRTT:            30 * time.Millisecond,
		MinImprovementRatio:          0.20,
		CooldownDuration:             5 * time.Minute,
		HysteresisBuffer:             15 * time.Millisecond,
		LockedNode:                   "",
		RollbackOnFailure:            true,
		MaxSampleAge:                 5 * time.Minute,
		MinSampleCount:               3,
		MinObservationWindow:         1 * time.Minute,
		VerificationGracePeriod:      10 * time.Second,
		VerificationProbeCount:       3,
		VerificationFailureThreshold: 2,
	}
}

// NodeEvaluation holds latest probe metrics and evidence provenance for a candidate node.
type NodeEvaluation struct {
	Name              string        `json:"name"`
	Available         bool          `json:"available"`
	RTT               time.Duration `json:"rtt"`
	Loss              float64       `json:"loss"`
	Bandwidth         float64       `json:"bandwidth"` // MB/s
	TriageStatus      string        `json:"triage_status"`
	SampleCount       int           `json:"sample_count"`
	FirstSampleTime   time.Time     `json:"first_sample_time"`
	LastSampleTime    time.Time     `json:"last_sample_time"`
	ObservationWindow time.Duration `json:"observation_window"`
}

// SwitchRecommendation represents a proposed switch in ModeRecommend.
type SwitchRecommendation struct {
	TargetNode     string        `json:"target_node"`
	Reason         string        `json:"reason"`
	CurrentRTT     time.Duration `json:"current_rtt"`
	TargetRTT      time.Duration `json:"target_rtt"`
	ImprovementPct int           `json:"improvement_pct"`
	TriggerType    string        `json:"trigger_type"`
}

// DecisionState tracks runtime state, switch cooldowns, failure counters, and audit trail.
type DecisionState struct {
	CurrentNode         string        `json:"current_node"`
	PreviousNode        string        `json:"previous_node,omitempty"`
	SelectedAt          time.Time     `json:"selected_at"`
	LastSwitchAt        time.Time     `json:"last_switch_at"`
	LastSwitchReason    string        `json:"last_switch_reason,omitempty"`
	ConsecutiveFailures int           `json:"consecutive_failures"`
	AuditTrail          []SwitchEvent `json:"audit_trail,omitempty"`
}

// SwitchEvent records an immutable audit log entry for a node switch.
type SwitchEvent struct {
	ID          string        `json:"id"`
	Timestamp   time.Time     `json:"timestamp"`
	TargetGroup string        `json:"target_group"`
	FromNode    string        `json:"from_node"`
	ToNode      string        `json:"to_node"`
	Reason      string        `json:"reason"`
	TriggerType string        `json:"trigger_type"` // "failure_failover", "latency_improvement", "manual_override", "rollback"
	Status      string        `json:"status"`       // "success", "rolled_back", "failed"
	OldMetric   time.Duration `json:"old_metric,omitempty"`
	NewMetric   time.Duration `json:"new_metric,omitempty"`
}

// DecisionResult represents the outcome of a policy evaluation.
type DecisionResult struct {
	Mode                OrchestratorMode      `json:"mode"`
	ShouldSwitch        bool                  `json:"should_switch"`
	TargetNode          string                `json:"target_node,omitempty"`
	Reason              string                `json:"reason,omitempty"`
	TriggerType         string                `json:"trigger_type,omitempty"`
	RequiresFreshProbe  bool                  `json:"requires_fresh_probe"`
	StaleNodes          []string              `json:"stale_nodes,omitempty"`
	Recommendation      *SwitchRecommendation `json:"recommendation,omitempty"`
}

// VerificationProbe represents one probe sample during post-switch verification.
type VerificationProbe struct {
	Index   int           `json:"index"`
	Success bool          `json:"success"`
	RTT     time.Duration `json:"rtt"`
	Error   string        `json:"error,omitempty"`
}

// VerificationResult contains the outcome of post-switch verification.
type VerificationResult struct {
	ShouldRollback bool   `json:"should_rollback"`
	Reason         string `json:"reason"`
	SuccessCount   int    `json:"success_count"`
	FailureCount   int    `json:"failure_count"`
	TotalCount     int    `json:"total_count"`
}

// DecisionEngine implements hysteresis-damped evaluation and sample evidence verification.
type DecisionEngine struct{}

// NewDecisionEngine creates an instance of the DecisionEngine.
func NewDecisionEngine() *DecisionEngine {
	return &DecisionEngine{}
}

// Evaluate analyzes current node health and candidate nodes against the policy.
func (e *DecisionEngine) Evaluate(
	now time.Time,
	p SwitchPolicy,
	state *DecisionState,
	evaluations []NodeEvaluation,
) DecisionResult {
	if state == nil {
		return DecisionResult{
			Mode:         p.Mode,
			ShouldSwitch: false,
			Reason:       "uninitialized decision state",
		}
	}

	// 1. Check Orchestrator Mode: Monitor Only mode NEVER performs or recommends switches.
	// This is the highest-priority guard.
	if p.Mode == ModeMonitorOnly {
		return DecisionResult{
			Mode:         p.Mode,
			ShouldSwitch: false,
			Reason:       "处于仅监测模式 (Monitor Only)，仅收集遥测数据，不执行或推荐切换",
		}
	}

	// 2. Check if manually locked to a node.
	// LockedNode semantics: pins user's explicit current selection, completely pausing automatic switching.
	// It does NOT background-force switches to LockedNode (node selection is an independent explicit user action).
	if p.LockedNode != "" {
		return DecisionResult{
			Mode:         p.Mode,
			ShouldSwitch: false,
			Reason:       fmt.Sprintf("已锁定节点 %s，暂停所有自动切换调度", p.LockedNode),
		}
	}

	// Index candidate evaluations
	evalMap := make(map[string]NodeEvaluation, len(evaluations))
	for _, ev := range evaluations {
		evalMap[ev.Name] = ev
	}

	// Whitelist filter
	candidateSet := make(map[string]struct{}, len(p.CandidateNodes))
	for _, c := range p.CandidateNodes {
		candidateSet[c] = struct{}{}
	}
	isEligible := func(name string) bool {
		if len(candidateSet) == 0 {
			return true
		}
		_, ok := candidateSet[name]
		return ok
	}

	currentEv, hasCurrent := evalMap[state.CurrentNode]

	// 3. Urgent Failover Check: current node has failed >= MaxConsecutiveFailures
	// or current node is explicitly confirmed unavailable / failed.
	// NOTE: Google/AI regional restrictions (TriageStatus == "blocked") are ONLY treated
	// as hard failures when the policy explicitly requires AI capability (Purpose == PurposeAI).
	// For general Selector policies, regional AI blocking does NOT cause unprovoked automatic failover.
	currentUnhealthy := false
	if state.ConsecutiveFailures >= p.MaxConsecutiveFailures && p.MaxConsecutiveFailures > 0 {
		currentUnhealthy = true
	} else if hasCurrent {
		if !currentEv.Available || currentEv.TriageStatus == "failed" {
			currentUnhealthy = true
		} else if currentEv.TriageStatus == "blocked" {
			if p.Purpose == PurposeAI {
				currentUnhealthy = true
			}
		}
	}

	// Sample Freshness & Evidence Filter for candidate nodes
	var staleNodes []string
	isFreshCandidate := func(ev NodeEvaluation) bool {
		if !ev.LastSampleTime.IsZero() && p.MaxSampleAge > 0 && now.Sub(ev.LastSampleTime) > p.MaxSampleAge {
			staleNodes = append(staleNodes, ev.Name)
			return false
		}
		if p.MinSampleCount > 0 && ev.SampleCount < p.MinSampleCount {
			staleNodes = append(staleNodes, ev.Name)
			return false
		}
		if p.MinObservationWindow > 0 && ev.ObservationWindow < p.MinObservationWindow {
			staleNodes = append(staleNodes, ev.Name)
			return false
		}
		return true
	}

	if currentUnhealthy {
		// Urgent failover: pick best available node that is verified and fresh
		bestNode := ""
		var bestRTT time.Duration = 1<<63 - 1

		for _, ev := range evaluations {
			if ev.Name == state.CurrentNode || !isEligible(ev.Name) {
				continue
			}
			if !isFreshCandidate(ev) {
				continue
			}
			// In AI purpose mode, candidates blocked on AI/Google cannot be chosen
			if p.Purpose == PurposeAI && ev.TriageStatus == "blocked" {
				continue
			}
			if ev.Available && ev.RTT > 0 && ev.RTT < bestRTT {
				bestRTT = ev.RTT
				bestNode = ev.Name
			}
		}

		if bestNode != "" {
			reason := fmt.Sprintf("当前节点 %s 持续故障（连续失败 %d 次），紧急切换至可用优质节点 %s (RTT: %v)",
				state.CurrentNode, state.ConsecutiveFailures, bestNode, bestRTT)

			if p.Mode == ModeRecommend {
				return DecisionResult{
					Mode:               p.Mode,
					ShouldSwitch:       false,
					Reason:             "生成故障切换建议",
					RequiresFreshProbe: len(staleNodes) > 0,
					StaleNodes:         staleNodes,
					Recommendation: &SwitchRecommendation{
						TargetNode:  bestNode,
						Reason:      reason,
						CurrentRTT:  0,
						TargetRTT:   bestRTT,
						TriggerType: "failure_failover",
					},
				}
			}

			return DecisionResult{
				Mode:               p.Mode,
				ShouldSwitch:       true,
				TargetNode:         bestNode,
				Reason:             reason,
				TriggerType:        "failure_failover",
				RequiresFreshProbe: len(staleNodes) > 0,
				StaleNodes:         staleNodes,
			}
		}

		return DecisionResult{
			Mode:               p.Mode,
			ShouldSwitch:       false,
			Reason:             fmt.Sprintf("当前节点 %s 故障，但候选池中无满足证据门槛的可用替代节点", state.CurrentNode),
			RequiresFreshProbe: len(staleNodes) > 0,
			StaleNodes:         staleNodes,
		}
	}

	// 4. Cooldown Check for non-emergency optimization
	if !state.LastSwitchAt.IsZero() && now.Sub(state.LastSwitchAt) < p.CooldownDuration {
		remaining := p.CooldownDuration - now.Sub(state.LastSwitchAt)
		return DecisionResult{
			Mode:         p.Mode,
			ShouldSwitch: false,
			Reason:       fmt.Sprintf("处于切换冷却期中，剩余 %v", remaining.Round(time.Second)),
		}
	}

	// 5. Latency Improvement Evaluation
	if !hasCurrent || !currentEv.Available || currentEv.RTT <= 0 {
		return DecisionResult{
			Mode:         p.Mode,
			ShouldSwitch: false,
			Reason:       "当前节点尚未测得有效基准延迟，暂不决策",
		}
	}

	bestCandidate := ""
	bestCandidateRTT := currentEv.RTT

	for _, ev := range evaluations {
		if ev.Name == state.CurrentNode || !isEligible(ev.Name) || !ev.Available || ev.RTT <= 0 {
			continue
		}
		if !isFreshCandidate(ev) {
			continue
		}
		// In AI purpose mode, candidates blocked on AI/Google cannot be chosen
		if p.Purpose == PurposeAI && ev.TriageStatus == "blocked" {
			continue
		}

		diff := currentEv.RTT - ev.RTT
		if diff < p.MinImprovementRTT {
			continue
		}
		ratio := float64(diff) / float64(currentEv.RTT)
		if ratio < p.MinImprovementRatio {
			continue
		}
		if ev.RTT < bestCandidateRTT-p.HysteresisBuffer {
			bestCandidateRTT = ev.RTT
			bestCandidate = ev.Name
		}
	}

	if bestCandidate != "" {
		improvementMs := (currentEv.RTT - bestCandidateRTT).Milliseconds()
		ratioPct := int(float64(currentEv.RTT-bestCandidateRTT) / float64(currentEv.RTT) * 100)
		reason := fmt.Sprintf("候选节点 %s 显著优于当前节点（延迟低 %dms / 优化 %d%%，满足证据门槛与防抖阈值）",
			bestCandidate, improvementMs, ratioPct)

		if p.Mode == ModeRecommend {
			return DecisionResult{
				Mode:               p.Mode,
				ShouldSwitch:       false,
				Reason:             "生成优化切换建议",
				RequiresFreshProbe: len(staleNodes) > 0,
				StaleNodes:         staleNodes,
				Recommendation: &SwitchRecommendation{
					TargetNode:     bestCandidate,
					Reason:         reason,
					CurrentRTT:     currentEv.RTT,
					TargetRTT:      bestCandidateRTT,
					ImprovementPct: ratioPct,
					TriggerType:    "latency_improvement",
				},
			}
		}

		return DecisionResult{
			Mode:               p.Mode,
			ShouldSwitch:       true,
			TargetNode:         bestCandidate,
			Reason:             reason,
			TriggerType:        "latency_improvement",
			RequiresFreshProbe: len(staleNodes) > 0,
			StaleNodes:         staleNodes,
		}
	}

	return DecisionResult{
		Mode:               p.Mode,
		ShouldSwitch:       false,
		Reason:             "当前节点表现稳定，无可显著改善的替代节点",
		RequiresFreshProbe: len(staleNodes) > 0,
		StaleNodes:         staleNodes,
	}
}

// EvaluateVerification checks post-switch multi-probe results to determine whether rollback is warranted.
// Rollback is executed ONLY if majority fail (failures >= threshold) or there is confirmed region blocking under AI purpose.
// Regional blocking is NOT treated as a hard failure under PurposeGeneral.
func (e *DecisionEngine) EvaluateVerification(
	probes []VerificationProbe,
	explicitBlocked bool,
	threshold int,
	purpose ...PolicyPurpose,
) VerificationResult {
	pPurpose := PurposeGeneral
	if len(purpose) > 0 && purpose[0] != "" {
		pPurpose = purpose[0]
	}

	if explicitBlocked {
		if pPurpose == PurposeAI {
			return VerificationResult{
				ShouldRollback: true,
				Reason:         "新节点在 AI 策略下明确捕获地区阻断证据 (HTTP 400 FAILED_PRECONDITION)，立即执行回退",
				TotalCount:     len(probes),
			}
		}
		// Under PurposeGeneral, regional AI blocking alone is not a hard failure for general traffic
	}

	successCount := 0
	failureCount := 0
	for _, p := range probes {
		if p.Success {
			successCount++
		} else {
			failureCount++
		}
	}

	if threshold <= 0 {
		threshold = 2
	}

	if failureCount >= threshold {
		return VerificationResult{
			ShouldRollback: true,
			Reason: fmt.Sprintf("切换后轻量复测多数失败 (%d/%d 失败，达到阈值 %d)，确认节点不稳定，触发防抖回退",
				failureCount, len(probes), threshold),
			SuccessCount: successCount,
			FailureCount: failureCount,
			TotalCount:   len(probes),
		}
	}

	return VerificationResult{
		ShouldRollback: false,
		Reason: fmt.Sprintf("新节点复测通过 (%d/%d 成功)，新路线验证完成",
			successCount, len(probes)),
		SuccessCount: successCount,
		FailureCount: failureCount,
		TotalCount:   len(probes),
	}
}

// RecordSwitch updates state when a switch succeeds.
func (e *DecisionEngine) RecordSwitch(
	state *DecisionState,
	from string,
	to string,
	group string,
	reason string,
	trigger string,
	oldRTT time.Duration,
	newRTT time.Duration,
) {
	now := time.Now()
	event := SwitchEvent{
		ID:          fmt.Sprintf("sw-%d", now.UnixNano()),
		Timestamp:   now,
		TargetGroup: group,
		FromNode:    from,
		ToNode:      to,
		Reason:      reason,
		TriggerType: trigger,
		Status:      "success",
		OldMetric:   oldRTT,
		NewMetric:   newRTT,
	}

	state.PreviousNode = from
	state.CurrentNode = to
	state.SelectedAt = now
	state.LastSwitchAt = now
	state.LastSwitchReason = reason
	state.ConsecutiveFailures = 0
	state.AuditTrail = append(state.AuditTrail, event)

	if len(state.AuditTrail) > 100 {
		state.AuditTrail = state.AuditTrail[len(state.AuditTrail)-100:]
	}
}

// RecordRollback updates state when a switch fails verification and rolls back.
func (e *DecisionEngine) RecordRollback(
	state *DecisionState,
	group string,
	failedNode string,
	reason string,
) {
	if state.PreviousNode == "" {
		return
	}
	now := time.Now()
	event := SwitchEvent{
		ID:          fmt.Sprintf("rb-%d", now.UnixNano()),
		Timestamp:   now,
		TargetGroup: group,
		FromNode:    failedNode,
		ToNode:      state.PreviousNode,
		Reason:      fmt.Sprintf("新节点验证失败回退: %s", reason),
		TriggerType: "rollback",
		Status:      "rolled_back",
	}

	state.CurrentNode = state.PreviousNode
	state.PreviousNode = ""
	state.SelectedAt = now
	state.LastSwitchAt = now
	state.LastSwitchReason = event.Reason
	state.ConsecutiveFailures = 0
	state.AuditTrail = append(state.AuditTrail, event)
}

// RecordFailure increments the failure count for the current node.
func (e *DecisionEngine) RecordFailure(state *DecisionState) {
	if state != nil {
		state.ConsecutiveFailures++
	}
}

// RecordSuccess resets the failure count for the current node.
func (e *DecisionEngine) RecordSuccess(state *DecisionState) {
	if state != nil {
		state.ConsecutiveFailures = 0
	}
}

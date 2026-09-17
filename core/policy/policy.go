package policy

import (
	"fmt"
	"time"
)

// SwitchPolicy defines user-configured criteria for automatic proxy node switching.
type SwitchPolicy struct {
	// AutoSwitchEnabled toggles automatic switching on/off.
	AutoSwitchEnabled bool `json:"auto_switch_enabled"`

	// TargetGroup specifies the proxy selector group in the external core (e.g. "PROXY", "节点选择").
	TargetGroup string `json:"target_group"`

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

	// RollbackOnFailure indicates whether to immediately revert to the previous working node
	// if the newly selected node fails initial verification.
	RollbackOnFailure bool `json:"rollback_on_failure"`
}

// DefaultSwitchPolicy returns sensible production defaults.
func DefaultSwitchPolicy() SwitchPolicy {
	return SwitchPolicy{
		AutoSwitchEnabled:      false,
		TargetGroup:            "PROXY",
		CandidateNodes:         nil,
		Interval:               1 * time.Minute,
		MaxConsecutiveFailures: 3,
		MinImprovementRTT:      30 * time.Millisecond,
		MinImprovementRatio:    0.20,
		CooldownDuration:       5 * time.Minute,
		HysteresisBuffer:       15 * time.Millisecond,
		LockedNode:             "",
		RollbackOnFailure:      true,
	}
}

// NodeEvaluation holds latest probe metrics for a node under evaluation.
type NodeEvaluation struct {
	Name         string        `json:"name"`
	Available    bool          `json:"available"`
	RTT          time.Duration `json:"rtt"`
	Loss         float64       `json:"loss"`
	Bandwidth    float64       `json:"bandwidth"` // MB/s
	TriageStatus string        `json:"triage_status"`
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
	ShouldSwitch bool   `json:"should_switch"`
	TargetNode   string `json:"target_node,omitempty"`
	Reason       string `json:"reason,omitempty"`
	TriggerType  string `json:"trigger_type,omitempty"`
}

// DecisionEngine implements hysteresis-damped evaluation logic.
type DecisionEngine struct{}

// NewDecisionEngine creates an instance of the DecisionEngine.
func NewDecisionEngine() *DecisionEngine {
	return &DecisionEngine{}
}

// Evaluate analyzes the current node's health and candidate nodes against the policy.
func (e *DecisionEngine) Evaluate(
	now time.Time,
	p SwitchPolicy,
	state *DecisionState,
	evaluations []NodeEvaluation,
) DecisionResult {
	if state == nil {
		return DecisionResult{ShouldSwitch: false, Reason: "uninitialized decision state"}
	}

	// 1. Check if manually locked to a node
	if p.LockedNode != "" {
		if state.CurrentNode != p.LockedNode {
			return DecisionResult{
				ShouldSwitch: true,
				TargetNode:   p.LockedNode,
				Reason:       fmt.Sprintf("手动锁定节点至 %s", p.LockedNode),
				TriggerType:  "manual_override",
			}
		}
		return DecisionResult{ShouldSwitch: false, Reason: fmt.Sprintf("已锁定至节点 %s，暂停自动切换", p.LockedNode)}
	}

	// 2. Check if auto switch is enabled
	if !p.AutoSwitchEnabled {
		return DecisionResult{ShouldSwitch: false, Reason: "自动切换未启用"}
	}

	// Index candidate evaluations
	evalMap := make(map[string]NodeEvaluation, len(evaluations))
	for _, ev := range evaluations {
		evalMap[ev.Name] = ev
	}

	// Whitelist filter map
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
	// or current node is explicitly confirmed unavailable / geo-blocked
	currentUnhealthy := false
	if state.ConsecutiveFailures >= p.MaxConsecutiveFailures && p.MaxConsecutiveFailures > 0 {
		currentUnhealthy = true
	} else if hasCurrent && (!currentEv.Available || currentEv.TriageStatus == "blocked" || currentEv.TriageStatus == "failed") {
		// If current node is explicitly unavailable
		currentUnhealthy = true
	}

	if currentUnhealthy {
		// Urgent failover ignores cooldown
		bestNode := ""
		var bestRTT time.Duration = 1<<63 - 1

		for _, ev := range evaluations {
			if ev.Name == state.CurrentNode || !isEligible(ev.Name) {
				continue
			}
			// Only consider verified available nodes
			if ev.Available && ev.RTT > 0 && ev.RTT < bestRTT {
				bestRTT = ev.RTT
				bestNode = ev.Name
			}
		}

		if bestNode != "" {
			return DecisionResult{
				ShouldSwitch: true,
				TargetNode:   bestNode,
				Reason: fmt.Sprintf("当前节点 %s 持续故障（连续失败 %d 次），紧急切换至可用节点 %s (RTT: %v)",
					state.CurrentNode, state.ConsecutiveFailures, bestNode, bestRTT),
				TriggerType: "failure_failover",
			}
		}

		return DecisionResult{
			ShouldSwitch: false,
			Reason:       fmt.Sprintf("当前节点 %s 故障，但候选池中无可用替代节点", state.CurrentNode),
		}
	}

	// 4. Cooldown & Hysteresis Check for non-emergency optimization
	if !state.LastSwitchAt.IsZero() && now.Sub(state.LastSwitchAt) < p.CooldownDuration {
		remaining := p.CooldownDuration - now.Sub(state.LastSwitchAt)
		return DecisionResult{
			ShouldSwitch: false,
			Reason:       fmt.Sprintf("处于切换冷却期中，剩余 %v", remaining.Round(time.Second)),
		}
	}

	// 5. Latency Improvement Evaluation
	if !hasCurrent || !currentEv.Available || currentEv.RTT <= 0 {
		return DecisionResult{ShouldSwitch: false, Reason: "当前节点尚未测得有效基准延迟，暂不决策"}
	}

	bestCandidate := ""
	bestCandidateRTT := currentEv.RTT

	for _, ev := range evaluations {
		if ev.Name == state.CurrentNode || !isEligible(ev.Name) || !ev.Available || ev.RTT <= 0 {
			continue
		}

		// Calculate difference
		diff := currentEv.RTT - ev.RTT

		// Check absolute improvement threshold
		if diff < p.MinImprovementRTT {
			continue
		}

		// Check relative improvement ratio
		ratio := float64(diff) / float64(currentEv.RTT)
		if ratio < p.MinImprovementRatio {
			continue
		}

		// Check hysteresis buffer against best candidate
		if ev.RTT < bestCandidateRTT-p.HysteresisBuffer {
			bestCandidateRTT = ev.RTT
			bestCandidate = ev.Name
		}
	}

	if bestCandidate != "" {
		improvementMs := (currentEv.RTT - bestCandidateRTT).Milliseconds()
		ratioPct := int(float64(currentEv.RTT-bestCandidateRTT) / float64(currentEv.RTT) * 100)
		return DecisionResult{
			ShouldSwitch: true,
			TargetNode:   bestCandidate,
			Reason: fmt.Sprintf("候选节点 %s 显著优于当前节点（延迟低 %dms / 优化 %d%%，满足防抖阈值）",
				bestCandidate, improvementMs, ratioPct),
			TriggerType: "latency_improvement",
		}
	}

	return DecisionResult{ShouldSwitch: false, Reason: "当前节点表现稳定，无可显著改善的替代节点"}
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

	// Keep last 100 audit events in memory
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

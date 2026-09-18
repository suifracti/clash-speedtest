package policy

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// =============================================================================
// PR#7 — Monitor Evidence → Recommendation (advisory, strictly non-executing)
//
// Pipeline implemented here:
//
//	Raw Samples / Derived Stats
//	     ↓   (application adapter projects persisted monitor samples → EvidenceSample)
//	Evidence Snapshot              (BuildEvidenceSnapshot)
//	     ↓
//	existing core/policy engine    (DecisionEngine.Evaluate — reused verbatim)
//	     ↓
//	Recommendation + Reasons       (DecisionEngine.RecommendFromEvidence)
//
// Hard invariants (enforced by construction, asserted by tests):
//   - This path NEVER calls controller.SelectNode and never mutates DecisionState.
//   - Every function in this file is a pure function of its arguments.
//   - No switch, no rollback, no cooldown execution, no controller write.
//   - ModeAuto execution remains unimplemented here (AutoImplemented == false).
//
// This file does NOT introduce a second policy model: candidate eligibility,
// purpose semantics, freshness thresholds and the stay/switch decision are all
// delegated to the existing SwitchPolicy / DecisionEngine.Evaluate.
//
// -----------------------------------------------------------------------------
// Two concepts that must stay separate (external review finding, PR#7)
// -----------------------------------------------------------------------------
//
//	MaxSampleAge       = a FRESHNESS gate on the newest sample only.
//	                     Existing usage: now - LastSampleTime > MaxSampleAge → stale.
//	                     It must NEVER be used to define how much history is scanned,
//	                     otherwise a node with a brand-new sample but a long, otherwise
//	                     sufficient history would be falsely gated as insufficient.
//	EvidenceWindow     = how much history is scanned. Resolved independently of
//	                     MaxSampleAge (see DefaultEvidenceWindow / EvidenceInput.EvidenceWindow).
//	ObservationWindow  = the ACTUAL span between the first and last considered sample.
//	                     This is the value MinObservationWindow gates, matching the
//	                     existing NodeEvaluation.ObservationWindow semantics.
//
// -----------------------------------------------------------------------------
// Isolation: samples must be attributable to exactly one logical node
// -----------------------------------------------------------------------------
//
// A persisted sample is only usable as evidence when it can be attributed to the
// node under evaluation. Three independent keys must agree:
//
//	ProfileID         — the logical subscription/profile. NodeIdentityKey is a pure
//	                    transport endpoint and is explicitly NOT unique per logical
//	                    node (monitor/fingerprint.go R-01): two profiles may share the
//	                    same CDN host / reverse-proxy endpoint and therefore the same
//	                    NodeIdentityKey. Qualifying by ProfileID is mandatory to avoid
//	                    cross-profile aggregation.
//	NodeIdentityKey   — the physical transport endpoint (what we query by, on purpose,
//	                    so revision changes do not orphan history).
//	ConfigRevisionKey — the config/credential revision. Evidence recorded under a
//	                    previous revision must not be mixed into the current one.
//
// Samples failing any of these attributions are excluded and counted, never averaged in.
// =============================================================================

// -----------------------------------------------------------------------------
// Probe scope & failure classification (purpose semantics)
// -----------------------------------------------------------------------------

// ProbeScope classifies a raw probe by what a failure of that probe actually proves.
type ProbeScope string

const (
	// ScopeTransport probes the raw proxy transport path against a neutral, globally
	// reachable endpoint. A failure here is evidence that the node itself is broken.
	ScopeTransport ProbeScope = "transport"

	// ScopeService probes a specific third-party service (Google, GitHub, ...).
	// A regional/service rejection here does NOT by itself prove the node transport
	// is broken — see PolicyPurpose for how it is weighted.
	ScopeService ProbeScope = "service"

	// ScopeAI probes an AI/LLM specific endpoint. Rejections here are the canonical
	// AI-region-block signal.
	ScopeAI ProbeScope = "ai"
)

// ClassifyProbeScope maps a monitor probe_type onto its evidence scope.
//
//	"rtt", "ttfb", "ttfb_heavy", "init", "exit_ip", "http_status" → transport
//	"service_*" (e.g. service_google, service_github)               → service
//	"ai_*", "ai", "antigravity*"                                    → ai
func ClassifyProbeScope(probeType string) ProbeScope {
	p := strings.ToLower(strings.TrimSpace(probeType))
	switch {
	case p == "":
		return ScopeTransport
	case strings.HasPrefix(p, "service_"):
		return ScopeService
	case strings.HasPrefix(p, "ai_"), p == "ai", strings.HasPrefix(p, "antigravity"):
		return ScopeAI
	default:
		return ScopeTransport
	}
}

// FailureKind is the purpose-aware meaning of a single failed sample.
type FailureKind string

const (
	FailureNone      FailureKind = "none"
	FailureTransport FailureKind = "transport_failure"
	FailureService   FailureKind = "service_failure"
	FailureAIBlock   FailureKind = "ai_block"
)

// regionRejectionStatusMarkers are the HTTP status codes that represent an explicit
// regional / capability rejection (e.g. 400 FAILED_PRECONDITION, 403, 451).
var regionRejectionStatusMarkers = []string{"400", "403", "451"}

// isRegionRejection reports whether an error looks like an explicit regional /
// service-level rejection rather than a broken transport path.
func isRegionRejection(errorClass, errorDetail string, regionBlocked bool) bool {
	if regionBlocked {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(errorClass)) {
	case "blocked", "geo_blocked", "region_blocked", "geo_restricted", "region_restricted":
		return true
	case "http_status_error":
		d := strings.ToLower(errorDetail)
		for _, marker := range regionRejectionStatusMarkers {
			if strings.Contains(d, marker) {
				return true
			}
		}
	}
	return false
}

// ClassifyFailure determines what a single failed sample proves about a node.
//
// Purpose-neutral by design: the returned kind is a fact about the probe, and the
// purpose (general vs ai) decides how it is weighted downstream. This is what keeps
// "Google blocked" from being silently promoted to "the whole node is broken".
func ClassifyFailure(probeType, errorClass, errorDetail string, regionBlocked bool) FailureKind {
	scope := ClassifyProbeScope(probeType)
	region := isRegionRejection(errorClass, errorDetail, regionBlocked)

	switch scope {
	case ScopeAI:
		if region {
			return FailureAIBlock
		}
		// A genuine timeout/TLS/DNS failure against an AI endpoint is still transport.
		return FailureTransport
	case ScopeService:
		if region {
			return FailureService
		}
		return FailureTransport
	default:
		return FailureTransport
	}
}

// -----------------------------------------------------------------------------
// Evidence input (neutral projection of persisted raw samples)
// -----------------------------------------------------------------------------

// EvidenceSample is a neutral projection of one persisted monitor sample.
// It carries no persistence concerns so the evidence builder stays pure and unit-testable.
type EvidenceSample struct {
	ProfileID         string
	NodeKey           string
	NodeIdentityKey   string
	ConfigRevisionKey string
	DisplayName       string
	ProbeType         string
	Target            string
	Timestamp         time.Time
	Success           bool
	Latency           time.Duration
	TTFB              time.Duration
	ErrorClass        string
	ErrorDetail       string
	// RegionBlocked is a transport of an explicit upstream region-block flag
	// (e.g. metadata.region_blocked). Interpretation lives in ClassifyFailure.
	RegionBlocked bool
}

// EvidenceNode describes a node targeted for evidence evaluation.
//
// ProfileID is mandatory for isolation: without it, samples belonging to the same
// physical endpoint under a different subscription/profile could be aggregated.
type EvidenceNode struct {
	ProfileID         string
	NodeKey           string
	NodeIdentityKey   string
	ConfigRevisionKey string
	DisplayName       string
}

// SampleSetKey is the key under which this node's raw samples must be provided in
// EvidenceInput.SamplesByNode.
//
// It is deliberately NOT NodeKey: the same subscription node can legitimately appear in
// two profiles with identical credentials, in which case ProfileID, NodeKey,
// NodeIdentityKey and ConfigRevisionKey are all equal and ProfileID is the only
// discriminator. Keying by NodeKey alone would make those two logical nodes collide, so
// one would silently read the other's evidence.
func (n EvidenceNode) SampleSetKey() string {
	return n.ProfileID + "\x00" + n.NodeIdentityKey + "\x00" + n.NodeKey
}

// EvidenceSource records the provenance of the underlying raw samples.
type EvidenceSource struct {
	JobID         string    `json:"job_id,omitempty"`
	ProfileID     string    `json:"profile_id,omitempty"`
	ProbeSet      string    `json:"probe_set,omitempty"`
	NodeSetSource string    `json:"node_set_source,omitempty"` // "monitor_job" | "persisted_samples"
	LookbackSince time.Time `json:"lookback_since"`
	LookbackUntil time.Time `json:"lookback_until"`
}

// EvidenceInput is the complete, pure input to BuildEvidenceSnapshot.
type EvidenceInput struct {
	Now     time.Time
	Purpose PolicyPurpose
	Policy  SwitchPolicy
	Source  EvidenceSource
	// EvidenceWindow is how much history is scanned. It is resolved independently of
	// MaxSampleAge (see DefaultEvidenceWindow). Zero means "derive the default".
	EvidenceWindow time.Duration
	// CurrentNodeKey / CurrentNodeIdentityKey identify the currently active node.
	// Either may be empty when the current node cannot be resolved.
	CurrentNodeKey         string
	CurrentNodeIdentityKey string
	// Nodes is the set of nodes to evaluate. Order is preserved.
	Nodes []EvidenceNode
	// SamplesByNode maps EvidenceNode.SampleSetKey() → all raw samples observed for that
	// node's transport identity inside the scan window. Samples may therefore belong to
	// other profiles or older ConfigRevisionKey revisions; the builder filters them.
	//
	// The key must be EvidenceNode.SampleSetKey() and not NodeKey: two logical nodes can
	// share a NodeKey (same subscription node in two profiles with identical credentials).
	SamplesByNode map[string][]EvidenceSample
	// RawSampleCount is the total number of raw samples fetched from persistence.
	RawSampleCount int
	// BudgetExceededNodes lists the SampleSetKey()s whose raw-sample window could not be read
	// COMPLETELY within the per-node budget. A partial read must never be presented as complete
	// evidence, so such a node is gated as insufficient instead.
	BudgetExceededNodes map[string]bool
	// PagesReadByNode records how many cursor pages were drained per SampleSetKey(). It is
	// provenance only (never used for a decision) so a reviewer can verify that multi-page
	// evidence was actually read completely.
	PagesReadByNode map[string]int
}

// DefaultEvidenceWindow resolves how much history to scan when the caller does not
// specify a window explicitly.
//
// It is deliberately NOT "MaxSampleAge": MaxSampleAge is a freshness gate on the newest
// sample, not a statement about how much history is meaningful. Tying the two together
// produces false negatives (a node with a brand-new sample and a long, otherwise
// sufficient history would be gated as insufficient purely because the window was narrow).
//
// The window is instead driven by the policy's own observation requirements, with
// headroom, and floored so that staleness can be detected and reported with a real age:
//
//	max(4 × MinObservationWindow, 4 × MaxSampleAge, 1h), capped at 24h,
//	and never smaller than MinObservationWindow.
//
// The last clause is load-bearing for MinObservationWindow > 24h: the cap bounds how much
// history a pathological policy can scan, but it must NEVER truncate the window below an
// explicitly configured MinObservationWindow, because that would make the requirement
// unsatisfiable forever and the node permanently insufficient_evidence with no configuration
// that could ever fix it. So the resolved semantics are: the cap applies to the derived
// window, and an explicit observation requirement always wins over it.
func DefaultEvidenceWindow(p SwitchPolicy) time.Duration {
	window := time.Hour
	if p.MinObservationWindow > 0 {
		if candidate := 4 * p.MinObservationWindow; candidate > window {
			window = candidate
		}
	}
	if p.MaxSampleAge > 0 {
		if candidate := 4 * p.MaxSampleAge; candidate > window {
			window = candidate
		}
	}
	if window > 24*time.Hour {
		window = 24 * time.Hour
	}
	if p.MinObservationWindow > window {
		window = p.MinObservationWindow
	}
	return window
}

// -----------------------------------------------------------------------------
// Evidence output (the snapshot that crosses into the policy engine)
// -----------------------------------------------------------------------------

// FreshnessStatus describes whether the node's newest sample is fresh enough to drive
// a decision. This is a property of the LATEST sample only — never of the window width.
type FreshnessStatus string

const (
	FreshnessFresh      FreshnessStatus = "fresh"
	FreshnessStale      FreshnessStatus = "stale"
	FreshnessNoEvidence FreshnessStatus = "no_evidence"
)

// Evidence gate reason codes.
const (
	GateReasonNoEvidence                     = "no_evidence"
	GateReasonStaleEvidence                  = "stale_evidence"
	GateReasonInsufficientSampleCount        = "insufficient_sample_count"
	GateReasonInsufficientObservationWindow  = "insufficient_observation_window"
	GateReasonOtherRevisionSamplesExcluded   = "evidence_only_from_other_config_revision"
	GateReasonUnknownRevisionSamplesExcluded = "unknown_revision_evidence_excluded"
	GateReasonOtherProfileSamplesExcluded    = "evidence_only_from_other_profile"
	GateReasonUnknownProfileSamplesExcluded  = "unknown_profile_evidence_excluded"
	GateReasonProfileIsolationAmbiguous      = "profile_isolation_unavailable"
	GateReasonEvidenceBudgetExceeded         = "evidence_budget_exceeded"
)

// EvidenceGate is the exact threshold set that was applied, echoed into the
// recommendation so the caller can explain "why".
//
// Note the deliberate separation: MaxSampleAge gates freshness; EvidenceWindow /
// WindowSince / WindowUntil describe how much history was scanned.
type EvidenceGate struct {
	MaxSampleAge         time.Duration `json:"max_sample_age"`
	MinSampleCount       int           `json:"min_sample_count"`
	MinObservationWindow time.Duration `json:"min_observation_window"`
	// EvidenceWindow is the length of history scanned (NOT derived from MaxSampleAge).
	EvidenceWindow time.Duration `json:"evidence_window"`
	// WindowSince / WindowUntil bound the scanned observation window.
	WindowSince time.Time `json:"window_since"`
	WindowUntil time.Time `json:"window_until"`
}

// EvidenceSufficiency states whether a node has enough trustworthy evidence.
type EvidenceSufficiency struct {
	Sufficient bool     `json:"sufficient"`
	Reasons    []string `json:"reasons,omitempty"` // stable machine-readable codes
	Details    []string `json:"details,omitempty"` // human readable explanation
}

// NodeEvidence is the per-node evidence bundle: raw statistics plus the
// purpose-aware verdicts derived from them, always with provenance.
type NodeEvidence struct {
	// --- identity / provenance ---
	ProfileID         string `json:"profile_id"`
	ProfileIDInferred bool   `json:"profile_id_inferred,omitempty"`
	NodeKey           string `json:"node_key"`
	NodeIdentityKey   string `json:"node_identity_key"`
	ConfigRevisionKey string `json:"config_revision_key"`
	DisplayName       string `json:"display_name"`
	IsCurrent         bool   `json:"is_current"`

	ProbeTypes []string `json:"probe_types,omitempty"`
	Targets    []string `json:"targets,omitempty"`

	// --- observation window (actual span of the considered samples) ---
	EvidenceWindowSince time.Time     `json:"evidence_window_since"`
	EvidenceWindowUntil time.Time     `json:"evidence_window_until"`
	FirstSampleAt       time.Time     `json:"first_sample_at"`
	LastSampleAt        time.Time     `json:"last_sample_at"`
	ObservationWindow   time.Duration `json:"observation_window"`
	SampleAge           time.Duration `json:"sample_age"`

	// --- counts / rates ---
	SampleCount  int     `json:"sample_count"`
	SuccessCount int     `json:"success_count"`
	FailureCount int     `json:"failure_count"`
	SuccessRate  float64 `json:"success_rate"`

	// --- scope-aware counts (purpose semantics) ---
	TransportSampleCount         int `json:"transport_sample_count"`
	TransportSuccessCount        int `json:"transport_success_count"`
	TransportFailureCount        int `json:"transport_failure_count"`
	ServiceSampleCount           int `json:"service_sample_count"`
	ServiceFailureCount          int `json:"service_failure_count"`
	AISampleCount                int `json:"ai_sample_count"`
	AIBlockCount                 int `json:"ai_block_count"`
	ConsecutiveTransportFailures int `json:"consecutive_transport_failures"`
	ConsecutiveAIFailures        int `json:"consecutive_ai_failures"`
	ConsecutiveServiceFailures   int `json:"consecutive_service_failures"`
	// ConsecutiveFailures is purpose-scoped and is what feeds DecisionState.
	ConsecutiveFailures int `json:"consecutive_failures"`

	// --- latency / ttfb percentiles (transport-scope successful samples only) ---
	// Restricting to transport scope keeps latency comparable across nodes: service
	// probes (google.com, api.github.com) measure a different path length and would
	// otherwise skew the ranking signal.
	LatencySampleCount int           `json:"latency_sample_count"`
	LatencyP50         time.Duration `json:"latency_p50"`
	LatencyP95         time.Duration `json:"latency_p95"`
	TTFBP50            time.Duration `json:"ttfb_p50"`
	TTFBP95            time.Duration `json:"ttfb_p95"`

	ErrorBreakdown map[string]int `json:"error_breakdown,omitempty"`

	// --- derived verdicts ---
	Freshness FreshnessStatus `json:"freshness"`
	// TransportHealthy means the raw transport path to a neutral endpoint works.
	// Service-level or AI-level rejections do NOT make a node transport-unhealthy.
	TransportHealthy bool `json:"transport_healthy"`
	// ServiceDegraded is informational under PurposeGeneral (Google/AI reachability
	// problems do not imply the node is broken for general traffic).
	ServiceDegraded bool `json:"service_degraded"`
	// BlockedForPurpose means the node carries confirmed region/service rejection
	// evidence that disqualifies it for the requested purpose. Only ever true under
	// PurposeAI.
	BlockedForPurpose bool `json:"blocked_for_purpose"`
	// TriageStatus mirrors the existing NodeEvaluation vocabulary:
	// "ok" | "failed" | "blocked".
	TriageStatus string `json:"triage_status"`

	Sufficiency EvidenceSufficiency `json:"evidence_sufficiency"`

	// --- isolation / integrity counters (never silently averaged in) ---
	ExcludedOtherProfileSamples    int  `json:"excluded_other_profile_samples"`
	ExcludedUnknownProfileSamples  int  `json:"excluded_unknown_profile_samples"`
	ExcludedOtherRevisionSamples   int  `json:"excluded_other_revision_samples"`
	ExcludedUnknownRevisionSamples int  `json:"excluded_unknown_revision_samples"`
	ExcludedFutureSamples          int  `json:"excluded_future_samples"`
	ProfileIsolationAmbiguous      bool `json:"profile_isolation_ambiguous,omitempty"`
	// ProfileIsolationUnknown is true when the evidence carries no profile provenance at
	// all (neither configured nor inferable). The samples are still used — there is nothing
	// to discriminate on — but the isolation gap is recorded rather than hidden.
	ProfileIsolationUnknown bool `json:"profile_isolation_unknown,omitempty"`
	// EvidenceBudgetExceeded means the raw-sample window could not be drained completely.
	// The statistics above are then INCOMPLETE and the node is gated as insufficient.
	EvidenceBudgetExceeded bool `json:"evidence_budget_exceeded,omitempty"`
	// PagesRead is how many cursor pages were drained for this node (B-01 provenance).
	PagesRead int `json:"pages_read"`
}

// EvidenceSnapshot is the full, self-describing evidence bundle handed to the policy engine.
type EvidenceSnapshot struct {
	GeneratedAt            time.Time      `json:"generated_at"`
	Purpose                PolicyPurpose  `json:"purpose"`
	Source                 EvidenceSource `json:"source"`
	Gate                   EvidenceGate   `json:"gate"`
	CurrentNodeKey         string         `json:"current_node_key,omitempty"`
	CurrentNodeIdentityKey string         `json:"current_node_identity_key,omitempty"`
	Nodes                  []NodeEvidence `json:"nodes"`
	RawSampleCount         int            `json:"raw_sample_count"`
	Truncated              bool           `json:"truncated"`
}

// CurrentNode resolves the evidence bundle for the current node, if present.
func (s *EvidenceSnapshot) CurrentNode() *NodeEvidence {
	for i := range s.Nodes {
		if s.Nodes[i].IsCurrent {
			return &s.Nodes[i]
		}
	}
	if s.CurrentNodeKey != "" {
		for i := range s.Nodes {
			if s.Nodes[i].NodeKey == s.CurrentNodeKey {
				return &s.Nodes[i]
			}
		}
	}
	if s.CurrentNodeIdentityKey != "" {
		for i := range s.Nodes {
			if s.Nodes[i].NodeIdentityKey == s.CurrentNodeIdentityKey {
				return &s.Nodes[i]
			}
		}
	}
	return nil
}

// clockSkewTolerance is the forward tolerance applied to sample timestamps so a few
// seconds of local clock drift do not silently drop otherwise valid evidence.
const clockSkewTolerance = 5 * time.Second

// BuildEvidenceSnapshot projects raw samples into a purpose-aware EvidenceSnapshot.
//
// It is a pure function: no store, no clock, no controller, no I/O.
func BuildEvidenceSnapshot(in EvidenceInput) EvidenceSnapshot {
	now := in.Now
	purpose := in.Purpose
	if purpose == "" {
		purpose = PurposeGeneral
	}

	evidenceWindow := in.EvidenceWindow
	if evidenceWindow <= 0 {
		evidenceWindow = DefaultEvidenceWindow(in.Policy)
	}
	windowSince := now.Add(-evidenceWindow)

	gate := EvidenceGate{
		MaxSampleAge:         in.Policy.MaxSampleAge,
		MinSampleCount:       in.Policy.MinSampleCount,
		MinObservationWindow: in.Policy.MinObservationWindow,
		EvidenceWindow:       evidenceWindow,
		WindowSince:          windowSince,
		WindowUntil:          now,
	}

	snap := EvidenceSnapshot{
		GeneratedAt:            now,
		Purpose:                purpose,
		Source:                 in.Source,
		Gate:                   gate,
		CurrentNodeKey:         in.CurrentNodeKey,
		CurrentNodeIdentityKey: in.CurrentNodeIdentityKey,
		RawSampleCount:         in.RawSampleCount,
		Truncated:              len(in.BudgetExceededNodes) > 0,
	}

	// hasCurrent tracks whether the current node has already been claimed, so that only the
	// FIRST match is marked current.
	hasCurrent := false
	for _, node := range in.Nodes {
		// Only the FIRST match is marked current: when two logical nodes share a NodeKey the
		// input is genuinely ambiguous, and marking both would be worse than picking one
		// deterministically.
		isCurrent := false
		if !hasCurrent {
			if in.CurrentNodeKey != "" && node.NodeKey == in.CurrentNodeKey {
				isCurrent = true
			} else if in.CurrentNodeKey == "" && in.CurrentNodeIdentityKey != "" &&
				node.NodeIdentityKey == in.CurrentNodeIdentityKey {
				isCurrent = true
			}
		}
		if isCurrent {
			hasCurrent = true
		}
		ev := buildNodeEvidence(
			now, purpose, in.Policy, gate, node,
			in.SamplesByNode[node.SampleSetKey()],
			in.BudgetExceededNodes[node.SampleSetKey()],
			in.PagesReadByNode[node.SampleSetKey()],
		)
		ev.IsCurrent = isCurrent
		snap.Nodes = append(snap.Nodes, ev)
	}
	return snap
}

func buildNodeEvidence(
	now time.Time,
	purpose PolicyPurpose,
	p SwitchPolicy,
	gate EvidenceGate,
	node EvidenceNode,
	samples []EvidenceSample,
	budgetExceeded bool,
	pagesRead int,
) NodeEvidence {
	ev := NodeEvidence{
		ProfileID:           strings.TrimSpace(node.ProfileID),
		NodeKey:             node.NodeKey,
		NodeIdentityKey:     node.NodeIdentityKey,
		ConfigRevisionKey:   node.ConfigRevisionKey,
		DisplayName:         node.DisplayName,
		EvidenceWindowSince: gate.WindowSince,
		EvidenceWindowUntil: gate.WindowUntil,
		ErrorBreakdown:      map[string]int{},
		PagesRead:           pagesRead,

		EvidenceBudgetExceeded: budgetExceeded,
	}

	targetRev := strings.TrimSpace(node.ConfigRevisionKey)

	// --- Stage 1: temporal + config-revision attribution -------------------------
	var candidates []EvidenceSample
	for _, s := range samples {
		if targetRev != "" {
			rev := strings.TrimSpace(s.ConfigRevisionKey)
			if rev == "" {
				// Legacy sample with no revision provenance. Attributing it to the current
				// revision would silently mix pre-change evidence, so it is excluded.
				ev.ExcludedUnknownRevisionSamples++
				continue
			}
			if rev != targetRev {
				// Evidence recorded under a different config/credential revision must never
				// be mixed with the current revision.
				ev.ExcludedOtherRevisionSamples++
				continue
			}
		}

		if s.Timestamp.After(now.Add(clockSkewTolerance)) {
			ev.ExcludedFutureSamples++
			continue
		}
		if s.Timestamp.Before(gate.WindowSince) {
			continue
		}
		candidates = append(candidates, s)
	}

	// --- Stage 2: profile isolation ---------------------------------------------
	// NodeIdentityKey is a pure transport endpoint and is explicitly NOT unique per
	// logical node (fingerprint.go R-01): two profiles can share the same CDN host or
	// reverse-proxy endpoint. ProfileID must therefore qualify the attribution.
	profilesSeen := map[string]struct{}{}
	for _, s := range candidates {
		if profile := strings.TrimSpace(s.ProfileID); profile != "" {
			profilesSeen[profile] = struct{}{}
		}
	}

	effectiveProfile := ev.ProfileID
	switch {
	case effectiveProfile != "":
		// Known target profile: strict isolation.
	case len(profilesSeen) == 1:
		// Target profile unknown but all observed samples agree on one profile, so the
		// attribution is unambiguous. Record it as inferred rather than pretending it
		// was configured.
		for profile := range profilesSeen {
			effectiveProfile = profile
		}
		ev.ProfileID = effectiveProfile
		ev.ProfileIDInferred = true
	case len(profilesSeen) > 1:
		// The same transport endpoint carries samples from several profiles and the
		// target profile is unknown: attribution is impossible. Refuse to guess.
		ev.ProfileIsolationAmbiguous = true
	}

	var considered []EvidenceSample
	if ev.ProfileIsolationAmbiguous {
		for _, s := range candidates {
			if strings.TrimSpace(s.ProfileID) != "" {
				ev.ExcludedOtherProfileSamples++
			} else {
				ev.ExcludedUnknownProfileSamples++
			}
		}
	} else {
		for _, s := range candidates {
			profile := strings.TrimSpace(s.ProfileID)
			if effectiveProfile != "" {
				if profile == "" {
					ev.ExcludedUnknownProfileSamples++
					continue
				}
				if profile != effectiveProfile {
					ev.ExcludedOtherProfileSamples++
					continue
				}
			}
			considered = append(considered, s)
		}
	}

	// Sort newest first for consecutive-failure derivation.
	sort.SliceStable(considered, func(i, j int) bool {
		return considered[i].Timestamp.After(considered[j].Timestamp)
	})

	if effectiveProfile == "" && len(considered) > 0 {
		// Nothing to discriminate on: no sample carries a profile. Use them, but record the
		// isolation gap explicitly instead of pretending the evidence is profile-qualified.
		ev.ProfileIsolationUnknown = true
	}

	probeTypeSet := map[string]struct{}{}
	targetSet := map[string]struct{}{}
	var latencySamples []time.Duration
	var ttfbSamples []time.Duration

	ev.SampleCount = len(considered)
	for _, s := range considered {
		if s.ProbeType != "" {
			probeTypeSet[s.ProbeType] = struct{}{}
		}
		if s.Target != "" {
			targetSet[s.Target] = struct{}{}
		}

		scope := ClassifyProbeScope(s.ProbeType)
		switch scope {
		case ScopeTransport:
			ev.TransportSampleCount++
		case ScopeService:
			ev.ServiceSampleCount++
		case ScopeAI:
			ev.AISampleCount++
		}

		if s.Success {
			ev.SuccessCount++
			if scope == ScopeTransport {
				ev.TransportSuccessCount++
				if s.Latency > 0 {
					latencySamples = append(latencySamples, s.Latency)
				}
				if s.TTFB > 0 {
					ttfbSamples = append(ttfbSamples, s.TTFB)
				}
			}
		} else {
			ev.FailureCount++
			if s.ErrorClass != "" {
				ev.ErrorBreakdown[s.ErrorClass]++
			} else {
				ev.ErrorBreakdown["unknown"]++
			}
			switch ClassifyFailure(s.ProbeType, s.ErrorClass, s.ErrorDetail, s.RegionBlocked) {
			case FailureTransport:
				ev.TransportFailureCount++
			case FailureService:
				ev.ServiceFailureCount++
			case FailureAIBlock:
				ev.AIBlockCount++
			}
		}
	}

	if ev.SampleCount > 0 {
		ev.SuccessRate = math.Round((float64(ev.SuccessCount)/float64(ev.SampleCount))*10000) / 10000
	}

	// --- observation window = the ACTUAL span of considered samples --------------
	// This is the value MinObservationWindow gates, matching the existing
	// NodeEvaluation.ObservationWindow semantics.
	if len(considered) > 0 {
		first := considered[len(considered)-1].Timestamp
		last := considered[0].Timestamp
		ev.FirstSampleAt = first
		ev.LastSampleAt = last
		if !last.Before(first) {
			ev.ObservationWindow = last.Sub(first)
		}
		ev.SampleAge = now.Sub(last)
	}

	// --- percentiles (nearest-rank, matching the SQL derived-stats convention) ---
	sort.Slice(latencySamples, func(i, j int) bool { return latencySamples[i] < latencySamples[j] })
	sort.Slice(ttfbSamples, func(i, j int) bool { return ttfbSamples[i] < ttfbSamples[j] })
	ev.LatencySampleCount = len(latencySamples)
	ev.LatencyP50 = percentileOf(latencySamples, 0.50)
	ev.LatencyP95 = percentileOf(latencySamples, 0.95)
	ev.TTFBP50 = percentileOf(ttfbSamples, 0.50)
	ev.TTFBP95 = percentileOf(ttfbSamples, 0.95)

	// --- consecutive failures (per scope, newest first) ---
	ev.ConsecutiveTransportFailures = consecutiveFailures(considered, ScopeTransport, false)
	ev.ConsecutiveAIFailures = consecutiveFailures(considered, ScopeAI, false)
	ev.ConsecutiveServiceFailures = consecutiveFailures(considered, ScopeService, true)

	// --- purpose-aware verdicts ---
	switch purpose {
	case PurposeAI:
		ev.ConsecutiveFailures = ev.ConsecutiveTransportFailures + ev.ConsecutiveAIFailures
		// Under AI purpose, confirmed region/service rejection is a hard failure:
		// the node cannot serve AI traffic even if its raw transport works.
		ev.BlockedForPurpose = ev.AIBlockCount > 0 || ev.ServiceFailureCount > 0
		ev.ServiceDegraded = ev.ServiceFailureCount > 0
	default:
		ev.ConsecutiveFailures = ev.ConsecutiveTransportFailures
		// Under general purpose, service-level Google/AI blocking is explicitly NOT a
		// transport failure and must never mark the node as broken.
		ev.BlockedForPurpose = false
		ev.ServiceDegraded = ev.ServiceFailureCount > 0
	}

	ev.TransportHealthy = true
	if ev.TransportSampleCount > 0 && ev.TransportSuccessCount == 0 {
		ev.TransportHealthy = false
	}
	if p.MaxConsecutiveFailures > 0 && ev.ConsecutiveTransportFailures >= p.MaxConsecutiveFailures {
		ev.TransportHealthy = false
	}

	ev.TriageStatus = "ok"
	if !ev.TransportHealthy {
		ev.TriageStatus = "failed"
	} else if ev.BlockedForPurpose {
		ev.TriageStatus = "blocked"
	}

	ev.ProbeTypes = sortedKeys(probeTypeSet)
	ev.Targets = sortedKeys(targetSet)

	ev.Freshness = deriveFreshness(ev, gate.MaxSampleAge)
	ev.Sufficiency = evaluateSufficiency(ev, gate)
	return ev
}

// deriveFreshness decides whether the node's NEWEST sample is fresh enough.
//
// MaxSampleAge is used here and only here: it is a freshness gate on the latest
// sample, never a definition of how much history is scanned.
func deriveFreshness(ev NodeEvidence, maxSampleAge time.Duration) FreshnessStatus {
	if ev.SampleCount == 0 {
		return FreshnessNoEvidence
	}
	if maxSampleAge > 0 && ev.SampleAge > maxSampleAge {
		return FreshnessStale
	}
	return FreshnessFresh
}

// evaluateSufficiency applies the conservative freshness / evidence gate. Insufficient
// evidence must yield "insufficient_evidence" downstream — never a hard recommendation.
func evaluateSufficiency(ev NodeEvidence, gate EvidenceGate) EvidenceSufficiency {
	sufficiency := EvidenceSufficiency{Sufficient: true}

	if ev.Freshness == FreshnessNoEvidence {
		sufficiency.Sufficient = false
		switch {
		case ev.ProfileIsolationAmbiguous:
			sufficiency.Reasons = append(sufficiency.Reasons, GateReasonProfileIsolationAmbiguous)
			sufficiency.Details = append(sufficiency.Details, fmt.Sprintf(
				"节点 %s 的传输端点 (%s) 在观察窗口内同时存在多个 Profile 的样本，而目标 ProfileID 未知，无法安全归属，拒绝猜测",
				ev.DisplayName, ev.NodeIdentityKey))
		case ev.ExcludedOtherRevisionSamples > 0 || ev.ExcludedUnknownRevisionSamples > 0:
			sufficiency.Reasons = append(sufficiency.Reasons, GateReasonOtherRevisionSamplesExcluded)
			sufficiency.Details = append(sufficiency.Details, fmt.Sprintf(
				"节点 %s 在观察窗口内只有 %d 条属于其他 ConfigRevisionKey、%d 条缺少配置版本来源的样本，当前配置版本无任何样本",
				ev.DisplayName, ev.ExcludedOtherRevisionSamples, ev.ExcludedUnknownRevisionSamples))
		case ev.ExcludedOtherProfileSamples > 0 || ev.ExcludedUnknownProfileSamples > 0:
			sufficiency.Reasons = append(sufficiency.Reasons, GateReasonOtherProfileSamplesExcluded)
			sufficiency.Details = append(sufficiency.Details, fmt.Sprintf(
				"节点 %s 在观察窗口内只有 %d 条属于其他 Profile、%d 条缺少 Profile 归属的样本，当前 Profile 无任何样本",
				ev.DisplayName, ev.ExcludedOtherProfileSamples, ev.ExcludedUnknownProfileSamples))
		default:
			sufficiency.Reasons = append(sufficiency.Reasons, GateReasonNoEvidence)
			sufficiency.Details = append(sufficiency.Details, fmt.Sprintf(
				"节点 %s 在观察窗口内没有任何原始样本", ev.DisplayName))
		}
	}

	if ev.Freshness == FreshnessStale {
		sufficiency.Sufficient = false
		sufficiency.Reasons = append(sufficiency.Reasons, GateReasonStaleEvidence)
		sufficiency.Details = append(sufficiency.Details, fmt.Sprintf(
			"节点 %s 最新样本距今 %s，超过 MaxSampleAge=%s，陈旧数据不得驱动推荐",
			ev.DisplayName, roundDuration(ev.SampleAge), roundDuration(gate.MaxSampleAge)))
	}

	if ev.EvidenceBudgetExceeded {
		// A partial read must never masquerade as complete evidence: the statistics below are
		// computed from an unknown fraction of the observation window.
		sufficiency.Sufficient = false
		sufficiency.Reasons = append(sufficiency.Reasons, GateReasonEvidenceBudgetExceeded)
		sufficiency.Details = append(sufficiency.Details, fmt.Sprintf(
			"节点 %s 在观察窗口内的原始样本超过单节点预算，未能完整读取（已读 %d 页），统计值不完整，按证据不足处理",
			ev.DisplayName, ev.PagesRead))
	}

	if gate.MinSampleCount > 0 && ev.SampleCount < gate.MinSampleCount {
		sufficiency.Sufficient = false
		sufficiency.Reasons = append(sufficiency.Reasons, GateReasonInsufficientSampleCount)
		sufficiency.Details = append(sufficiency.Details, fmt.Sprintf(
			"节点 %s 观察窗口内样本数 %d 少于 MinSampleCount=%d",
			ev.DisplayName, ev.SampleCount, gate.MinSampleCount))
	}

	if gate.MinObservationWindow > 0 && ev.ObservationWindow < gate.MinObservationWindow {
		sufficiency.Sufficient = false
		sufficiency.Reasons = append(sufficiency.Reasons, GateReasonInsufficientObservationWindow)
		sufficiency.Details = append(sufficiency.Details, fmt.Sprintf(
			"节点 %s 实际观察窗口 %s 短于 MinObservationWindow=%s",
			ev.DisplayName, roundDuration(ev.ObservationWindow), roundDuration(gate.MinObservationWindow)))
	}

	return sufficiency
}

// consecutiveFailures counts leading failures for the given scope when walking
// samples newest-first. When blockingOnly is true, only explicit region/service
// rejections break the streak (a plain timeout on a service probe is transport evidence).
func consecutiveFailures(samples []EvidenceSample, scope ProbeScope, blockingOnly bool) int {
	count := 0
	for _, s := range samples {
		if ClassifyProbeScope(s.ProbeType) != scope {
			continue
		}
		if s.Success {
			break
		}
		if blockingOnly {
			if ClassifyFailure(s.ProbeType, s.ErrorClass, s.ErrorDetail, s.RegionBlocked) != FailureService {
				continue
			}
		}
		count++
	}
	return count
}

// percentileOf implements the nearest-rank percentile (same convention as the
// SQL derived-stats implementation) over an ascending-sorted slice.
func percentileOf(sortedAsc []time.Duration, p float64) time.Duration {
	n := len(sortedAsc)
	if n == 0 {
		return 0
	}
	rank := int(math.Ceil(p*float64(n))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= n {
		rank = n - 1
	}
	return sortedAsc[rank]
}

func sortedKeys(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func roundDuration(d time.Duration) time.Duration {
	if d == 0 {
		return 0
	}
	if d < time.Second {
		return d.Round(time.Millisecond)
	}
	return d.Round(time.Second)
}

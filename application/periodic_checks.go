package application

import (
	"context"
	"fmt"
	"time"
)

type PeriodicServiceCheck struct {
	ServiceID       string     `json:"service_id"`
	Name            string     `json:"name"`
	LastDetectedAt  *time.Time `json:"last_detected_at"`
	Execution       string     `json:"execution"`
	Persistence     string     `json:"persistence"`
	Outcome         string     `json:"outcome"`
	CompetingProbes string     `json:"competing_probes,omitempty"`
}
type PeriodicChecks struct {
	Nodes  []MonitorNodeOptionDTO `json:"nodes"`
	Node   MonitorNodeOptionDTO   `json:"node"`
	Checks []PeriodicServiceCheck `json:"checks"`
}

func (s *AppService) GetPeriodicServiceChecks(ctx context.Context, profile, nodeKey string) (PeriodicChecks, error) {
	p := s.periodic
	p.mu.Lock()
	cfg := p.cfg
	p.mu.Unlock()
	nodes, err := p.nodes(cfg)
	if err != nil {
		return PeriodicChecks{}, err
	}
	if len(nodes) == 0 {
		return PeriodicChecks{}, fmt.Errorf("当前没有巡检节点")
	}
	node := nodes[0]
	if profile != "" || nodeKey != "" {
		found := false
		for _, n := range nodes {
			if n.ProfileID == profile && n.NodeKey == nodeKey {
				node = n
				found = true
				break
			}
		}
		if !found {
			return PeriodicChecks{}, fmt.Errorf("节点不在当前巡检范围")
		}
	}
	rows, err := s.historyStore.DB().LatestPublicServiceChecks(ctx, node.ProfileID, node.NodeIdentityKey, node.ConfigRevisionKey)
	if err != nil {
		return PeriodicChecks{}, err
	}
	result := PeriodicChecks{Nodes: nodes, Node: node, Checks: []PeriodicServiceCheck{}}
	for _, rule := range periodicServices(cfg) {
		check := PeriodicServiceCheck{ServiceID: rule.ServiceID, Name: rule.Name, Execution: "unknown", Persistence: "not_applicable", Outcome: "unknown"}
		for _, a := range rows {
			if a.ServiceID != rule.ServiceID {
				continue
			}
			check.Execution = a.ExecutionState
			check.Persistence = a.PersistenceState
			if a.Result != nil {
				at := a.Result.FinishedAt
				check.LastDetectedAt = &at
				check.Outcome = a.Result.Outcome
				check.CompetingProbes = a.Result.Details["competing_probes"]
			}
			break
		}
		result.Checks = append(result.Checks, check)
	}
	return result, nil
}

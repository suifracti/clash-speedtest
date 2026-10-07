package application

import (
	"context"
	"fmt"
	"github.com/faceair/clash-speedtest/core/publicservice"
	"time"
)

type PeriodicSampleRequest struct {
	Concurrency int                    `json:"concurrency"`
	Selections  []MonitorNodeOptionDTO `json:"selections"`
	ServiceIDs  []string               `json:"service_ids"`
}

func validatePeriodicSample(req PeriodicSampleRequest) error {
	if req.Concurrency != 16 && req.Concurrency != 32 && req.Concurrency != 64 {
		return fmt.Errorf("样本并发仅支持16/32/64")
	}
	if len(req.Selections) < 1 || len(req.Selections) > 16 || len(req.ServiceIDs) < 1 || len(req.ServiceIDs) > 8 {
		return fmt.Errorf("样本限定1..16节点与1..8服务，不能默认为全量")
	}
	seen := map[string]bool{}
	for _, id := range req.ServiceIDs {
		if _, ok := publicservice.RuleFor(id); !ok || seen[id] {
			return fmt.Errorf("样本服务未知或重复")
		}
		seen[id] = true
		if id == "antigravity" && len(req.Selections) > 1 {
			return fmt.Errorf("模型样本最多一个节点")
		}
	}
	for _, n := range req.Selections {
		key := n.ProfileID + "\x00" + n.NodeKey
		if seen[key] {
			return fmt.Errorf("样本节点重复")
		}
		seen[key] = true
	}
	return nil
}

// Explicit small experiments use the original measurement/save path. They do
// not change the persistent full catalog or latency configuration.
func (s *AppService) RunPeriodicSamplingSample(ctx context.Context, req PeriodicSampleRequest) (PeriodicCycle, error) {
	if err := validatePeriodicSample(req); err != nil {
		return PeriodicCycle{}, err
	}
	p := s.periodic
	p.lifecycle.Lock()
	defer p.lifecycle.Unlock()
	p.mu.Lock()
	running := p.cancel != nil
	closed := p.closed
	cfg := p.cfg
	p.mu.Unlock()
	if running || closed {
		return PeriodicCycle{}, fmt.Errorf("先停止全量服务/下载采样，再执行有限样本；延迟任务可保持原配置运行")
	}
	cfg.Selections = req.Selections
	cfg.ServiceConcurrency = req.Concurrency
	if err := validatePeriodicSamplingConfig(cfg); err != nil {
		return PeriodicCycle{}, err
	}
	nodes, err := p.nodes(cfg)
	if err != nil {
		return PeriodicCycle{}, err
	}
	rules := []publicservice.Rule{}
	for _, id := range req.ServiceIDs {
		r, _ := publicservice.RuleFor(id)
		rules = append(rules, r)
	}
	sample := &periodicSampling{app: s, cycles: map[string]PeriodicCycle{}, nodesHook: func() ([]MonitorNodeOptionDTO, error) { return nodes, nil }, servicesHook: func(PeriodicSamplingConfig) []publicservice.Rule { return rules }}
	ctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, scheduledRoundContextKey{}, "diagnostic")
	sample.runCycle(ctx, "service", time.Now(), cfg)
	sample.mu.Lock()
	defer sample.mu.Unlock()
	return sample.cycles["service"], nil
}

// Keep existing latency schedules and frozen selections running while explicit
// experiments suspend only the full service/download loops. Persist before
// stopping so an unexpected restart cannot collide with the sample.
func (s *AppService) PausePeriodicMeasurementsForSample() (*PeriodicSamplingStatus, error) {
	p := s.periodic
	p.lifecycle.Lock()
	defer p.lifecycle.Unlock()
	p.mu.Lock()
	cfg := p.cfg
	cfg.Enabled = false
	err := p.persistLocked(cfg)
	if err == nil {
		p.cfg = cfg
	}
	p.mu.Unlock()
	if err != nil {
		return nil, err
	}
	p.stopMeasurementsLocked()
	return s.GetPeriodicSampling()
}

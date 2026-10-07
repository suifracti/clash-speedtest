package application

import (
	"context"
	"errors"
	"fmt"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/publicservice"
	"time"
)

const periodicMaximumServiceConcurrency = 64
const periodicDownloadBatchSize = 4

func periodicServiceConcurrency(cfg PeriodicSamplingConfig) int {
	if cfg.ServiceConcurrency == 0 {
		return 16
	}
	return cfg.ServiceConcurrency
}

type periodicTask struct {
	queuedAt time.Time
	node     MonitorNodeOptionDTO
	service  string
}

// Each live profile gets one turn. Within it, nodes rotate; the service
// offsets ensure the first large profile cannot monopolize a single target.
func periodicFairServiceTasks(nodes []MonitorNodeOptionDTO, rules []publicservice.Rule) []periodicTask {
	if len(rules) == 0 {
		return nil
	}
	order := []string{}
	groups := map[string][]MonitorNodeOptionDTO{}
	ordinal := map[string]int{}
	for i, n := range nodes {
		if _, ok := groups[n.ProfileID]; !ok {
			order = append(order, n.ProfileID)
		}
		groups[n.ProfileID] = append(groups[n.ProfileID], n)
		ordinal[n.ProfileID+"\x00"+n.NodeKey] = i
	}
	cursor := map[string]int{}
	tasks := make([]periodicTask, 0, len(nodes)*len(rules))
	for len(tasks) < len(nodes)*len(rules) {
		for _, profile := range order {
			group := groups[profile]
			pos := cursor[profile]
			if pos >= len(group)*len(rules) {
				continue
			}
			n := group[pos%len(group)]
			service := (pos/len(group) + ordinal[profile+"\x00"+n.NodeKey]) % len(rules)
			tasks = append(tasks, periodicTask{node: n, service: rules[service].ServiceID})
			cursor[profile]++
		}
	}
	return tasks
}

// Admission retries reuse RequestID. Only a typed busy condition is retried;
// real measurement, revision and storage failures are never remeasured here.
func (p *periodicSampling) startPeriodicPublicService(ctx context.Context, req WorkbenchPublicServiceTestRequest, limit int) (*history.PublicServiceAttempt, float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	waited := 0.0
	for {
		if err := ctx.Err(); err != nil {
			return nil, waited, fmt.Errorf("未执行：admission 阶段原请求预算耗尽或取消: %w", err)
		}
		attempt, err := p.app.startWorkbenchPublicServiceTest(withProbeTiming(ctx, "admission_wait_ns", int64(waited*1e9)), req, limit)
		if !errors.Is(err, errWorkbenchAdmissionBusy) {
			return attempt, waited, err
		}
		begin := time.Now()
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			waited += time.Since(begin).Seconds()
			return nil, waited, fmt.Errorf("未执行：admission 阶段原请求预算耗尽或取消: %w", ctx.Err())
		case <-timer.C:
			waited += time.Since(begin).Seconds()
		}
	}
}
func (p *periodicSampling) addPeriodicWait(kind string, seconds float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cycles == nil {
		return
	}
	c := p.cycles[kind]
	c.WaitSeconds += seconds
	p.cycles[kind] = c
}

func (p *periodicSampling) startPeriodicDownload(ctx context.Context, req WorkbenchDownloadTestRequest) (*history.WorkbenchDownloadAttempt, float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	waited := 0.0
	for {
		if err := ctx.Err(); err != nil {
			return nil, waited, err
		}
		a, err := p.app.StartWorkbenchDownloadTest(withProbeTiming(ctx, "admission_wait_ns", int64(waited*1e9)), req)
		if !errors.Is(err, errWorkbenchAdmissionBusy) {
			return a, waited, err
		}
		begin := time.Now()
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			waited += time.Since(begin).Seconds()
			return nil, waited, ctx.Err()
		case <-timer.C:
			waited += time.Since(begin).Seconds()
		}
	}
}

func periodicFairDownloadNodes(nodes []MonitorNodeOptionDTO) []MonitorNodeOptionDTO {
	tasks := periodicFairServiceTasks(nodes, []publicservice.Rule{{ServiceID: "download"}})
	out := make([]MonitorNodeOptionDTO, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, task.node)
	}
	return out
}

package application

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/faceair/clash-speedtest/adapter/controller/mihomo"
	"github.com/faceair/clash-speedtest/core/speedtester"
)

type ClashConnectionSettings struct {
	Mode     string `json:"mode"`
	Endpoint string `json:"endpoint"`
	Secret   string `json:"secret"`
}
type ClashNodeStatus struct {
	Connected bool   `json:"connected"`
	Loaded    bool   `json:"loaded"`
	Name      string `json:"name"`
	Endpoint  string `json:"endpoint"`
}
type ClashMeasurement struct {
	Success   bool   `json:"success"`
	LatencyMS int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}
type ClashComparison struct {
	Name       string           `json:"name"`
	Target     string           `json:"target"`
	CST        ClashMeasurement `json:"cst"`
	Clash      ClashMeasurement `json:"clash"`
	FinishedAt time.Time        `json:"finished_at"`
}

func (s *AppService) clashClient() (*mihomo.Client, string, error) {
	settings, err := s.GetSettings()
	if err != nil {
		return nil, "", err
	}
	cfg := mihomo.Config{Timeout: 12 * time.Second}
	endpoint := fmt.Sprintf("/var/run/clash-verge-service/users/%d/verge-mihomo.sock", os.Getuid())
	if settings.ClashConnection != nil && settings.ClashConnection.Mode == "tcp" {
		cfg.Endpoint, cfg.Secret = settings.ClashConnection.Endpoint, settings.ClashConnection.Secret
		endpoint = cfg.Endpoint
	} else {
		cfg.SocketPath = endpoint
	}
	client, err := mihomo.NewClient(cfg)
	return client, endpoint, err
}

func (s *AppService) GetClashNodeStatus(ctx context.Context, scope WorkbenchLatencyBatchSelection) (ClashNodeStatus, error) {
	_, name, proxy, err := s.resolveWorkbenchScopedProxy(scope.ProfileID, scope.NodeKey, scope.NodeIdentityKey, scope.ConfigRevisionKey)
	if err != nil {
		return ClashNodeStatus{}, err
	}
	defer closeWorkbenchDownloadProxy(proxy)
	client, endpoint, err := s.clashClient()
	status := ClashNodeStatus{Name: name, Endpoint: endpoint}
	if err != nil {
		return status, nil
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	nodes, err := client.ListNodes(ctx, "")
	if err != nil {
		return status, nil
	}
	status.Connected = true
	for _, n := range nodes {
		if n.Name == name {
			status.Loaded = true
			break
		}
	}
	return status, nil
}

// CompareClashNode runs only from an explicit user action. It does not save a
// workbench round: these are two adjacent single samples, not a stability test.
func (s *AppService) CompareClashNode(ctx context.Context, scope WorkbenchLatencyBatchSelection) (*ClashComparison, error) {
	_, name, proxy, err := s.resolveWorkbenchScopedProxy(scope.ProfileID, scope.NodeKey, scope.NodeIdentityKey, scope.ConfigRevisionKey)
	if err != nil {
		return nil, err
	}
	defer closeWorkbenchDownloadProxy(proxy)
	client, _, err := s.clashClient()
	if err != nil {
		return nil, err
	}
	defer client.Close()
	nodes, err := client.ListNodes(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("无法连接 Clash 内核")
	}
	loaded := false
	for _, n := range nodes {
		if n.Name == name {
			loaded = true
			break
		}
	}
	if !loaded {
		return nil, fmt.Errorf("Clash 当前没有加载这个节点")
	}
	target, _ := speedtester.LatencyTargetURL("cloudflare")
	result := &ClashComparison{Name: name, Target: target}
	st, err := speedtester.New(&speedtester.Config{Mode: speedtester.SpeedModeFast, Metrics: speedtester.MetricSet{Latency: true}, Concurrent: 1, Timeout: 10 * time.Second, Rounds: 1, PingCount: 1, LatencyTargetURL: target})
	if err != nil {
		return nil, err
	}
	measured := st.TestSinglePhysicalContext(ctx, name, proxy, func(*speedtester.Result) bool { return ctx.Err() == nil })
	if measured != nil && len(measured.LatencySamples) > 0 {
		sample := measured.LatencySamples[0]
		result.CST = ClashMeasurement{Success: sample.Success, LatencyMS: sample.LatencyMs, Error: sample.Error}
	} else if measured != nil && measured.LatencyError != "" {
		result.CST.Error = measured.LatencyError
	} else {
		result.CST.Error = "未取得测量结果"
	}
	delay, err := client.TestDelay(ctx, name, target, 10*time.Second)
	if err != nil {
		result.Clash.Error = err.Error()
	} else {
		result.Clash.Success = true
		result.Clash.LatencyMS = delay.Milliseconds()
	}
	result.FinishedAt = time.Now().UTC()
	return result, nil
}

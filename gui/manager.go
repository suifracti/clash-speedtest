package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/faceair/clash-speedtest/history"
	"github.com/faceair/clash-speedtest/profiles"
	"github.com/faceair/clash-speedtest/speedtester"
)

type Event struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

type TestConfig struct {
	Metrics      []string `json:"metrics"` // "latency", "download", "upload", "antigravity"
	Concurrent   int      `json:"concurrent"`
	TimeoutSec   int      `json:"timeout_sec"`
	DownloadSize int      `json:"download_size"` // bytes
	UploadSize   int      `json:"upload_size"`   // bytes
	ServerURL    string   `json:"server_url"`
	Rounds       int      `json:"rounds"`
}

type BatchTestRequest struct {
	AirportID string     `json:"airport_id"`
	NodeNames []string   `json:"node_names"` // empty means all
	Config    TestConfig `json:"config"`
}

type SingleTestRequest struct {
	AirportID string     `json:"airport_id"`
	NodeName  string     `json:"node_name"`
	Config    TestConfig `json:"config"`
}

type TestStatus struct {
	IsRunning     bool      `json:"is_running"`
	CurrentNode   string    `json:"current_node,omitempty"`
	CurrentStep   string    `json:"current_step,omitempty"` // latency, download, upload, antigravity
	CurrentIndex  int       `json:"current_index"`
	TotalNodes    int       `json:"total_nodes"`
	Percent       int       `json:"percent"`
	AirportName   string    `json:"airport_name,omitempty"`
	StartedAt     time.Time `json:"started_at,omitempty"`
}

type Broadcaster struct {
	clients map[chan []byte]struct{}
	mu      sync.Mutex
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		clients: make(map[chan []byte]struct{}),
	}
}

func (b *Broadcaster) Subscribe() chan []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan []byte, 64)
	b.clients[ch] = struct{}{}
	return ch
}

func (b *Broadcaster) Unsubscribe(ch chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.clients, ch)
	close(ch)
}

func (b *Broadcaster) Broadcast(event Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- data:
		default:
		}
	}
}

type TestManager struct {
	historyStore  *history.Store
	profilePaths  profiles.Paths
	broadcaster   *Broadcaster
	cancelFunc    context.CancelFunc
	cancelCtx     context.Context
	mu            sync.Mutex
	status        TestStatus
	currentRunID  string
	stoppedByUser atomic.Bool
}

func NewTestManager(hStore *history.Store, paths profiles.Paths, broadcaster *Broadcaster) *TestManager {
	return &TestManager{
		historyStore: hStore,
		profilePaths: paths,
		broadcaster:  broadcaster,
	}
}

func (tm *TestManager) Status() TestStatus {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.status
}

func (tm *TestManager) Stop() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.cancelFunc != nil {
		tm.stoppedByUser.Store(true)
		tm.cancelFunc()
		tm.cancelFunc = nil
	}
	tm.status.IsRunning = false
	tm.broadcaster.Broadcast(Event{
		Type: "test_stopped",
		Payload: map[string]any{
			"message": "测试已被用户中断",
		},
	})
}

func (tm *TestManager) StartBatch(req BatchTestRequest, token string) error {
	tm.mu.Lock()
	if tm.status.IsRunning {
		tm.mu.Unlock()
		return fmt.Errorf("已有测速任务在运行中")
	}

	airportStore, err := profiles.LoadStore(tm.profilePaths.StoreFile())
	if err != nil {
		tm.mu.Unlock()
		return fmt.Errorf("加载机场配置失败: %w", err)
	}

	airport := airportStore.Get(req.AirportID)
	if airport == nil {
		tm.mu.Unlock()
		return fmt.Errorf("未找到指定机场")
	}

	cacheFile := tm.profilePaths.CacheFile(airport.ID)
	if !tm.profilePaths.HasCache(airport.ID) {
		tm.mu.Unlock()
		return fmt.Errorf("机场节点缓存不存在，请先更新订阅")
	}

	metrics := parseMetricSlice(req.Config.Metrics)
	mode := metrics.ToSpeedMode()

	concurrent := req.Config.Concurrent
	if concurrent <= 0 {
		concurrent = 4
	}
	timeoutSec := req.Config.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 5
	}
	serverURL := req.Config.ServerURL
	if serverURL == "" {
		serverURL = speedtester.DefaultSpeedServer
	}
	downloadSize := req.Config.DownloadSize
	if downloadSize <= 0 {
		downloadSize = 50 * 1024 * 1024
	}
	uploadSize := req.Config.UploadSize
	if uploadSize <= 0 {
		uploadSize = 20 * 1024 * 1024
	}

	stConfig := &speedtester.Config{
		ConfigPaths:      cacheFile,
		Mode:             mode,
		Metrics:          metrics,
		Concurrent:       concurrent,
		Timeout:          time.Duration(timeoutSec) * time.Second,
		DownloadSize:     downloadSize,
		UploadSize:       uploadSize,
		ServerURL:        serverURL,
		AntigravityToken: token,
		Rounds:           req.Config.Rounds,
	}

	st, err := speedtester.New(stConfig)
	if err != nil {
		tm.mu.Unlock()
		return fmt.Errorf("初始化测速引擎失败: %w", err)
	}

	allProxies, err := st.LoadProxies()
	if err != nil {
		tm.mu.Unlock()
		return fmt.Errorf("加载代理节点失败: %w", err)
	}

	var targetProxies map[string]*speedtester.CProxy
	if len(req.NodeNames) > 0 {
		nameSet := make(map[string]struct{}, len(req.NodeNames))
		for _, n := range req.NodeNames {
			nameSet[n] = struct{}{}
		}
		targetProxies = make(map[string]*speedtester.CProxy)
		for name, p := range allProxies {
			if _, ok := nameSet[name]; ok {
				targetProxies[name] = p
			}
		}
	} else {
		targetProxies = allProxies
	}

	if len(targetProxies) == 0 {
		tm.mu.Unlock()
		return fmt.Errorf("没有选中的可测试节点")
	}

	ctx, cancel := context.WithCancel(context.Background())
	tm.cancelCtx = ctx
	tm.cancelFunc = cancel
	tm.stoppedByUser.Store(false)
	tm.status = TestStatus{
		IsRunning:    true,
		TotalNodes:   len(targetProxies),
		AirportName:  airport.Name,
		StartedAt:    time.Now(),
		CurrentIndex: 0,
		Percent:      0,
	}
	tm.mu.Unlock()

	tm.broadcaster.Broadcast(Event{
		Type: "test_started",
		Payload: map[string]any{
			"airport_name": airport.Name,
			"total_nodes":  len(targetProxies),
			"metrics":      req.Config.Metrics,
		},
	})

	go tm.runBatchTest(ctx, st, targetProxies, airport.ID, airport.Name, req.Config.Metrics)
	return nil
}

func (tm *TestManager) runBatchTest(ctx context.Context, st *speedtester.SpeedTester, proxies map[string]*speedtester.CProxy, airportID, airportName string, metrics []string) {
	defer func() {
		tm.mu.Lock()
		tm.status.IsRunning = false
		tm.cancelFunc = nil
		tm.mu.Unlock()
	}()

	resultsMap := make(map[string]*history.RunNodeResult)
	var mapMu sync.Mutex
	currentIndex := 0
	total := len(proxies)

	names := make([]string, 0, len(proxies))
	for name := range proxies {
		names = append(names, name)
	}
	sort.Strings(names)

	isBandwidthTest := st.Metrics().Download || st.Metrics().Upload
	if !isBandwidthTest && len(proxies) > 1 {
		// Multi-node concurrency for latency and antigravity (safe since no bandwidth congestion)
		workerCount := 6
		if workerCount > len(proxies) {
			workerCount = len(proxies)
		}
		jobs := make(chan string, len(names))
		for _, n := range names {
			jobs <- n
		}
		close(jobs)

		var wg sync.WaitGroup
		for w := 0; w < workerCount; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for name := range jobs {
					select {
					case <-ctx.Done():
						return
					default:
					}
					p := proxies[name]
					st.TestSingle(name, p, func(res *speedtester.Result) bool {
						select {
						case <-ctx.Done():
							return false
						default:
						}
						nodeResult := convertResult(res)

						mapMu.Lock()
						if _, exists := resultsMap[res.ProxyName]; !exists {
							currentIndex++
						}
						resultsMap[res.ProxyName] = nodeResult
						currIdx := currentIndex
						mapMu.Unlock()

						tm.mu.Lock()
						tm.status.CurrentIndex = currIdx
						tm.status.CurrentNode = res.ProxyName
						if total > 0 {
							tm.status.Percent = int(math.Min(100, float64(currIdx*100)/float64(total)))
						}
						pct := tm.status.Percent
						tm.mu.Unlock()

						tm.broadcaster.Broadcast(Event{
							Type: "node_progress",
							Payload: map[string]any{
								"current_index": currIdx,
								"total":         total,
								"percent":       pct,
								"result":        nodeResult,
							},
						})
						return true
					})
				}
			}()
		}
		wg.Wait()
	} else {
		// Sequential test for bandwidth-sensitive metrics (Download/Upload)
		st.TestProxiesUntil(proxies, func(res *speedtester.Result) bool {
			select {
			case <-ctx.Done():
				return false
			default:
			}

			nodeResult := convertResult(res)

			mapMu.Lock()
			if _, exists := resultsMap[res.ProxyName]; !exists {
				currentIndex++
			}
			resultsMap[res.ProxyName] = nodeResult
			currIdx := currentIndex
			mapMu.Unlock()

			tm.mu.Lock()
			tm.status.CurrentIndex = currIdx
			tm.status.CurrentNode = res.ProxyName
			if total > 0 {
				tm.status.Percent = int(math.Min(100, float64(currIdx*100)/float64(total)))
			}
			pct := tm.status.Percent
			tm.mu.Unlock()

			tm.broadcaster.Broadcast(Event{
				Type: "node_progress",
				Payload: map[string]any{
					"current_index": currIdx,
					"total":         total,
					"percent":       pct,
					"result":        nodeResult,
				},
			})

			return true
		})
	}

	results := make([]*history.RunNodeResult, 0, len(resultsMap))
	for _, n := range names {
		if r, ok := resultsMap[n]; ok {
			results = append(results, r)
		}
	}

	if tm.stoppedByUser.Load() {
		return
	}

	// Save test run to history
	run := &history.TestRun{
		AirportID:   airportID,
		AirportName: airportName,
		CreatedAt:   time.Now(),
		Metrics:     metrics,
		TotalNodes:  total,
		PassedNodes: len(results),
		Results:     results,
	}

	runID, err := tm.historyStore.Save(run)
	if err != nil {
		log.Printf("保存历史记录失败: %s", err)
	}

	// Auto generate & save standalone HTML report
	allRuns, _ := tm.historyStore.GetAllRuns()
	if len(allRuns) == 0 {
		allRuns = []*history.TestRun{run}
	}
	reportPaths := []string{"report.html"}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		reportPaths = append(reportPaths, filepath.Join(home, ".clash-speedtest", "report.html"))
	}
	if err := history.SaveHTMLReport(allRuns, reportPaths...); err != nil {
		log.Printf("生成离线 HTML 报告失败: %s", err)
	}

	tm.broadcaster.Broadcast(Event{
		Type: "test_completed",
		Payload: map[string]any{
			"run_id":       runID,
			"airport_name": airportName,
			"total_nodes":  total,
			"passed_nodes": len(results),
			"report_file":  "report.html",
		},
	})
}

func (tm *TestManager) TestSingle(req SingleTestRequest, token string) (*history.RunNodeResult, error) {
	airportStore, err := profiles.LoadStore(tm.profilePaths.StoreFile())
	if err != nil {
		return nil, fmt.Errorf("加载机场配置失败: %w", err)
	}

	airport := airportStore.Get(req.AirportID)
	if airport == nil {
		return nil, fmt.Errorf("未找到指定机场")
	}

	cacheFile := tm.profilePaths.CacheFile(airport.ID)
	if !tm.profilePaths.HasCache(airport.ID) {
		return nil, fmt.Errorf("机场节点缓存不存在")
	}

	metrics := parseMetricSlice(req.Config.Metrics)
	mode := metrics.ToSpeedMode()

	concurrent := req.Config.Concurrent
	if concurrent <= 0 {
		concurrent = 4
	}
	timeoutSec := req.Config.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 5
	}
	serverURL := req.Config.ServerURL
	if serverURL == "" {
		serverURL = speedtester.DefaultSpeedServer
	}

	stConfig := &speedtester.Config{
		ConfigPaths:      cacheFile,
		Mode:             mode,
		Metrics:          metrics,
		Concurrent:       concurrent,
		Timeout:          time.Duration(timeoutSec) * time.Second,
		DownloadSize:     req.Config.DownloadSize,
		UploadSize:       req.Config.UploadSize,
		ServerURL:        serverURL,
		AntigravityToken: token,
		Rounds:           1,
	}

	st, err := speedtester.New(stConfig)
	if err != nil {
		return nil, fmt.Errorf("初始化测速引擎失败: %w", err)
	}

	allProxies, err := st.LoadProxies()
	if err != nil {
		return nil, fmt.Errorf("加载代理节点失败: %w", err)
	}

	proxy, exists := allProxies[req.NodeName]
	if !exists {
		return nil, fmt.Errorf("节点 %q 不存在", req.NodeName)
	}

	tm.broadcaster.Broadcast(Event{
		Type: "single_test_started",
		Payload: map[string]any{
			"node_name": req.NodeName,
		},
	})

	var lastRes *history.RunNodeResult
	st.TestSingle(req.NodeName, proxy, func(r *speedtester.Result) bool {
		lastRes = convertResult(r)
		tm.broadcaster.Broadcast(Event{
			Type: "single_node_progress",
			Payload: map[string]any{
				"result": lastRes,
			},
		})
		return true
	})

	tm.broadcaster.Broadcast(Event{
		Type: "single_test_completed",
		Payload: map[string]any{
			"result": lastRes,
		},
	})

	return lastRes, nil
}

func convertResult(r *speedtester.Result) *history.RunNodeResult {
	if r == nil {
		return nil
	}
	code := profiles.DetectCountry(r.ProxyName)
	flag := profiles.FlagFromCode(code)

	serverStr := ""
	portInt := 0
	if r.ProxyConfig != nil {
		if s, ok := r.ProxyConfig["server"].(string); ok {
			serverStr = s
		}
		if p, ok := r.ProxyConfig["port"].(int); ok {
			portInt = p
		}
	}

	var samples []history.LatencySample
	timeoutCount := 0
	if len(r.LatencySamples) > 0 {
		samples = make([]history.LatencySample, len(r.LatencySamples))
		for i, s := range r.LatencySamples {
			samples[i] = history.LatencySample{
				Seq:       s.Seq,
				Timestamp: s.Timestamp,
				LatencyMs: s.LatencyMs,
				Success:   s.Success,
				Error:     s.Error,
			}
			if !s.Success {
				timeoutCount++
			}
		}
	}

	return &history.RunNodeResult{
		ProxyName:         r.ProxyName,
		ProxyType:         r.ProxyType,
		Server:            serverStr,
		Port:              portInt,
		CountryCode:       code,
		CountryFlag:       flag,
		LatencyMs:         r.Latency.Milliseconds(),
		JitterMs:          r.Jitter.Milliseconds(),
		PacketLoss:        r.PacketLoss,
		LatencySamples:    samples,
		TimeoutCount:      timeoutCount,
		DownloadSpeedMBps: math.Round(r.DownloadSpeed*100) / 100,
		UploadSpeedMBps:   math.Round(r.UploadSpeed*100) / 100,
		DownloadError:     r.DownloadError,
		UploadError:       r.UploadError,
		AntigravityStatus: r.AntigravityStatus,
		AntigravityDetail: r.AntigravityDetail,
		GoogleTTFBMs:      r.GoogleTTFBMs,
		ExitCountry:       r.ExitCountry,
		ExitCountryCode:   r.ExitCountryCode,
		IPInfo: func() *history.IPInfo {
			if r.IPInfo == nil {
				return nil
			}
			var p0 *history.Ping0Info
			if r.IPInfo.Ping0 != nil {
				p0 = &history.Ping0Info{
					IP:        r.IPInfo.Ping0.IP,
					Location:  r.IPInfo.Ping0.Location,
					Country:   r.IPInfo.Ping0.Country,
					ASN:       r.IPInfo.Ping0.ASN,
					Org:       r.IPInfo.Ping0.Org,
					IsIDC:     r.IPInfo.Ping0.IsIDC,
					RiskScore: r.IPInfo.Ping0.RiskScore,
				}
			}
			var ipr *history.IPPureInfo
			if r.IPInfo.IPPure != nil {
				ipr = &history.IPPureInfo{
					IP:            r.IPInfo.IPPure.IP,
					ASN:           r.IPInfo.IPPure.ASN,
					ASOrg:         r.IPInfo.IPPure.ASOrg,
					Country:       r.IPInfo.IPPure.Country,
					CountryCode:   r.IPInfo.IPPure.CountryCode,
					City:          r.IPInfo.IPPure.City,
					FraudScore:    r.IPInfo.IPPure.FraudScore,
					IsResidential: r.IPInfo.IPPure.IsResidential,
					IsBroadcast:   r.IPInfo.IPPure.IsBroadcast,
				}
			}
			return &history.IPInfo{
				IP:          r.IPInfo.IP,
				ASN:         r.IPInfo.ASN,
				ISP:         r.IPInfo.ISP,
				IPType:      r.IPInfo.IPType,
				OriginType:  r.IPInfo.OriginType,
				RiskScore:   r.IPInfo.RiskScore,
				FraudScore:  r.IPInfo.FraudScore,
				Location:    r.IPInfo.Location,
				Country:     r.IPInfo.Country,
				CountryCode: r.IPInfo.CountryCode,
				Ping0:       p0,
				IPPure:      ipr,
			}
		}(),
		Stability: func() *history.StabilityInfo {
			if r.Stability == nil {
				return nil
			}
			return &history.StabilityInfo{
				TotalProbes:   r.Stability.TotalProbes,
				SuccessProbes: r.Stability.SuccessProbes,
				StabilityRate: r.Stability.StabilityRate,
				Flapping:      r.Stability.Flapping,
				ExitIPs:       append([]string{}, r.Stability.ExitIPs...),
				BlockedIPs:    append([]string{}, r.Stability.BlockedIPs...),
				FlapReason:    r.Stability.FlapReason,
				GoogleTTFBMs:  r.Stability.GoogleTTFBMs,
				GoogleMinTTFB: r.Stability.GoogleMinTTFB,
				GoogleMaxTTFB: r.Stability.GoogleMaxTTFB,
				LatencyGrade:  r.Stability.LatencyGrade,
			}
		}(),
	}
}

func parseMetricSlice(metrics []string) speedtester.MetricSet {
	if len(metrics) == 0 {
		return speedtester.MetricSet{
			Latency:     true,
			Download:    true,
			Antigravity: true,
		}
	}
	set := speedtester.MetricSet{}
	for _, m := range metrics {
		switch m {
		case "latency":
			set.Latency = true
		case "download":
			set.Download = true
		case "upload":
			set.Upload = true
		case "antigravity":
			set.Antigravity = true
		}
	}
	return set
}

package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/faceair/clash-speedtest/adapter/controller/mihomo"
	"github.com/faceair/clash-speedtest/core/auth"
	"github.com/faceair/clash-speedtest/core/controller"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/policy"
	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/speedtester"
	"gopkg.in/yaml.v2"
)

// AppService orchestrates domain operations across speedtesting, history analysis,
// profile subscriptions, external proxy controllers, and auto-switch decision policies.
type AppService struct {
	historyStore      *history.Store
	profilePaths      profiles.Paths
	emitter           EventEmitter
	cancelFunc        context.CancelFunc
	cancelCtx         context.Context
	mu                sync.Mutex
	status            TestStatus
	currentRunID      string
	stoppedByUser     atomic.Bool
	antigravityToken  string
	antigravitySource string
	antigravityMu     sync.RWMutex

	// External Controller & Decision Policy fields
	ctrlMu         sync.RWMutex
	controller     controller.Controller
	controllerCfg  ControllerConfigDTO
	policy         policy.SwitchPolicy
	decisionEngine *policy.DecisionEngine
	decisionState  policy.DecisionState
}

// NewAppService creates a new application service instance.
func NewAppService(hStore *history.Store, paths profiles.Paths, emitter EventEmitter) *AppService {
	if emitter == nil {
		emitter = NewMemoryEventEmitter()
	}
	svc := &AppService{
		historyStore:   hStore,
		profilePaths:   paths,
		emitter:        emitter,
		decisionEngine: policy.NewDecisionEngine(),
		policy:         policy.DefaultSwitchPolicy(),
		controllerCfg: ControllerConfigDTO{
			Endpoint: "http://127.0.0.1:9090",
			Mode:     "external",
		},
	}

	// Initialize default Mihomo controller adapter
	if client, err := mihomo.NewClient(mihomo.Config{
		Endpoint: svc.controllerCfg.Endpoint,
		Secret:   svc.controllerCfg.Secret,
	}); err == nil {
		svc.controller = client
	}

	// Auto-detect Antigravity token on startup
	if token, src, err := auth.TryAutoDetectAntigravityToken(); err == nil && token != "" {
		svc.antigravityToken = token
		svc.antigravitySource = src
	}

	return svc
}

// Emitter returns the configured EventEmitter.
func (s *AppService) Emitter() EventEmitter {
	return s.emitter
}

// Status returns a copy of the current active test status.
func (s *AppService) Status() TestStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Stop interrupts any running test task.
func (s *AppService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelFunc != nil {
		s.stoppedByUser.Store(true)
		s.cancelFunc()
		s.cancelFunc = nil
	}
	s.status.IsRunning = false
	s.emitter.Emit(Event{
		Type: "test_stopped",
		Payload: map[string]any{
			"message": "测试已被用户中断",
		},
	})
}

// StartBatch starts a batch speed test across specified or all nodes of an airport.
func (s *AppService) StartBatch(req BatchTestRequest, explicitToken string) error {
	s.mu.Lock()
	if s.status.IsRunning {
		s.mu.Unlock()
		return fmt.Errorf("已有测速任务在运行中")
	}

	airportStore, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("加载机场配置失败: %w", err)
	}

	airport := airportStore.Get(req.AirportID)
	if airport == nil {
		s.mu.Unlock()
		return fmt.Errorf("未找到指定机场")
	}

	cacheFile := s.profilePaths.CacheFile(airport.ID)
	if !s.profilePaths.HasCache(airport.ID) {
		s.mu.Unlock()
		return fmt.Errorf("机场节点缓存不存在，请先刷新节点")
	}

	metrics := ParseMetricSlice(req.Config.Metrics)
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

	token := explicitToken
	if token == "" {
		token = s.GetAntigravityToken()
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
		Rounds:           req.Config.Rounds,
	}

	st, err := speedtester.New(stConfig)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("初始化测速引擎失败: %w", err)
	}

	allProxies, err := st.LoadProxies()
	if err != nil {
		s.mu.Unlock()
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
		s.mu.Unlock()
		return fmt.Errorf("没有选中的可测试节点")
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancelCtx = ctx
	s.cancelFunc = cancel
	s.stoppedByUser.Store(false)
	s.status = TestStatus{
		IsRunning:    true,
		TotalNodes:   len(targetProxies),
		AirportName:  airport.Name,
		StartedAt:    time.Now(),
		CurrentIndex: 0,
		Percent:      0,
	}
	s.mu.Unlock()

	s.emitter.Emit(Event{
		Type: "test_started",
		Payload: map[string]any{
			"airport_name": airport.Name,
			"total_nodes":  len(targetProxies),
			"metrics":      req.Config.Metrics,
		},
	})

	go s.runBatchTest(ctx, st, targetProxies, airport.ID, airport.Name, req.Config.Metrics)
	return nil
}

func (s *AppService) runBatchTest(ctx context.Context, st *speedtester.SpeedTester, proxies map[string]*speedtester.CProxy, airportID, airportName string, metrics []string) {
	defer func() {
		s.mu.Lock()
		s.status.IsRunning = false
		s.cancelFunc = nil
		s.mu.Unlock()
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
						nodeResult := ConvertResult(res)

						mapMu.Lock()
						if _, exists := resultsMap[res.ProxyName]; !exists {
							currentIndex++
						}
						resultsMap[res.ProxyName] = nodeResult
						currIdx := currentIndex
						mapMu.Unlock()

						s.mu.Lock()
						s.status.CurrentIndex = currIdx
						s.status.CurrentNode = res.ProxyName
						if total > 0 {
							s.status.Percent = int(math.Min(100, float64(currIdx*100)/float64(total)))
						}
						pct := s.status.Percent
						s.mu.Unlock()

						s.emitter.Emit(Event{
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
		st.TestProxiesUntil(proxies, func(res *speedtester.Result) bool {
			select {
			case <-ctx.Done():
				return false
			default:
			}

			nodeResult := ConvertResult(res)

			mapMu.Lock()
			if _, exists := resultsMap[res.ProxyName]; !exists {
				currentIndex++
			}
			resultsMap[res.ProxyName] = nodeResult
			currIdx := currentIndex
			mapMu.Unlock()

			s.mu.Lock()
			s.status.CurrentIndex = currIdx
			s.status.CurrentNode = res.ProxyName
			if total > 0 {
				s.status.Percent = int(math.Min(100, float64(currIdx*100)/float64(total)))
			}
			pct := s.status.Percent
			s.mu.Unlock()

			s.emitter.Emit(Event{
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

	if s.stoppedByUser.Load() {
		return
	}

	run := &history.TestRun{
		AirportID:   airportID,
		AirportName: airportName,
		CreatedAt:   time.Now(),
		Metrics:     metrics,
		TotalNodes:  total,
		PassedNodes: len(results),
		Results:     results,
	}

	runID, err := s.historyStore.Save(run)
	if err != nil {
		log.Printf("保存历史记录失败: %s", err)
	}

	allRuns, _ := s.historyStore.GetAllRuns()
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

	s.emitter.Emit(Event{
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

// TestSingle executes a single node test synchronously.
func (s *AppService) TestSingle(req SingleTestRequest, token string) (*history.RunNodeResult, error) {
	airportStore, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return nil, fmt.Errorf("加载机场配置失败: %w", err)
	}

	airport := airportStore.Get(req.AirportID)
	if airport == nil {
		return nil, fmt.Errorf("未找到指定机场")
	}

	cacheFile := s.profilePaths.CacheFile(airport.ID)
	if !s.profilePaths.HasCache(airport.ID) {
		return nil, fmt.Errorf("机场节点缓存不存在")
	}

	metrics := ParseMetricSlice(req.Config.Metrics)
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

	if token == "" {
		token = s.GetAntigravityToken()
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

	s.emitter.Emit(Event{
		Type: "single_test_started",
		Payload: map[string]any{
			"node_name": req.NodeName,
		},
	})

	var lastRes *history.RunNodeResult
	st.TestSingle(req.NodeName, proxy, func(r *speedtester.Result) bool {
		lastRes = ConvertResult(r)
		s.emitter.Emit(Event{
			Type: "single_node_progress",
			Payload: map[string]any{
				"result": lastRes,
			},
		})
		return true
	})

	s.emitter.Emit(Event{
		Type: "single_test_completed",
		Payload: map[string]any{
			"result": lastRes,
		},
	})

	return lastRes, nil
}

// ConvertResult maps domain speedtester.Result into persistent history.RunNodeResult.
func ConvertResult(r *speedtester.Result) *history.RunNodeResult {
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

// ParseMetricSlice converts slice of metric names into domain MetricSet.
func ParseMetricSlice(metrics []string) speedtester.MetricSet {
	if len(metrics) == 0 {
		return speedtester.MetricSet{
			Latency:  true,
			Download: true,
		}
	}
	set := speedtester.MetricSet{}
	for _, m := range metrics {
		switch strings.ToLower(m) {
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

// History operations

func (s *AppService) ListHistory() ([]*history.RunSummary, error) {
	return s.historyStore.List()
}

func (s *AppService) GetHistory(id string) (*history.TestRun, error) {
	return s.historyStore.Get(id)
}

func (s *AppService) DeleteHistory(id string) error {
	return s.historyStore.Delete(id)
}

func (s *AppService) GetAirportTimeline(airportID string) (*history.AirportHistory, error) {
	return s.historyStore.GetAirportHistory(airportID)
}

func (s *AppService) CompareRuns(baseID, targetID string) (*history.RunComparison, error) {
	return s.historyStore.Compare(baseID, targetID)
}

// Airport & Profile operations

// Airport & Profile operations

func (s *AppService) ListAirports() ([]AirportDTO, error) {
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return nil, err
	}

	if len(store.Airports) == 0 {
		if s.profilePaths.ImportLegacyIfEmpty(store) {
			_ = profiles.SaveStore(s.profilePaths.StoreFile(), store)
		}
	}

	dtos := make([]AirportDTO, 0, len(store.Airports))
	for _, ap := range store.Airports {
		hasCache := s.profilePaths.HasCache(ap.ID)
		nodeCount := 0
		if hasCache {
			nodeCount = s.CountCachedNodes(ap.ID)
		}
		dtos = append(dtos, AirportDTO{
			ID:        ap.ID,
			Name:      ap.Name,
			URL:       ap.URL,
			UpdatedAt: ap.UpdatedAt,
			NodeCount: nodeCount,
			HasCache:  hasCache,
		})
	}
	return dtos, nil
}

func (s *AppService) CountCachedNodes(airportID string) int {
	cacheFile := s.profilePaths.CacheFile(airportID)
	st, err := speedtester.New(&speedtester.Config{
		ConfigPaths: cacheFile,
		Mode:        speedtester.SpeedModeFast,
	})
	if err != nil {
		return 0
	}
	proxies, err := st.LoadProxies()
	if err != nil {
		return 0
	}
	return len(proxies)
}

func (s *AppService) CreateAirport(name, rawURL, userAgent string) (*AirportDTO, error) {
	name = strings.TrimSpace(name)
	rawURL = strings.TrimSpace(rawURL)
	if name == "" || rawURL == "" {
		return nil, fmt.Errorf("机场名称和订阅链接不能为空")
	}

	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return nil, err
	}

	ap := &profiles.Airport{
		ID:   fmt.Sprintf("%d", time.Now().UnixNano()),
		Name: name,
		URL:  rawURL,
	}

	if profiles.IsHTTPURL(ap.URL) {
		body, err := profiles.FetchSubscription(ap.URL, userAgent)
		if err != nil {
			return nil, fmt.Errorf("获取订阅节点失败: %w", err)
		}
		if err := s.profilePaths.WriteCache(ap.ID, body); err != nil {
			return nil, fmt.Errorf("写入节点缓存失败: %w", err)
		}
		ap.UpdatedAt = time.Now()
	} else {
		expanded := profiles.ExpandLocalPath(ap.URL)
		data, err := os.ReadFile(expanded)
		if err != nil {
			return nil, fmt.Errorf("读取本地节点文件失败: %w", err)
		}
		if err := s.profilePaths.WriteCache(ap.ID, data); err != nil {
			return nil, fmt.Errorf("写入节点缓存失败: %w", err)
		}
		ap.UpdatedAt = time.Now()
	}

	store.Add(ap)
	if err := profiles.SaveStore(s.profilePaths.StoreFile(), store); err != nil {
		return nil, err
	}

	nodeCount := s.CountCachedNodes(ap.ID)
	return &AirportDTO{
		ID:        ap.ID,
		Name:      ap.Name,
		URL:       ap.URL,
		UpdatedAt: ap.UpdatedAt,
		NodeCount: nodeCount,
		HasCache:  true,
	}, nil
}

func (s *AppService) UpdateAirport(id, name, rawURL, userAgent string) (*AirportDTO, error) {
	name = strings.TrimSpace(name)
	rawURL = strings.TrimSpace(rawURL)
	if name == "" || rawURL == "" {
		return nil, fmt.Errorf("机场名称和订阅链接不能为空")
	}

	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return nil, err
	}

	ap := store.Get(id)
	if ap == nil {
		return nil, fmt.Errorf("未找到指定机场")
	}

	urlChanged := ap.URL != rawURL
	ap.Name = name
	ap.URL = rawURL

	if urlChanged || !s.profilePaths.HasCache(ap.ID) {
		if profiles.IsHTTPURL(ap.URL) {
			body, err := profiles.FetchSubscription(ap.URL, userAgent)
			if err != nil {
				return nil, fmt.Errorf("获取订阅节点失败: %w", err)
			}
			if err := s.profilePaths.WriteCache(ap.ID, body); err != nil {
				return nil, fmt.Errorf("写入节点缓存失败: %w", err)
			}
			ap.UpdatedAt = time.Now()
		} else {
			expanded := profiles.ExpandLocalPath(ap.URL)
			data, err := os.ReadFile(expanded)
			if err != nil {
				return nil, fmt.Errorf("读取本地节点文件失败: %w", err)
			}
			if err := s.profilePaths.WriteCache(ap.ID, data); err != nil {
				return nil, fmt.Errorf("写入节点缓存失败: %w", err)
			}
			ap.UpdatedAt = time.Now()
		}
	}

	if err := profiles.SaveStore(s.profilePaths.StoreFile(), store); err != nil {
		return nil, err
	}

	nodeCount := s.CountCachedNodes(ap.ID)
	return &AirportDTO{
		ID:        ap.ID,
		Name:      ap.Name,
		URL:       ap.URL,
		UpdatedAt: ap.UpdatedAt,
		NodeCount: nodeCount,
		HasCache:  s.profilePaths.HasCache(ap.ID),
	}, nil
}

func (s *AppService) DeleteAirport(id string) error {
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return err
	}
	removed := store.Remove(id)
	if removed == nil {
		return fmt.Errorf("未找到指定机场")
	}
	_ = os.Remove(s.profilePaths.CacheFile(id))
	return profiles.SaveStore(s.profilePaths.StoreFile(), store)
}

func (s *AppService) RefreshAirport(id, userAgent string) (*AirportDTO, error) {
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return nil, err
	}
	ap := store.Get(id)
	if ap == nil {
		return nil, fmt.Errorf("未找到指定机场")
	}

	if profiles.IsHTTPURL(ap.URL) {
		body, err := profiles.FetchSubscription(ap.URL, userAgent)
		if err != nil {
			return nil, fmt.Errorf("刷新订阅节点失败: %w", err)
		}
		if err := s.profilePaths.WriteCache(ap.ID, body); err != nil {
			return nil, fmt.Errorf("写入节点缓存失败: %w", err)
		}
	} else {
		expanded := profiles.ExpandLocalPath(ap.URL)
		data, err := os.ReadFile(expanded)
		if err != nil {
			return nil, fmt.Errorf("读取本地节点文件失败: %w", err)
		}
		if err := s.profilePaths.WriteCache(ap.ID, data); err != nil {
			return nil, fmt.Errorf("写入节点缓存失败: %w", err)
		}
	}
	ap.UpdatedAt = time.Now()
	_ = profiles.SaveStore(s.profilePaths.StoreFile(), store)

	nodeCount := s.CountCachedNodes(ap.ID)
	return &AirportDTO{
		ID:        ap.ID,
		Name:      ap.Name,
		URL:       ap.URL,
		UpdatedAt: ap.UpdatedAt,
		NodeCount: nodeCount,
		HasCache:  true,
	}, nil
}

func (s *AppService) GetAirportNodes(airportID string) ([]map[string]any, error) {
	airportStore, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return nil, err
	}
	airport := airportStore.Get(airportID)
	if airport == nil {
		return nil, fmt.Errorf("未找到指定机场")
	}

	cacheFile := s.profilePaths.CacheFile(airport.ID)
	if !s.profilePaths.HasCache(airport.ID) {
		return nil, fmt.Errorf("机场节点缓存不存在，请先刷新节点")
	}

	st, err := speedtester.New(&speedtester.Config{
		ConfigPaths: cacheFile,
		Mode:        speedtester.SpeedModeFast,
	})
	if err != nil {
		return nil, fmt.Errorf("init speedtester: %w", err)
	}

	proxies, err := st.LoadProxies()
	if err != nil {
		return nil, fmt.Errorf("load proxies: %w", err)
	}

	res := make([]map[string]any, 0, len(proxies))
	for name, p := range proxies {
		code := profiles.DetectCountry(name)
		flag := profiles.FlagFromCode(code)
		res = append(res, map[string]any{
			"name":         name,
			"type":         p.Type().String(),
			"country_code": code,
			"country_flag": flag,
		})
	}

	sort.Slice(res, func(i, j int) bool {
		return res[i]["name"].(string) < res[j]["name"].(string)
	})

	return res, nil
}

// Antigravity Token & OAuth operations

func (s *AppService) GetAntigravityToken() string {
	s.antigravityMu.RLock()
	defer s.antigravityMu.RUnlock()
	return s.antigravityToken
}

func (s *AppService) GetAntigravitySource() string {
	s.antigravityMu.RLock()
	defer s.antigravityMu.RUnlock()
	return s.antigravitySource
}

func (s *AppService) SetAntigravityToken(token, source string) {
	s.antigravityMu.Lock()
	s.antigravityToken = token
	s.antigravitySource = source
	s.antigravityMu.Unlock()

	s.emitter.Emit(Event{
		Type: "antigravity_token_updated",
		Payload: map[string]any{
			"source": source,
		},
	})
}

func (s *AppService) GetTokenStatus() TokenStatusDTO {
	s.antigravityMu.Lock()
	defer s.antigravityMu.Unlock()

	if s.antigravityToken == "" {
		if tok, src, err := auth.TryAutoDetectAntigravityToken(); err == nil && tok != "" {
			s.antigravityToken = tok
			s.antigravitySource = src
		}
	}

	hasToken := s.antigravityToken != ""
	preview := ""
	if hasToken {
		if len(s.antigravityToken) > 12 {
			preview = s.antigravityToken[:6] + "..." + s.antigravityToken[len(s.antigravityToken)-4:]
		} else {
			preview = "***"
		}
	}

	return TokenStatusDTO{
		HasToken: hasToken,
		Source:   s.antigravitySource,
		Preview:  preview,
	}
}

// Config Export operations

func (s *AppService) ExportClashConfig(airportID string, nodeNames []string) ([]byte, error) {
	airportStore, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return nil, fmt.Errorf("load airport store: %w", err)
	}
	airport := airportStore.Get(airportID)
	if airport == nil {
		return nil, fmt.Errorf("airport not found: %s", airportID)
	}

	cacheFile := s.profilePaths.CacheFile(airport.ID)
	if !s.profilePaths.HasCache(airport.ID) {
		return nil, fmt.Errorf("airport cache not found")
	}

	st, err := speedtester.New(&speedtester.Config{
		ConfigPaths: cacheFile,
		Mode:        speedtester.SpeedModeFast,
	})
	if err != nil {
		return nil, err
	}

	proxies, err := st.LoadProxies()
	if err != nil {
		return nil, err
	}

	nameSet := make(map[string]struct{}, len(nodeNames))
	for _, name := range nodeNames {
		nameSet[name] = struct{}{}
	}

	var matched []map[string]any
	for name, p := range proxies {
		if len(nodeNames) == 0 {
			matched = append(matched, map[string]any{"name": name, "type": p.Type().String()})
		} else if _, ok := nameSet[name]; ok {
			matched = append(matched, map[string]any{"name": name, "type": p.Type().String()})
		}
	}

	return ExportClashYAML(matched)
}

// ExportClashConfigFromResults creates Clash YAML from node results of a test run.
func (s *AppService) ExportClashConfigFromResults(runID string, nodeNames []string) ([]byte, error) {
	run, err := s.historyStore.Get(runID)
	if err != nil {
		return nil, fmt.Errorf("get run %s: %w", runID, err)
	}

	cacheFile := s.profilePaths.CacheFile(run.AirportID)

	st, err := speedtester.New(&speedtester.Config{
		ConfigPaths: cacheFile,
		Mode:        speedtester.SpeedModeFast,
	})
	if err != nil {
		return nil, err
	}
	proxies, err := st.LoadProxies()
	if err != nil {
		return nil, err
	}

	nameSet := make(map[string]struct{}, len(nodeNames))
	for _, n := range nodeNames {
		nameSet[n] = struct{}{}
	}

	var matched []map[string]any
	for _, r := range run.Results {
		if len(nodeNames) > 0 {
			if _, ok := nameSet[r.ProxyName]; !ok {
				continue
			}
		}
		if p, ok := proxies[r.ProxyName]; ok {
			matched = append(matched, map[string]any{"name": r.ProxyName, "type": p.Type().String()})
		}
	}

	return ExportClashYAML(matched)
}

// ExportClashYAML formats proxy configurations into a Clash YAML structure.
func ExportClashYAML(proxies []map[string]any) ([]byte, error) {
	config := &speedtester.RawConfig{
		Proxies: proxies,
	}
	return yaml.Marshal(config)
}

// Settings operations

func defaultSettingsPath() string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		return filepath.Join(home, ".clash-speedtest", "settings.json")
	}
	return filepath.Join(".", ".clash-speedtest", "settings.json")
}

func (s *AppService) GetSettings() (*AppSettings, error) {
	p := defaultSettingsPath()
	data, err := os.ReadFile(p)
	if err != nil {
		return &AppSettings{
			PreferredBrowser: "default",
		}, nil
	}

	var settings AppSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return &AppSettings{PreferredBrowser: "default"}, nil
	}
	if settings.PreferredBrowser == "" {
		settings.PreferredBrowser = "default"
	}
	return &settings, nil
}

func (s *AppService) SaveSettings(settings *AppSettings) error {
	if settings == nil {
		return nil
	}
	p := defaultSettingsPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// --- Controller & Smart Orchestrator Methods ---

// SetController allows directly setting a controller instance (useful for tests or custom adapters).
func (s *AppService) SetController(ctrl controller.Controller, cfg ControllerConfigDTO) {
	s.ctrlMu.Lock()
	defer s.ctrlMu.Unlock()
	s.controller = ctrl
	s.controllerCfg = cfg
}

// ConfigureController reconfigures the external controller connection.
func (s *AppService) ConfigureController(ctx context.Context, cfg ControllerConfigDTO) error {
	s.ctrlMu.Lock()
	defer s.ctrlMu.Unlock()

	client, err := mihomo.NewClient(mihomo.Config{
		Endpoint:                     cfg.Endpoint,
		Secret:                       cfg.Secret,
		AllowRemote:                  cfg.AllowRemote,
		AllowInsecurePlaintextRemote: cfg.AllowInsecurePlaintextRemote,
	})
	if err != nil {
		return fmt.Errorf("create controller client: %w", err)
	}

	s.controller = client
	s.controllerCfg = cfg

	s.emitter.Emit(Event{
		Type: "controller_status_changed",
		Payload: map[string]any{
			"endpoint": cfg.Endpoint,
			"mode":     cfg.Mode,
		},
	})
	return nil
}

// GetControllerStatus returns the connection and operational status of the controller.
func (s *AppService) GetControllerStatus(ctx context.Context) (ControllerStatusDTO, error) {
	s.ctrlMu.RLock()
	ctrl := s.controller
	cfg := s.controllerCfg
	pol := s.policy
	state := s.decisionState
	s.ctrlMu.RUnlock()

	status := ControllerStatusDTO{
		Connected:       false,
		Endpoint:        cfg.Endpoint,
		CurrentGroup:    pol.TargetGroup,
		CurrentNode:     state.CurrentNode,
		LockedNode:      pol.LockedNode,
		Mode:            string(pol.Mode),
		HasSecret:       cfg.Secret != "",
	}

	if ctrl == nil {
		return status, nil
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	v, err := ctrl.GetVersion(ctxTimeout)
	if err != nil {
		return status, nil
	}

	status.Connected = true
	status.CoreVersion = v.Version
	status.CoreType = v.CoreType

	groups, err := ctrl.ListGroups(ctxTimeout)
	if err == nil {
		for _, g := range groups {
			status.AvailableGroups = append(status.AvailableGroups, g.Name)
			if pol.TargetGroup != "" && g.Name == pol.TargetGroup {
				status.CurrentNode = g.Now
				s.ctrlMu.Lock()
				s.decisionState.CurrentNode = g.Now
				s.ctrlMu.Unlock()
			}
		}
	}

	return status, nil
}

// ListControllerGroups lists all selector/proxy groups from the external core.
func (s *AppService) ListControllerGroups(ctx context.Context) ([]controller.Group, error) {
	s.ctrlMu.RLock()
	ctrl := s.controller
	s.ctrlMu.RUnlock()

	if ctrl == nil {
		return nil, fmt.Errorf("external controller is not initialized")
	}
	return ctrl.ListGroups(ctx)
}

// SelectControllerNode explicitly switches the active proxy node for a group.
func (s *AppService) SelectControllerNode(ctx context.Context, group string, nodeName string) error {
	s.ctrlMu.Lock()
	ctrl := s.controller
	pol := s.policy
	fromNode := s.decisionState.CurrentNode
	s.ctrlMu.Unlock()

	if ctrl == nil {
		return fmt.Errorf("external controller is not initialized")
	}

	if group == "" {
		group = pol.TargetGroup
	}
	if group == "" {
		group = "PROXY"
	}

	if err := ctrl.SelectNode(ctx, group, nodeName); err != nil {
		s.emitter.Emit(Event{
			Type: "controller_switch_failed",
			Payload: map[string]any{
				"group":     group,
				"from_node": fromNode,
				"to_node":   nodeName,
				"error":     err.Error(),
			},
		})
		return err
	}

	s.ctrlMu.Lock()
	s.decisionEngine.RecordSwitch(&s.decisionState, fromNode, nodeName, group, "手动在控制台切换", "manual_override", 0, 0)
	s.ctrlMu.Unlock()

	s.emitter.Emit(Event{
		Type: "controller_node_switched",
		Payload: map[string]any{
			"group":        group,
			"from_node":    fromNode,
			"to_node":      nodeName,
			"trigger_type": "manual_override",
			"timestamp":    time.Now(),
		},
	})
	return nil
}

// GetSwitchPolicy returns the active auto-switch policy.
func (s *AppService) GetSwitchPolicy(ctx context.Context) (policy.SwitchPolicy, error) {
	s.ctrlMu.RLock()
	defer s.ctrlMu.RUnlock()
	return s.policy, nil
}

// UpdateSwitchPolicy updates the auto-switch policy.
func (s *AppService) UpdateSwitchPolicy(ctx context.Context, p policy.SwitchPolicy) error {
	s.ctrlMu.Lock()
	defer s.ctrlMu.Unlock()
	s.policy = p

	s.emitter.Emit(Event{
		Type:    "controller_policy_updated",
		Payload: p,
	})
	return nil
}

// GetSwitchAuditTrail returns recent switch events.
func (s *AppService) GetSwitchAuditTrail(ctx context.Context) ([]policy.SwitchEvent, error) {
	s.ctrlMu.RLock()
	defer s.ctrlMu.RUnlock()
	return s.decisionState.AuditTrail, nil
}

// EvaluateAndAutoSwitch evaluates latest probe results against policy, checks freshness,
// and executes switch, recommendation, or resilient verification with rollback.
func (s *AppService) EvaluateAndAutoSwitch(ctx context.Context, evals []policy.NodeEvaluation) (policy.DecisionResult, error) {
	s.ctrlMu.Lock()
	pol := s.policy
	engine := s.decisionEngine
	ctrl := s.controller
	res := engine.Evaluate(time.Now(), pol, &s.decisionState, evals)
	s.ctrlMu.Unlock()

	// 1. If candidate data is stale or insufficient, notify clients that fresh probes are needed
	if res.RequiresFreshProbe && len(res.StaleNodes) > 0 {
		s.emitter.Emit(Event{
			Type: "controller_fresh_probe_required",
			Payload: map[string]any{
				"stale_nodes": res.StaleNodes,
				"reason":      "候选节点样本过旧或样本数不足，需轻量复测后方可参评",
			},
		})
	}

	// 2. If in Recommend mode and a recommendation was produced, emit recommendation event
	if res.Recommendation != nil {
		s.emitter.Emit(Event{
			Type:    "controller_switch_recommended",
			Payload: res.Recommendation,
		})
		return res, nil
	}

	// 3. If ShouldSwitch is false, nothing to execute
	if !res.ShouldSwitch || res.TargetNode == "" {
		return res, nil
	}

	if ctrl == nil {
		return res, fmt.Errorf("controller not initialized, cannot execute switch to %s", res.TargetNode)
	}

	group := pol.TargetGroup
	if group == "" {
		group = "PROXY"
	}

	s.ctrlMu.Lock()
	fromNode := s.decisionState.CurrentNode
	s.ctrlMu.Unlock()

	// Execute switch in external proxy core
	if err := ctrl.SelectNode(ctx, group, res.TargetNode); err != nil {
		s.emitter.Emit(Event{
			Type: "controller_switch_failed",
			Payload: map[string]any{
				"group":     group,
				"from_node": fromNode,
				"to_node":   res.TargetNode,
				"reason":    res.Reason,
				"error":     err.Error(),
			},
		})
		return res, fmt.Errorf("execute switch: %w", err)
	}

	// Record switch in state
	s.ctrlMu.Lock()
	s.decisionEngine.RecordSwitch(&s.decisionState, fromNode, res.TargetNode, group, res.Reason, res.TriggerType, 0, 0)
	s.ctrlMu.Unlock()

	s.emitter.Emit(Event{
		Type: "controller_node_switched",
		Payload: map[string]any{
			"group":        group,
			"from_node":    fromNode,
			"to_node":      res.TargetNode,
			"reason":       res.Reason,
			"trigger_type": res.TriggerType,
			"timestamp":    time.Now(),
		},
	})

	// 4. Post-Switch Verification & Resilient Rollback:
	// If RollbackOnFailure is enabled, run multiple lightweight verification probes.
	// A single transient probe failure will NOT cause an immediate rollback.
	// Rollback is executed ONLY if majority fail (failureCount >= threshold).
	if pol.RollbackOnFailure && fromNode != "" && fromNode != res.TargetNode {
		probeCount := pol.VerificationProbeCount
		if probeCount <= 0 {
			probeCount = 3
		}
		threshold := pol.VerificationFailureThreshold
		if threshold <= 0 {
			threshold = 2
		}

		var verificationProbes []policy.VerificationProbe
		for i := 1; i <= probeCount; i++ {
			select {
			case <-ctx.Done():
				return res, ctx.Err()
			default:
			}

			d, err := ctrl.TestDelay(ctx, res.TargetNode, "", 2*time.Second)
			if err != nil {
				verificationProbes = append(verificationProbes, policy.VerificationProbe{
					Index:   i,
					Success: false,
					Error:   err.Error(),
				})
			} else {
				verificationProbes = append(verificationProbes, policy.VerificationProbe{
					Index:   i,
					Success: true,
					RTT:     d,
				})
			}
		}

		verifResult := engine.EvaluateVerification(verificationProbes, false, threshold, pol.Purpose)
		if verifResult.ShouldRollback {
			// Rollback: update Selector selection back to previous node in external core.
			// By default this preserves existing connections without terminating them.
			_ = ctrl.SelectNode(ctx, group, fromNode)

			s.ctrlMu.Lock()
			s.decisionEngine.RecordRollback(&s.decisionState, group, res.TargetNode, verifResult.Reason)
			s.ctrlMu.Unlock()

			s.emitter.Emit(Event{
				Type: "controller_node_rolled_back",
				Payload: map[string]any{
					"group":         group,
					"rolled_back_to": fromNode,
					"failed_node":   res.TargetNode,
					"reason":        verifResult.Reason,
					"probes":        verificationProbes,
					"timestamp":     time.Now(),
				},
			})
		}
	}

	return res, nil
}



package application

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/faceair/clash-speedtest/core/appdata"
	"github.com/faceair/clash-speedtest/core/auth"
	"github.com/faceair/clash-speedtest/core/controller"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
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
	appPaths          appdata.AppPaths
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

	// 24/7 Monitor subsystem fields
	monitorMu         sync.RWMutex
	monitorSchedulers map[string]*monitor.Scheduler
	monitorRunner     *monitor.Runner
	monitorLoadErr    error

	// latencyPersistenceWG keeps the result-first workbench path from closing
	// history.db while a just-finished latency result is still being persisted.
	latencyPersistenceWG sync.WaitGroup
	// latencySaveHook is test-only dependency injection for slow/failing-save
	// verification. Production uses historyStore.SaveLatencyTest directly.
	latencySaveHook func(context.Context, *history.LatencyTest) error
}

// NewAppService creates a new application service instance.
func NewAppService(hStore *history.Store, paths profiles.Paths, emitter EventEmitter) *AppService {
	return newAppService(hStore, appdata.FromLegacy(paths.Dir, ""), paths, emitter)
}

// NewAppServiceWithPaths creates the production service from the shared path
// decision used by both the Web and Wails adapters.
func NewAppServiceWithPaths(hStore *history.Store, paths appdata.AppPaths, emitter EventEmitter) *AppService {
	profilePaths := profiles.Paths{Dir: paths.ProfileDir}
	return newAppService(hStore, paths, profilePaths, emitter)
}

func newAppService(hStore *history.Store, appPaths appdata.AppPaths, profilePaths profiles.Paths, emitter EventEmitter) *AppService {
	if emitter == nil {
		emitter = NewMemoryEventEmitter()
	}
	svc := &AppService{
		historyStore:      hStore,
		profilePaths:      profilePaths,
		appPaths:          appPaths,
		emitter:           emitter,
		decisionEngine:    policy.NewDecisionEngine(),
		policy:            policy.DefaultSwitchPolicy(),
		monitorSchedulers: make(map[string]*monitor.Scheduler),
		controllerCfg: ControllerConfigDTO{
			Endpoint: "http://127.0.0.1:9090",
			Mode:     "external",
		},
	}

	if hStore != nil {
		svc.monitorRunner = monitor.NewRunner(monitor.RunnerConfig{
			Store: hStore,
		})
		if err := svc.loadPersistedMonitorJobs(); err != nil {
			svc.monitorLoadErr = err
			log.Printf("monitor job definitions were not loaded: %v", err)
		}
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

// HistoryStore returns the configured history Store.
func (s *AppService) HistoryStore() *history.Store {
	return s.historyStore
}

// Status returns a copy of the current active test status.
func (s *AppService) Status() TestStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Stop interrupts any running test task and stops background monitor jobs.
func (s *AppService) Stop() {
	s.mu.Lock()
	if s.cancelFunc != nil {
		s.stoppedByUser.Store(true)
		s.cancelFunc()
		s.cancelFunc = nil
	}
	s.status.IsRunning = false
	s.mu.Unlock()

	s.StopAllMonitorJobs()

	s.emitter.Emit(Event{
		Type: "test_stopped",
		Payload: map[string]any{
			"message": "测试已被用户中断",
		},
	})
}

// Close stops all background tasks and cleanly releases database connections.
func (s *AppService) Close() error {
	s.Stop()
	s.latencyPersistenceWG.Wait()
	if s.historyStore != nil {
		return s.historyStore.Close()
	}
	return nil
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

	go s.runBatchTest(ctx, cancel, st, targetProxies, airport.ID, airport.Name, req.Config.Metrics)
	return nil
}

func (s *AppService) runBatchTest(ctx context.Context, cancel context.CancelFunc, st *speedtester.SpeedTester, proxies map[string]*speedtester.CProxy, airportID, airportName string, metrics []string) {
	defer cancel()
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

// GetProfileSetup returns the current canonical profile state and safe source
// summaries. It does not create a store or import anything.
func (s *AppService) GetProfileSetup() (*ProfileSetupDTO, error) {
	status := s.profilePaths.InspectSetup()
	dto := &ProfileSetupDTO{
		State:             "needs_choice",
		Initialized:       status.Initialized,
		DataRoot:          s.appPaths.DataRoot,
		ProfileDir:        s.appPaths.ProfileDir,
		HistoryDir:        s.appPaths.HistoryDir,
		SettingsFile:      s.appPaths.SettingsFile,
		UnfinishedStaging: append([]string(nil), status.UnfinishedStaging...),
		LockPresent:       status.LockPresent,
	}
	if dto.DataRoot == "" && s.profilePaths.Dir != "" {
		dto.DataRoot = filepath.Dir(s.profilePaths.Dir)
	}
	if dto.ProfileDir == "" {
		dto.ProfileDir = s.profilePaths.Dir
	}
	if dto.HistoryDir == "" {
		dto.HistoryDir = history.DefaultHistoryDir()
	}
	if dto.SettingsFile == "" {
		dto.SettingsFile = defaultSettingsPath()
	}

	if status.Error != "" {
		dto.State = "error"
		dto.Error = status.Error
		return dto, nil
	}
	if status.Initialized {
		dto.State = "ready"
		dto.Initialized = true
		return dto, nil
	}

	for _, source := range profiles.DiscoverSourceCandidates() {
		dto.Sources = append(dto.Sources, profileSourceDTO(source))
	}
	return dto, nil
}

// InspectProfileSource previews an explicitly selected local source without
// importing it or touching the canonical data root.
func (s *AppService) InspectProfileSource(sourcePath string) (*ProfileSourceDTO, error) {
	source, err := profiles.InspectSource(sourcePath)
	if err != nil {
		return nil, err
	}
	dto := profileSourceDTO(source)
	dto.Label = "用户选择"
	return &dto, nil
}

// InitializeEmptyProfileStore records the explicit user's choice to start
// with a real empty canonical store.
func (s *AppService) InitializeEmptyProfileStore() error {
	return s.profilePaths.InitializeEmpty()
}

// ImportProfileSource commits only the explicitly selected local source. No
// network request or subscription refresh is performed here.
func (s *AppService) ImportProfileSource(ctx context.Context, sourcePath string) error {
	return s.profilePaths.ImportFrom(ctx, sourcePath)
}

// DiscardProfileImport removes only unfinished import staging after explicit
// user recovery action; it never removes canonical profile data.
func (s *AppService) DiscardProfileImport() error {
	return s.profilePaths.DiscardUnfinishedImport()
}

func profileSourceDTO(source profiles.SourceInfo) ProfileSourceDTO {
	return ProfileSourceDTO{
		Path:             source.Path,
		Label:            source.Label,
		Available:        source.Available,
		ProfileCount:     source.ProfileCount,
		CacheCount:       source.CacheCount,
		Missing:          append([]string(nil), source.Missing...),
		PossibleTestData: source.PossibleTestData,
		Error:            source.Error,
	}
}

func (s *AppService) ListAirports() ([]AirportDTO, error) {
	status := s.profilePaths.InspectSetup()
	if status.Error != "" {
		return nil, errors.New(status.Error)
	}
	if !status.Initialized {
		return nil, profiles.ErrProfileNotInitialized
	}
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return nil, err
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

func (s *AppService) settingsPath() string {
	if s.appPaths.SettingsFile != "" {
		return s.appPaths.SettingsFile
	}
	return defaultSettingsPath()
}

func (s *AppService) GetSettings() (*AppSettings, error) {
	p := s.settingsPath()
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
	p := s.settingsPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
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
		Connected:    false,
		Endpoint:     cfg.Endpoint,
		CurrentGroup: pol.TargetGroup,
		CurrentNode:  state.CurrentNode,
		LockedNode:   pol.LockedNode,
		Mode:         string(pol.Mode),
		HasSecret:    cfg.Secret != "",
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
	// Reject an unimplemented mode at the write boundary. Storing it would leave the
	// orchestrator holding a policy it cannot honour, and every later read would have to
	// guess what the operator meant. Empty is normalized to the safe default instead.
	mode, err := policy.NormalizeOrchestratorMode(p.Mode)
	if err != nil {
		return monitor.WrapValidationError(fmt.Errorf("invalid switch policy: %w", err))
	}
	p.Mode = mode

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
	trail := make([]policy.SwitchEvent, len(s.decisionState.AuditTrail))
	copy(trail, s.decisionState.AuditTrail)
	return trail, nil
}

// EvaluateAndAutoSwitch evaluates latest probe results against policy, checks freshness,
// and executes switch, recommendation, or resilient verification with rollback.
func (s *AppService) EvaluateAndAutoSwitch(ctx context.Context, evals []policy.NodeEvaluation) (policy.DecisionResult, error) {
	s.ctrlMu.Lock()
	pol := s.policy
	engine := s.decisionEngine
	ctrl := s.controller
	group := pol.TargetGroup
	if group == "" {
		group = "PROXY"
	}
	s.ctrlMu.Unlock()

	// Active Controller State Synchronization (B-03):
	// External Controller is the single source of truth for the active selection.
	if ctrl != nil {
		if actualCurrent, err := ctrl.GetCurrentSelection(ctx, group); err == nil && actualCurrent != "" {
			s.ctrlMu.Lock()
			if s.decisionState.CurrentNode == "" {
				// Initial startup synchronization: adopt core selection and proceed with evaluation
				s.decisionState.CurrentNode = actualCurrent
			} else if s.decisionState.CurrentNode != actualCurrent {
				// External / manual selection change detected during runtime:
				// Update internal CurrentNode and clear prior node's failure counters.
				s.decisionState.CurrentNode = actualCurrent
				s.decisionState.ConsecutiveFailures = 0
				s.ctrlMu.Unlock()

				// Suppress auto reverse switch in this round so user's manual change is not immediately hijacked back.
				return policy.DecisionResult{
					Mode:         pol.Mode,
					ShouldSwitch: false,
					Reason:       fmt.Sprintf("检测到外部或用户手动更新当前节点为 %s，本轮暂停自动调度以完成同步", actualCurrent),
				}, nil
			}
			s.ctrlMu.Unlock()
		}
	}

	s.ctrlMu.Lock()
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

	// 3. Second Hard Line of Defense (B-02):
	// Only ModeAuto is permitted to execute switches in external core.
	// Monitor Only and Recommend modes MUST NEVER execute switches under any circumstances.
	if pol.Mode != policy.ModeAuto {
		return res, nil
	}

	// 4. If ShouldSwitch is false, nothing to execute
	if !res.ShouldSwitch || res.TargetNode == "" {
		return res, nil
	}

	if ctrl == nil {
		return res, fmt.Errorf("controller not initialized, cannot execute switch to %s", res.TargetNode)
	}

	group = pol.TargetGroup
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
					"group":          group,
					"rolled_back_to": fromNode,
					"failed_node":    res.TargetNode,
					"reason":         verifResult.Reason,
					"probes":         verificationProbes,
					"timestamp":      time.Now(),
				},
			})
		}
	}

	return res, nil
}

// --- 24/7 Monitor Subsystem ---

// SetMonitorRunner injects a custom runner (useful for testing).
func (s *AppService) SetMonitorRunner(runner *monitor.Runner) {
	s.monitorMu.Lock()
	defer s.monitorMu.Unlock()
	s.monitorRunner = runner
}

// CreateMonitorJob durably commits a new product definition before exposing its
// scheduler to the rest of the process. Runtime state remains in memory only.
//
// Uniqueness (B-02):
// A JobID must be unique among registered schedulers. Re-creating a job with an existing JobID
// (regardless of running, paused, or stopped state) is rejected.
func (s *AppService) CreateMonitorJob(job monitor.MonitorJob) (*monitor.MonitorJob, error) {
	s.monitorMu.Lock()
	defer s.monitorMu.Unlock()

	if job.ID != "" {
		if _, exists := s.monitorSchedulers[job.ID]; exists {
			return nil, fmt.Errorf("monitor job %s already exists", job.ID)
		}
	} else {
		job.ID = fmt.Sprintf("job_%d", time.Now().UnixNano())
	}
	if job.Name == "" {
		job.Name = "24/7 Monitor Job"
	}
	if job.Interval <= 0 {
		job.Interval = 5 * time.Minute
	}
	if job.Timeout <= 0 {
		job.Timeout = 10 * time.Second
	}
	if job.ProbeSet == "" {
		job.ProbeSet = monitor.ProbeSetLight
	}

	// Compute NodeKeys, NodeIdentityKeys, and ConfigRevisionKeys for all nodes
	monitor.PopulateNodesKeys(job.Nodes)
	job.NodeKeys = make([]string, len(job.Nodes))
	for i, n := range job.Nodes {
		job.NodeKeys[i] = n.NodeKey
	}

	job.State = monitor.JobStateStopped
	job.BlockedReason = ""
	now := time.Now()
	job.CreatedAt = now
	job.UpdatedAt = now

	runner := s.monitorRunner
	if runner == nil {
		if s.historyStore != nil {
			runner = monitor.NewRunner(monitor.RunnerConfig{
				Store: s.historyStore,
			})
			s.monitorRunner = runner
		} else {
			return nil, fmt.Errorf("history store is not initialized")
		}
	}

	sched, err := monitor.NewScheduler(monitor.SchedulerConfig{
		Job:    &job,
		Runner: runner,
		Store:  s.historyStore,
	})
	if err != nil {
		return nil, fmt.Errorf("create monitor scheduler: %w", err)
	}
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	if err := s.historyStore.SaveMonitorJobDefinition(context.Background(), monitorJobDefinitionFromJob(job)); err != nil {
		return nil, fmt.Errorf("persist monitor job %s: %w", job.ID, err)
	}

	s.monitorSchedulers[job.ID] = sched

	s.emitter.Emit(Event{
		Type: "monitor_job_created",
		Payload: map[string]any{
			"job_id": job.ID,
			"name":   job.Name,
			"nodes":  len(job.Nodes),
		},
	})

	return &job, nil
}

// StartMonitorJob starts the scheduler loop for the given job.
func (s *AppService) StartMonitorJob(jobID string) error {
	s.monitorMu.RLock()
	sched, ok := s.monitorSchedulers[jobID]
	s.monitorMu.RUnlock()
	if !ok {
		return fmt.Errorf("monitor job %s not found", jobID)
	}

	if err := sched.Start(context.Background()); err != nil {
		return fmt.Errorf("start monitor job %s: %w", jobID, err)
	}

	s.emitter.Emit(Event{
		Type: "monitor_job_started",
		Payload: map[string]any{
			"job_id": jobID,
		},
	})
	return nil
}

// PauseMonitorJob pauses execution of the monitor job without stopping the scheduler.
func (s *AppService) PauseMonitorJob(jobID string) error {
	s.monitorMu.RLock()
	sched, ok := s.monitorSchedulers[jobID]
	s.monitorMu.RUnlock()
	if !ok {
		return fmt.Errorf("monitor job %s not found", jobID)
	}

	if err := sched.Pause(); err != nil {
		return fmt.Errorf("pause monitor job %s: %w", jobID, err)
	}

	s.emitter.Emit(Event{
		Type: "monitor_job_paused",
		Payload: map[string]any{
			"job_id": jobID,
		},
	})
	return nil
}

// ResumeMonitorJob resumes a paused monitor job.
func (s *AppService) ResumeMonitorJob(jobID string) error {
	s.monitorMu.RLock()
	sched, ok := s.monitorSchedulers[jobID]
	s.monitorMu.RUnlock()
	if !ok {
		return fmt.Errorf("monitor job %s not found", jobID)
	}

	if err := sched.Resume(); err != nil {
		return fmt.Errorf("resume monitor job %s: %w", jobID, err)
	}

	s.emitter.Emit(Event{
		Type: "monitor_job_resumed",
		Payload: map[string]any{
			"job_id": jobID,
		},
	})
	return nil
}

// StopMonitorJob stops the scheduler and waits for in-flight tasks to exit cleanly.
func (s *AppService) StopMonitorJob(jobID string) error {
	s.monitorMu.RLock()
	sched, ok := s.monitorSchedulers[jobID]
	s.monitorMu.RUnlock()
	if !ok {
		return fmt.Errorf("monitor job %s not found", jobID)
	}

	if err := sched.Stop(); err != nil {
		return fmt.Errorf("stop monitor job %s: %w", jobID, err)
	}

	s.emitter.Emit(Event{
		Type: "monitor_job_stopped",
		Payload: map[string]any{
			"job_id": jobID,
		},
	})
	return nil
}

// StopAllMonitorJobs stops all configured monitor schedulers.
func (s *AppService) StopAllMonitorJobs() {
	s.monitorMu.RLock()
	schedulers := make([]*monitor.Scheduler, 0, len(s.monitorSchedulers))
	for _, sched := range s.monitorSchedulers {
		schedulers = append(schedulers, sched)
	}
	s.monitorMu.RUnlock()

	for _, sched := range schedulers {
		_ = sched.Stop()
	}
}

// GetMonitorJob returns the current status and config of the specified job.
func (s *AppService) GetMonitorJob(jobID string) (*monitor.MonitorJob, error) {
	s.monitorMu.RLock()
	sched, ok := s.monitorSchedulers[jobID]
	s.monitorMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("monitor job %s not found", jobID)
	}

	job := sched.Job()
	return &job, nil
}

// ListMonitorJobs returns all configured monitor jobs.
func (s *AppService) ListMonitorJobs() []monitor.MonitorJob {
	s.monitorMu.RLock()
	defer s.monitorMu.RUnlock()

	jobs := make([]monitor.MonitorJob, 0, len(s.monitorSchedulers))
	for _, sched := range s.monitorSchedulers {
		jobs = append(jobs, sched.Job())
	}
	return jobs
}

// TriggerMonitorJob triggers an immediate single round execution of the specified job.
func (s *AppService) TriggerMonitorJob(jobID string) (*monitor.MonitorRun, error) {
	s.monitorMu.RLock()
	sched, ok := s.monitorSchedulers[jobID]
	s.monitorMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("monitor job %s not found", jobID)
	}

	return sched.TriggerImmediate(context.Background())
}

// QueryMonitorSamples retrieves raw probe samples according to the filter.
func (s *AppService) QueryMonitorSamples(ctx context.Context, filter monitor.SampleFilter) ([]*monitor.MonitorSample, error) {
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	return s.historyStore.QueryMonitorSamples(ctx, filter)
}

// QueryMonitorRuns retrieves past monitor run metadata.
func (s *AppService) QueryMonitorRuns(ctx context.Context, jobID string, limit int) ([]*monitor.MonitorRun, error) {
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	return s.historyStore.QueryMonitorRuns(ctx, jobID, limit)
}

// GetNodeTimelineSamples retrieves timestamp-ordered samples for a node key.
func (s *AppService) GetNodeTimelineSamples(ctx context.Context, nodeKey string, since time.Time) ([]*monitor.MonitorSample, error) {
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	return s.historyStore.GetNodeTimelineSamples(ctx, nodeKey, since)
}

// QueryMonitorSamplesCursor retrieves raw probe samples via keyset pagination.
func (s *AppService) QueryMonitorSamplesCursor(ctx context.Context, filter monitor.CursorFilter) (*monitor.SampleCursorPage, error) {
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	return s.historyStore.QueryMonitorSamplesCursor(ctx, filter)
}

// GetMonitorStats dynamically derives aggregated statistical metrics across raw samples.
func (s *AppService) GetMonitorStats(ctx context.Context, query monitor.StatsQuery) (*monitor.DerivedStats, error) {
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	return s.historyStore.GetDerivedStats(ctx, query)
}

// Facet scan bounds. The facet read model is presentation-only, so its cost is bounded by a
// window rather than by the full retention horizon: the UI only needs the dimensions that are
// plausibly reachable from the ranges it offers.
const (
	monitorFacetDefaultWindow = 7 * 24 * time.Hour
	monitorFacetMaxWindow     = 90 * 24 * time.Hour
	monitorFacetMaxNodes      = 500
	monitorFacetMaxValues     = 500
)

// GetMonitorSampleFacets returns the distinct node / profile / probe_type / target dimensions
// that exist in raw samples over a bounded window.
//
// This is a presentation-only projection used to populate UI filters. It does not aggregate
// or replace raw samples, and it does not participate in any monitoring decision.
func (s *AppService) GetMonitorSampleFacets(ctx context.Context, since, until *time.Time) (*monitor.MonitorSampleFacets, error) {
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}

	now := time.Now().UTC()
	winUntil := now
	if until != nil && !until.IsZero() {
		winUntil = until.UTC()
	}
	winSince := winUntil.Add(-monitorFacetDefaultWindow)
	if since != nil && !since.IsZero() {
		winSince = since.UTC()
	}

	if winSince.After(winUntil) {
		return nil, monitor.WrapValidationError(monitor.ErrInvalidTimeRange)
	}
	if winUntil.Sub(winSince) > monitorFacetMaxWindow {
		return nil, monitor.WrapValidationError(monitor.ErrQueryWindowTooLarge)
	}

	return s.historyStore.GetMonitorSampleFacets(ctx, winSince, winUntil, monitorFacetMaxNodes, monitorFacetMaxValues)
}

// ApplyRetention applies a retention policy by pruning historical raw samples and orphaned runs.
func (s *AppService) ApplyRetention(ctx context.Context, req monitor.RetentionRequest) (*monitor.RetentionResult, error) {
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	res, err := s.historyStore.ApplyRetention(ctx, req)
	if err != nil {
		var retErr *monitor.RetentionError
		if errors.As(err, &retErr) && retErr.Result != nil && retErr.Result.Partial {
			s.emitter.Emit(Event{
				Type: "monitor_retention_partial_failure",
				Payload: map[string]any{
					"policy":          retErr.Result.Policy,
					"cutoff":          retErr.Result.Cutoff,
					"samples_deleted": retErr.Result.SamplesDeleted,
					"runs_deleted":    retErr.Result.RunsDeleted,
					"duration_ms":     retErr.Result.DurationMs,
					"partial":         true,
					"error":           retErr.Err.Error(),
				},
			})
			return retErr.Result, err
		}
		return nil, err
	}

	s.emitter.Emit(Event{
		Type: "monitor_retention_applied",
		Payload: map[string]any{
			"policy":          res.Policy,
			"cutoff":          res.Cutoff,
			"samples_deleted": res.SamplesDeleted,
			"runs_deleted":    res.RunsDeleted,
			"duration_ms":     res.DurationMs,
			"partial":         false,
		},
	})
	return res, nil
}

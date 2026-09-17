package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/faceair/clash-speedtest/history"
	"github.com/faceair/clash-speedtest/profiles"
	"github.com/faceair/clash-speedtest/speedtester"
)

type ServerConfig struct {
	Port         int
	ProfilePaths profiles.Paths
	HistoryDir   string
	UserAgent    string
}

type Server struct {
	config            ServerConfig
	profilePaths      profiles.Paths
	historyStore      *history.Store
	broadcaster       *Broadcaster
	testManager       *TestManager
	httpServer        *http.Server
	port              int
	antigravityToken  string
	antigravitySource string
	antigravityMu     sync.RWMutex
	shutdownChan      chan struct{}
}

func NewServer(cfg ServerConfig) (*Server, error) {
	if cfg.ProfilePaths.Dir == "" {
		cfg.ProfilePaths = profiles.DefaultPaths()
	}

	hStore, err := history.NewStore(cfg.HistoryDir)
	if err != nil {
		return nil, fmt.Errorf("init history store: %w", err)
	}

	broadcaster := NewBroadcaster()
	testMgr := NewTestManager(hStore, cfg.ProfilePaths, broadcaster)

	s := &Server{
		config:        cfg,
		profilePaths:  cfg.ProfilePaths,
		historyStore:  hStore,
		broadcaster:   broadcaster,
		testManager:   testMgr,
		shutdownChan:  make(chan struct{}, 1),
	}

	// Auto-detect Antigravity token on startup
	if token, src, err := speedtester.TryAutoDetectAntigravityToken(); err == nil && token != "" {
		s.antigravityToken = token
		s.antigravitySource = src
	}

	return s, nil
}

func (s *Server) GetAntigravityToken() (string, string) {
	s.antigravityMu.Lock()
	defer s.antigravityMu.Unlock()
	if s.antigravityToken == "" {
		if tok, src, err := speedtester.TryAutoDetectAntigravityToken(); err == nil && tok != "" {
			s.antigravityToken = tok
			s.antigravitySource = src
		}
	}
	return s.antigravityToken, s.antigravitySource
}

func (s *Server) SetAntigravityToken(token, source string) {
	s.antigravityMu.Lock()
	defer s.antigravityMu.Unlock()
	s.antigravityToken = token
	s.antigravitySource = source
}

func (s *Server) Port() int {
	return s.port
}

func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", s.port)
}

func (s *Server) ShutdownChan() <-chan struct{} {
	return s.shutdownChan
}

func (s *Server) Start() error {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.config.Port))
	if err != nil {
		return fmt.Errorf("listen on port %d: %w", s.config.Port, err)
	}
	s.port = listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()

	// REST API routes
	mux.HandleFunc("GET /api/airports", s.handleGetAirports)
	mux.HandleFunc("POST /api/airports", s.handleCreateAirport)
	mux.HandleFunc("PUT /api/airports/{id}", s.handleUpdateAirport)
	mux.HandleFunc("DELETE /api/airports/{id}", s.handleDeleteAirport)
	mux.HandleFunc("POST /api/airports/{id}/refresh", s.handleRefreshAirport)
	mux.HandleFunc("GET /api/airports/{id}/nodes", s.handleGetAirportNodes)

	mux.HandleFunc("POST /api/test/batch", s.handleTestBatch)
	mux.HandleFunc("POST /api/test/single", s.handleTestSingle)
	mux.HandleFunc("POST /api/test/stop", s.handleTestStop)
	mux.HandleFunc("GET /api/test/status", s.handleTestStatus)

	mux.HandleFunc("GET /api/history", s.handleListHistory)
	mux.HandleFunc("GET /api/history/airports", s.handleListHistoryAirports)
	mux.HandleFunc("GET /api/history/airport", s.handleGetAirportHistory)
	mux.HandleFunc("GET /api/history/nodes", s.handleListHistoryNodes)
	mux.HandleFunc("GET /api/history/node-timeline", s.handleGetNodeTimeline)
	mux.HandleFunc("GET /api/history/{id}", s.handleGetHistory)
	mux.HandleFunc("DELETE /api/history/{id}", s.handleDeleteHistory)
	mux.HandleFunc("GET /api/history/compare", s.handleCompareHistory)

	mux.HandleFunc("GET /api/antigravity/status", s.handleAntigravityStatus)
	mux.HandleFunc("POST /api/antigravity/token", s.handleAntigravityToken)
	mux.HandleFunc("POST /api/antigravity/login", s.handleAntigravityLogin)

	mux.HandleFunc("POST /api/export/clash", s.handleExportClash)
	mux.HandleFunc("GET /api/report/html", s.handleGetReportHTML)
	mux.HandleFunc("POST /api/report/open", s.handleOpenReport)
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("POST /api/settings", s.handleSaveSettings)
	mux.HandleFunc("POST /api/settings/reopen", s.handleReopenBrowser)

	mux.HandleFunc("GET /api/events", s.handleEventsSSE)
	mux.HandleFunc("POST /api/shutdown", s.handleShutdown)

	// Frontend SPA handler for static assets
	mux.Handle("/", WebHandler())

	s.httpServer = &http.Server{
		Handler:      corsMiddleware(mux),
		ReadTimeout:  30 * time.Minute,
		WriteTimeout: 30 * time.Minute,
	}

	go func() {
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %s", err)
		}
	}()

	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	if s.testManager != nil {
		s.testManager.Stop()
	}
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// Helper: JSON response writer
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- Airport Endpoints ----

type AirportDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	NodeCount int       `json:"node_count"`
	HasCache  bool      `json:"has_cache"`
}

func (s *Server) handleGetAirports(w http.ResponseWriter, r *http.Request) {
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Auto import legacy if empty
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
			nodeCount = s.countCachedNodes(ap.ID)
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

	writeJSON(w, http.StatusOK, dtos)
}

func (s *Server) countCachedNodes(airportID string) int {
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

type CreateAirportReq struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func (s *Server) handleCreateAirport(w http.ResponseWriter, r *http.Request) {
	var req CreateAirportReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.URL = strings.TrimSpace(req.URL)
	if req.Name == "" || req.URL == "" {
		writeError(w, http.StatusBadRequest, "机场名称和订阅链接不能为空")
		return
	}

	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	ap := &profiles.Airport{
		ID:   fmt.Sprintf("%d", time.Now().UnixNano()),
		Name: req.Name,
		URL:  req.URL,
	}

	// Fetch subscription
	if profiles.IsHTTPURL(ap.URL) {
		body, err := profiles.FetchSubscription(ap.URL, s.config.UserAgent)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("获取订阅节点失败: %s", err))
			return
		}
		if err := s.profilePaths.WriteCache(ap.ID, body); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("写入节点缓存失败: %s", err))
			return
		}
		ap.UpdatedAt = time.Now()
	} else {
		// Local file path
		expanded := profiles.ExpandLocalPath(ap.URL)
		data, err := os.ReadFile(expanded)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("读取本地节点文件失败: %s", err))
			return
		}
		if err := s.profilePaths.WriteCache(ap.ID, data); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("写入节点缓存失败: %s", err))
			return
		}
		ap.UpdatedAt = time.Now()
	}

	store.Add(ap)
	if err := profiles.SaveStore(s.profilePaths.StoreFile(), store); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	nodeCount := s.countCachedNodes(ap.ID)
	writeJSON(w, http.StatusOK, AirportDTO{
		ID:        ap.ID,
		Name:      ap.Name,
		URL:       ap.URL,
		UpdatedAt: ap.UpdatedAt,
		NodeCount: nodeCount,
		HasCache:  true,
	})
}

func (s *Server) handleUpdateAirport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req CreateAirportReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}

	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	ap := store.Get(id)
	if ap == nil {
		writeError(w, http.StatusNotFound, "未找到该机场")
		return
	}

	urlChanged := req.URL != "" && req.URL != ap.URL
	if req.Name != "" {
		ap.Name = req.Name
	}
	if req.URL != "" {
		ap.URL = req.URL
	}

	if urlChanged {
		s.profilePaths.RemoveCache(ap.ID)
		if profiles.IsHTTPURL(ap.URL) {
			if body, err := profiles.FetchSubscription(ap.URL, s.config.UserAgent); err == nil {
				_ = s.profilePaths.WriteCache(ap.ID, body)
				ap.UpdatedAt = time.Now()
			}
		}
	}

	if err := profiles.SaveStore(s.profilePaths.StoreFile(), store); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	nodeCount := s.countCachedNodes(ap.ID)
	writeJSON(w, http.StatusOK, AirportDTO{
		ID:        ap.ID,
		Name:      ap.Name,
		URL:       ap.URL,
		UpdatedAt: ap.UpdatedAt,
		NodeCount: nodeCount,
		HasCache:  s.profilePaths.HasCache(ap.ID),
	})
}

func (s *Server) handleDeleteAirport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	removed := store.Remove(id)
	if removed == nil {
		writeError(w, http.StatusNotFound, "未找到该机场")
		return
	}

	s.profilePaths.RemoveCache(id)
	if err := profiles.SaveStore(s.profilePaths.StoreFile(), store); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleRefreshAirport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	ap := store.Get(id)
	if ap == nil {
		writeError(w, http.StatusNotFound, "未找到该机场")
		return
	}

	var body []byte
	if profiles.IsHTTPURL(ap.URL) {
		b, err := profiles.FetchSubscription(ap.URL, s.config.UserAgent)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("获取订阅失败: %s", err))
			return
		}
		body = b
	} else {
		expanded := profiles.ExpandLocalPath(ap.URL)
		b, err := os.ReadFile(expanded)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("读取本地文件失败: %s", err))
			return
		}
		body = b
	}

	if err := s.profilePaths.WriteCache(ap.ID, body); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("缓存保存失败: %s", err))
		return
	}

	ap.UpdatedAt = time.Now()
	_ = profiles.SaveStore(s.profilePaths.StoreFile(), store)

	nodeCount := s.countCachedNodes(ap.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"node_count": nodeCount,
		"updated_at": ap.UpdatedAt,
	})
}

type NodeDTO struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Server       string `json:"server,omitempty"`
	Port         int    `json:"port,omitempty"`
	CountryCode  string `json:"country_code"`
	CountryFlag  string `json:"country_flag"`
	CountryLabel string `json:"country_label"`
}

type AirportNodesResp struct {
	Nodes      []NodeDTO               `json:"nodes"`
	Countries  []profiles.CountryGroup `json:"countries"`
	TotalNodes int                     `json:"total_nodes"`
}

func (s *Server) handleGetAirportNodes(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cacheFile := s.profilePaths.CacheFile(id)
	if !s.profilePaths.HasCache(id) {
		writeError(w, http.StatusBadRequest, "机场节点尚未缓存，请先更新订阅")
		return
	}

	st, err := speedtester.New(&speedtester.Config{
		ConfigPaths: cacheFile,
		Mode:        speedtester.SpeedModeFast,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("解析节点失败: %s", err))
		return
	}

	proxies, err := st.LoadProxies()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("加载节点失败: %s", err))
		return
	}

	nodeNames := make([]string, 0, len(proxies))
	dtos := make([]NodeDTO, 0, len(proxies))

	for name, p := range proxies {
		nodeNames = append(nodeNames, name)
		code := profiles.DetectCountry(name)
		flag := profiles.FlagFromCode(code)
		label := profiles.LabelFromCode(code)
		if code == profiles.OtherCountryCode {
			code = ""
			flag = ""
			label = ""
		}

		serverStr := ""
		portInt := 0
		if p.Config != nil {
			if sVal, ok := p.Config["server"].(string); ok {
				serverStr = sVal
			}
			if pVal, ok := p.Config["port"].(int); ok {
				portInt = pVal
			}
		}

		dtos = append(dtos, NodeDTO{
			Name:         name,
			Type:         p.Type().String(),
			Server:       serverStr,
			Port:         portInt,
			CountryCode:  code,
			CountryFlag:  flag,
			CountryLabel: label,
		})
	}

	countryGroups := profiles.GroupByCountry(nodeNames)

	writeJSON(w, http.StatusOK, AirportNodesResp{
		Nodes:      dtos,
		Countries:  countryGroups,
		TotalNodes: len(dtos),
	})
}

// ---- Test Endpoints ----

func (s *Server) handleTestBatch(w http.ResponseWriter, r *http.Request) {
	var req BatchTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}

	token, _ := s.GetAntigravityToken()
	if err := s.testManager.StartBatch(req, token); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"started": true})
}

func (s *Server) handleTestSingle(w http.ResponseWriter, r *http.Request) {
	var req SingleTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}

	token, _ := s.GetAntigravityToken()
	res, err := s.testManager.TestSingle(req, token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleTestStop(w http.ResponseWriter, r *http.Request) {
	s.testManager.Stop()
	writeJSON(w, http.StatusOK, map[string]bool{"stopped": true})
}

func (s *Server) handleTestStatus(w http.ResponseWriter, r *http.Request) {
	status := s.testManager.Status()
	writeJSON(w, http.StatusOK, status)
}

// ---- History Endpoints ----

func (s *Server) handleListHistory(w http.ResponseWriter, r *http.Request) {
	list, err := s.historyStore.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := s.historyStore.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "历史记录不存在")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleDeleteHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.historyStore.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleCompareHistory(w http.ResponseWriter, r *http.Request) {
	baseID := r.URL.Query().Get("base")
	if baseID == "" {
		baseID = r.URL.Query().Get("base_id")
	}
	targetID := r.URL.Query().Get("target")
	if targetID == "" {
		targetID = r.URL.Query().Get("target_id")
	}
	if baseID == "" || targetID == "" {
		writeError(w, http.StatusBadRequest, "必须指定 base 和 target 记录 ID")
		return
	}

	comp, err := s.historyStore.Compare(baseID, targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, comp)
}

func (s *Server) handleListHistoryNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.historyStore.ListAllDistinctNodes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

func (s *Server) handleGetNodeTimeline(w http.ResponseWriter, r *http.Request) {
	nodeName := r.URL.Query().Get("name")
	if nodeName == "" {
		writeError(w, http.StatusBadRequest, "必须指定节点名称 (name)")
		return
	}
	timeline, err := s.historyStore.GetNodeTimeline(nodeName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, timeline)
}

func (s *Server) handleListHistoryAirports(w http.ResponseWriter, r *http.Request) {
	airports, err := s.historyStore.ListHistoryAirports()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, airports)
}

func (s *Server) handleGetAirportHistory(w http.ResponseWriter, r *http.Request) {
	airport := r.URL.Query().Get("airport")
	history, err := s.historyStore.GetAirportHistory(airport)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, history)
}

// ---- Antigravity Endpoints ----

type AntigravityStatusResp struct {
	HasToken     bool   `json:"has_token"`
	Source       string `json:"source"`
	TokenPreview string `json:"token_preview"`
}

func (s *Server) handleAntigravityStatus(w http.ResponseWriter, r *http.Request) {
	token, source := s.GetAntigravityToken()

	preview := ""
	if len(token) > 16 {
		preview = token[:8] + "..." + token[len(token)-4:]
	} else if token != "" {
		preview = "***"
	}

	writeJSON(w, http.StatusOK, AntigravityStatusResp{
		HasToken:     token != "",
		Source:       source,
		TokenPreview: preview,
	})
}

type TokenReq struct {
	Token string `json:"token"`
}

func (s *Server) handleAntigravityToken(w http.ResponseWriter, r *http.Request) {
	var req TokenReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}

	token := strings.TrimSpace(req.Token)
	parsed := speedtester.ParseAntigravityToken(token)
	if parsed == "" {
		writeError(w, http.StatusBadRequest, "Token 格式无效，请检查")
		return
	}

	s.SetAntigravityToken(parsed, "手动输入")
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		_ = os.WriteFile(filepath.Join(home, ".antigravity_token"), []byte(parsed+"\n"), 0600)
	}

	s.broadcaster.Broadcast(Event{
		Type: "antigravity_token_updated",
		Payload: map[string]any{
			"source": "手动输入",
		},
	})

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleAntigravityLogin(w http.ResponseWriter, r *http.Request) {
	go func() {
		token, err := speedtester.StartOAuthBrowserFlow(os.Stdout)
		if err != nil {
			log.Printf("OAuth login failed: %s", err)
			s.broadcaster.Broadcast(Event{
				Type: "antigravity_login_failed",
				Payload: map[string]any{
					"error": err.Error(),
				},
			})
			return
		}

		s.SetAntigravityToken(token, "Google 浏览器登录")
		_ = os.WriteFile(".antigravity_token", []byte(token+"\n"), 0600)

		s.broadcaster.Broadcast(Event{
			Type: "antigravity_token_updated",
			Payload: map[string]any{
				"source": "Google 浏览器登录",
			},
		})
	}()

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "browser_flow_started",
	})
}

// ---- Export Endpoints ----

type ExportClashReq struct {
	AirportID string   `json:"airport_id"`
	NodeNames []string `json:"node_names"` // empty means all
	RunID     string   `json:"run_id,omitempty"`
}

func (s *Server) handleExportClash(w http.ResponseWriter, r *http.Request) {
	var req ExportClashReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}

	cacheFile := s.profilePaths.CacheFile(req.AirportID)
	if !s.profilePaths.HasCache(req.AirportID) {
		writeError(w, http.StatusBadRequest, "机场缓存不存在")
		return
	}

	st, err := speedtester.New(&speedtester.Config{
		ConfigPaths: cacheFile,
		Mode:        speedtester.SpeedModeFast,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	proxies, err := st.LoadProxies()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	nameSet := make(map[string]struct{}, len(req.NodeNames))
	for _, n := range req.NodeNames {
		nameSet[n] = struct{}{}
	}

	var exportProxies []map[string]any
	for name, p := range proxies {
		if len(req.NodeNames) > 0 {
			if _, ok := nameSet[name]; !ok {
				continue
			}
		}
		if p.Config != nil {
			exportProxies = append(exportProxies, p.Config)
		}
	}

	yamlData, err := ExportClashYAML(exportProxies)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("导出失败: %s", err))
		return
	}

	filename := fmt.Sprintf("clash-proxies-%s.yaml", time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(yamlData)))
	_, _ = w.Write(yamlData)
}

type ExportCSVReq struct {
	RunID string `json:"run_id"`
}

func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	var req ExportCSVReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}

	run, err := s.historyStore.Get(req.RunID)
	if err != nil {
		writeError(w, http.StatusNotFound, "历史记录不存在")
		return
	}

	csvData, err := ExportCSV(run.Results)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("导出失败: %s", err))
		return
	}

	filename := fmt.Sprintf("speedtest-%s.csv", run.ID)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(csvData)))
	_, _ = w.Write(csvData)
}

// ---- Settings Endpoints ----

type SettingsResp struct {
	PreferredBrowser  string        `json:"preferred_browser"`
	AvailableBrowsers []BrowserInfo `json:"available_browsers"`
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, _ := LoadSettings()
	browsers := GetAvailableBrowsers()
	writeJSON(w, http.StatusOK, SettingsResp{
		PreferredBrowser:  settings.PreferredBrowser,
		AvailableBrowsers: browsers,
	})
}

type SaveSettingsReq struct {
	PreferredBrowser string `json:"preferred_browser"`
}

func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	var req SaveSettingsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效参数")
		return
	}
	settings, _ := LoadSettings()
	if req.PreferredBrowser != "" {
		settings.PreferredBrowser = req.PreferredBrowser
	}
	_ = SaveSettings(settings)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleReopenBrowser(w http.ResponseWriter, r *http.Request) {
	var req SaveSettingsReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	target := req.PreferredBrowser
	if target == "" {
		settings, _ := LoadSettings()
		target = settings.PreferredBrowser
	}
	_, err := LaunchApp(s.URL(), target)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("启动失败: %s", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"reopened": true})
}

func (s *Server) handleGetReportHTML(w http.ResponseWriter, r *http.Request) {
	allRuns, err := s.historyStore.GetAllRuns()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取历史记录失败: "+err.Error())
		return
	}
	html, err := history.GenerateHTMLReport(allRuns)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "生成报告失败: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}

func (s *Server) handleOpenReport(w http.ResponseWriter, r *http.Request) {
	allRuns, _ := s.historyStore.GetAllRuns()
	reportPaths := []string{"report.html"}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		reportPaths = append(reportPaths, filepath.Join(home, ".clash-speedtest", "report.html"))
	}
	_ = history.SaveHTMLReport(allRuns, reportPaths...)

	absPath, err := filepath.Abs("report.html")
	if err != nil {
		absPath = "report.html"
	}
	fileURL := "file:///" + filepath.ToSlash(absPath)

	preferred := ""
	if settings, err := LoadSettings(); err == nil {
		preferred = settings.PreferredBrowser
	}
	_, _ = LaunchApp(fileURL, preferred)

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"file":    absPath,
		"url":     fileURL,
	})
}

// ---- SSE Stream Endpoint ----

func (s *Server) handleEventsSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := s.broadcaster.Subscribe()
	defer s.broadcaster.Unsubscribe(ch)

	// Send initial ping
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": keep-alive\n\n")
			flusher.Flush()
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", string(msg))
			flusher.Flush()
		}
	}
}

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"shutdown": true})
	go func() {
		time.Sleep(200 * time.Millisecond)
		select {
		case s.shutdownChan <- struct{}{}:
		default:
		}
	}()
}

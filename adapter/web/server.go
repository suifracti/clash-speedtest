package web

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/auth"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/policy"
	"github.com/faceair/clash-speedtest/core/profiles"
)

// ServerConfig configures the HTTP & SSE server.
type ServerConfig struct {
	Port          int
	ProfilePaths  profiles.Paths
	HistoryDir    string
	UserAgent     string
	StaticHandler http.Handler
}

// Server provides the HTTP REST and SSE endpoints, adapting them to AppService.
type Server struct {
	config       ServerConfig
	app          *application.AppService
	emitter      *SSEEmitter
	httpServer   *http.Server
	port         int
	shutdownChan chan struct{}
	stopOnce     sync.Once
	stopErr      error
}

// NewServer constructs a new web adapter Server.
func NewServer(cfg ServerConfig) (*Server, error) {
	if cfg.ProfilePaths.Dir == "" {
		cfg.ProfilePaths = profiles.DefaultPaths()
	}

	hStore, err := history.NewStore(cfg.HistoryDir)
	if err != nil {
		return nil, fmt.Errorf("init history store: %w", err)
	}

	emitter := NewSSEEmitter()
	appSvc := application.NewAppService(hStore, cfg.ProfilePaths, emitter)

	s := &Server{
		config:       cfg,
		app:          appSvc,
		emitter:      emitter,
		shutdownChan: make(chan struct{}, 1),
	}

	return s, nil
}

func (s *Server) AppService() *application.AppService {
	return s.app
}

func (s *Server) Emitter() *SSEEmitter {
	return s.emitter
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

// Close stops the server and cleanly releases backend resources including draining monitor jobs and closing SQLite connections.
func (s *Server) Close() error {
	return s.Stop(context.Background())
}

// Handler returns the fully configured http.Handler with routing and security middleware.
func (s *Server) Handler() http.Handler {
	return s.buildHandler()
}

func (s *Server) buildHandler() http.Handler {
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
	mux.HandleFunc("GET /api/history/airport", s.handleGetAirportHistory)
	mux.HandleFunc("GET /api/history/{id}", s.handleGetHistory)
	mux.HandleFunc("DELETE /api/history/{id}", s.handleDeleteHistory)
	mux.HandleFunc("GET /api/history/compare", s.handleCompareHistory)

	mux.HandleFunc("GET /api/antigravity/status", s.handleAntigravityStatus)
	mux.HandleFunc("POST /api/antigravity/token", s.handleAntigravityToken)
	mux.HandleFunc("POST /api/antigravity/login", s.handleAntigravityLogin)

	mux.HandleFunc("POST /api/export/clash", s.handleExportClash)
	mux.HandleFunc("POST /api/export/csv", s.handleExportCSV)
	mux.HandleFunc("GET /api/report/html", s.handleGetReportHTML)
	mux.HandleFunc("POST /api/report/open", s.handleOpenReport)
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("POST /api/settings", s.handleSaveSettings)

	// Controller & Policy REST routes
	mux.HandleFunc("GET /api/controller/status", s.handleGetControllerStatus)
	mux.HandleFunc("POST /api/controller/config", s.handleConfigureController)
	mux.HandleFunc("GET /api/controller/groups", s.handleListControllerGroups)
	mux.HandleFunc("POST /api/controller/select", s.handleSelectControllerNode)
	mux.HandleFunc("GET /api/controller/policy", s.handleGetSwitchPolicy)
	mux.HandleFunc("POST /api/controller/policy", s.handleUpdateSwitchPolicy)
	mux.HandleFunc("GET /api/controller/audit", s.handleGetSwitchAuditTrail)

	// 24/7 Monitor REST routes
	mux.HandleFunc("GET /api/monitor/nodes", s.handleListMonitorNodeOptions)
	mux.HandleFunc("POST /api/monitor/jobs", s.handleCreateMonitorJob)
	mux.HandleFunc("GET /api/monitor/jobs", s.handleListMonitorJobs)
	mux.HandleFunc("GET /api/monitor/jobs/{id}", s.handleGetMonitorJob)
	mux.HandleFunc("POST /api/monitor/jobs/{id}/start", s.handleStartMonitorJob)
	mux.HandleFunc("POST /api/monitor/jobs/{id}/pause", s.handlePauseMonitorJob)
	mux.HandleFunc("POST /api/monitor/jobs/{id}/resume", s.handleResumeMonitorJob)
	mux.HandleFunc("POST /api/monitor/jobs/{id}/stop", s.handleStopMonitorJob)
	mux.HandleFunc("POST /api/monitor/jobs/{id}/trigger", s.handleTriggerMonitorJob)
	mux.HandleFunc("GET /api/monitor/runs", s.handleQueryMonitorRuns)
	mux.HandleFunc("GET /api/monitor/samples", s.handleQueryMonitorSamples)
	mux.HandleFunc("GET /api/monitor/samples/cursor", s.handleQueryMonitorSamplesCursor)
	mux.HandleFunc("GET /api/monitor/stats", s.handleGetMonitorStats)
	mux.HandleFunc("POST /api/monitor/retention", s.handleApplyRetention)
	mux.HandleFunc("GET /api/monitor/timeline", s.handleGetNodeTimelineSamples)
	mux.HandleFunc("GET /api/monitor/facets", s.handleGetMonitorFacets)

	// PR#7: read-only evidence → recommendation. There is deliberately NO execution
	// endpoint here: the recommendation is advisory only and never switches a node.
	mux.HandleFunc("GET /api/monitor/recommendation", s.handleGetMonitorRecommendation)

	mux.HandleFunc("GET /api/events", s.handleEventsSSE)
	mux.HandleFunc("POST /api/shutdown", s.handleShutdown)

	// Static UI assets
	if s.config.StaticHandler != nil {
		mux.Handle("/", s.config.StaticHandler)
	}

	return securityMiddleware(mux)
}

func (s *Server) Start() error {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.config.Port))
	if err != nil {
		return fmt.Errorf("listen on port %d: %w", s.config.Port, err)
	}
	s.port = listener.Addr().(*net.TCPAddr).Port

	s.httpServer = &http.Server{
		Handler:      s.buildHandler(),
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 0, // Allow SSE streaming
	}

	go func() {
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %s", err)
		}
	}()

	return nil
}

// Stop shuts down the HTTP server and cascades to AppService.Close() to drain monitor jobs and close storage.
// Protected by sync.Once to guarantee idempotent teardown without double-close errors.
func (s *Server) Stop(ctx context.Context) error {
	s.stopOnce.Do(func() {
		var httpErr error
		if s.httpServer != nil {
			httpErr = s.httpServer.Shutdown(ctx)
		}
		var appErr error
		if s.app != nil {
			appErr = s.app.Close()
		}
		if httpErr != nil {
			s.stopErr = httpErr
		} else {
			s.stopErr = appErr
		}
	})
	return s.stopErr
}

// SPAHandler wraps an fs.FS and serves static assets with fallback to index.html for SPA routing.
func SPAHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		if f, err := fsys.Open(path); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// Fallback to index.html for SPA client routes
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// isLoopbackHost checks whether a host (with or without port) is a trusted loopback address.
func isLoopbackHost(rawHost string) bool {
	if rawHost == "" {
		return false
	}
	host := rawHost
	if h, _, err := net.SplitHostPort(rawHost); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// securityMiddleware enforces loopback Host validation, CSRF defenses on mutating endpoints,
// and rejects untrusted origins (no CORS wildcard allowed).
func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Host header validation: local server must only be accessed via loopback host.
		// Protects against DNS rebinding attacks.
		if !isLoopbackHost(r.Host) {
			http.Error(w, "Forbidden: invalid or non-loopback Host header", http.StatusForbidden)
			return
		}

		origin := r.Header.Get("Origin")
		var isLoopbackOrigin bool
		if origin != "" {
			if u, err := url.Parse(origin); err == nil {
				isLoopbackOrigin = isLoopbackHost(u.Hostname())
			}
		}

		// 2. CORS preflight (OPTIONS)
		if r.Method == http.MethodOptions {
			if origin != "" {
				if !isLoopbackOrigin {
					http.Error(w, "Forbidden: untrusted origin", http.StatusForbidden)
					return
				}
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Vary", "Origin")
			}
			w.WriteHeader(http.StatusOK)
			return
		}

		// 3. Mutating requests protection (CSRF defense)
		if isMutatingMethod(r.Method) {
			if origin != "" && !isLoopbackOrigin {
				http.Error(w, "Forbidden: untrusted origin for mutating request", http.StatusForbidden)
				return
			}
			if ref := r.Header.Get("Referer"); ref != "" {
				if u, err := url.Parse(ref); err != nil || !isLoopbackHost(u.Hostname()) {
					http.Error(w, "Forbidden: untrusted referer for mutating request", http.StatusForbidden)
					return
				}
			}
			if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
				http.Error(w, "Forbidden: cross-site mutating request rejected", http.StatusForbidden)
				return
			}
		}

		// 4. Safe reflection of loopback origin for local dev servers (e.g. Vite on localhost:5173).
		// NEVER emit Access-Control-Allow-Origin: *
		if origin != "" && isLoopbackOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}

		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// Handlers

func (s *Server) handleGetAirports(w http.ResponseWriter, r *http.Request) {
	airports, err := s.app.ListAirports()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, airports)
}

type createAirportReq struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func (s *Server) handleCreateAirport(w http.ResponseWriter, r *http.Request) {
	var req createAirportReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	dto, err := s.app.CreateAirport(req.Name, req.URL, s.config.UserAgent)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) handleUpdateAirport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req createAirportReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	dto, err := s.app.UpdateAirport(id, req.Name, req.URL, s.config.UserAgent)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) handleDeleteAirport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.app.DeleteAirport(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleRefreshAirport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dto, err := s.app.RefreshAirport(id, s.config.UserAgent)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) handleGetAirportNodes(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	nodes, err := s.app.GetAirportNodes(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

func (s *Server) handleTestBatch(w http.ResponseWriter, r *http.Request) {
	var req application.BatchTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	if err := s.app.StartBatch(req, ""); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"started": true})
}

func (s *Server) handleTestSingle(w http.ResponseWriter, r *http.Request) {
	var req application.SingleTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效的请求参数")
		return
	}
	res, err := s.app.TestSingle(req, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleTestStop(w http.ResponseWriter, r *http.Request) {
	s.app.Stop()
	writeJSON(w, http.StatusOK, map[string]bool{"stopped": true})
}

func (s *Server) handleTestStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.app.Status())
}

func (s *Server) handleListHistory(w http.ResponseWriter, r *http.Request) {
	list, err := s.app.ListHistory()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetAirportHistory(w http.ResponseWriter, r *http.Request) {
	airportID := r.URL.Query().Get("airport_id")
	if airportID == "" {
		writeError(w, http.StatusBadRequest, "airport_id 不能为空")
		return
	}
	h, err := s.app.GetAirportTimeline(airportID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, h)
}

func (s *Server) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := s.app.GetHistory(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "历史记录未找到")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleDeleteHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.app.DeleteHistory(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleCompareHistory(w http.ResponseWriter, r *http.Request) {
	baseID := r.URL.Query().Get("base_id")
	targetID := r.URL.Query().Get("target_id")
	if baseID == "" || targetID == "" {
		writeError(w, http.StatusBadRequest, "base_id 和 target_id 不能为空")
		return
	}
	cmp, err := s.app.CompareRuns(baseID, targetID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cmp)
}

func (s *Server) handleAntigravityStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.app.GetTokenStatus())
}

type tokenReq struct {
	Token string `json:"token"`
}

func (s *Server) handleAntigravityToken(w http.ResponseWriter, r *http.Request) {
	var req tokenReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效请求")
		return
	}
	parsed := auth.ParseAntigravityToken(strings.TrimSpace(req.Token))
	if parsed == "" {
		writeError(w, http.StatusBadRequest, "Token 格式无效，请检查")
		return
	}

	s.app.SetAntigravityToken(parsed, "手动输入")
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		_ = os.WriteFile(filepath.Join(home, ".antigravity_token"), []byte(parsed+"\n"), 0600)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleAntigravityLogin(w http.ResponseWriter, r *http.Request) {
	go func() {
		token, err := auth.StartOAuthBrowserFlow(os.Stdout)
		if err != nil {
			log.Printf("OAuth login failed: %s", err)
			s.emitter.Emit(application.Event{
				Type: "antigravity_login_failed",
				Payload: map[string]any{
					"error": err.Error(),
				},
			})
			return
		}
		s.app.SetAntigravityToken(token, "Google OAuth 登录")
	}()
	writeJSON(w, http.StatusOK, map[string]bool{"started": true})
}

type exportClashReq struct {
	AirportID string   `json:"airport_id"`
	NodeNames []string `json:"node_names"`
}

func (s *Server) handleExportClash(w http.ResponseWriter, r *http.Request) {
	var req exportClashReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效请求")
		return
	}
	yamlData, err := s.app.ExportClashConfig(req.AirportID, req.NodeNames)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	filename := fmt.Sprintf("clash-proxies-%s.yaml", time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(yamlData)))
	_, _ = w.Write(yamlData)
}

type exportCSVReq struct {
	RunID string `json:"run_id"`
}

func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	var req exportCSVReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "无效请求")
		return
	}
	run, err := s.app.GetHistory(req.RunID)
	if err != nil {
		writeError(w, http.StatusNotFound, "历史记录不存在")
		return
	}
	csvData, err := exportCSVData(run.Results)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	filename := fmt.Sprintf("speedtest-%s.csv", run.ID)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(csvData)))
	_, _ = w.Write(csvData)
}

func exportCSVData(results []*history.RunNodeResult) ([]byte, error) {
	buf := new(bytes.Buffer)
	buf.WriteString("\xEF\xBB\xBF") // UTF-8 BOM
	w := csv.NewWriter(buf)
	header := []string{
		"序号", "节点名称", "协议类型", "国家地区", "真实延迟 (ms)", "抖动 (ms)",
		"丢包率 (%)", "下载速度 (MB/s)", "上传速度 (MB/s)", "Antigravity状态", "Antigravity详情",
	}
	if err := w.Write(header); err != nil {
		return nil, err
	}
	for i, r := range results {
		row := []string{
			strconv.Itoa(i + 1),
			r.ProxyName,
			r.ProxyType,
			r.CountryFlag + " " + r.CountryCode,
			strconv.FormatInt(r.LatencyMs, 10),
			strconv.FormatInt(r.JitterMs, 10),
			fmt.Sprintf("%.1f", r.PacketLoss),
			fmt.Sprintf("%.2f", r.DownloadSpeedMBps),
			fmt.Sprintf("%.2f", r.UploadSpeedMBps),
			r.AntigravityStatus,
			r.AntigravityDetail,
		}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

func (s *Server) handleGetReportHTML(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile("report.html")
	if err != nil {
		if home, err2 := os.UserHomeDir(); err2 == nil && home != "" {
			data, err = os.ReadFile(filepath.Join(home, ".clash-speedtest", "report.html"))
		}
	}
	if err != nil {
		writeError(w, http.StatusNotFound, "报告文件尚未生成，请先进行一次测速")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *Server) handleOpenReport(w http.ResponseWriter, r *http.Request) {
	reportPath := "report.html"
	if _, err := os.Stat(reportPath); err != nil {
		if home, err2 := os.UserHomeDir(); err2 == nil && home != "" {
			alt := filepath.Join(home, ".clash-speedtest", "report.html")
			if _, err3 := os.Stat(alt); err3 == nil {
				reportPath = alt
			}
		}
	}
	abs, err := filepath.Abs(reportPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	fileURL := "file:///" + filepath.ToSlash(abs)
	_ = auth.OpenBrowser(fileURL)
	writeJSON(w, http.StatusOK, map[string]string{"url": fileURL})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.app.GetSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	var settings application.AppSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, "无效参数")
		return
	}
	if err := s.app.SaveSettings(&settings); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleEventsSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := s.emitter.Subscribe()
	defer s.emitter.Unsubscribe(ch)

	// Send initial status
	initialEvent := application.Event{
		Type:    "initial_status",
		Payload: s.app.Status(),
	}
	initData, _ := json.Marshal(initialEvent)
	fmt.Fprintf(w, "data: %s\n\n", initData)
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"shutting_down": true})
	go func() {
		time.Sleep(200 * time.Millisecond)
		select {
		case s.shutdownChan <- struct{}{}:
		default:
		}
	}()
}

// --- Controller & Policy Handlers ---

func (s *Server) handleGetControllerStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.app.GetControllerStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleConfigureController(w http.ResponseWriter, r *http.Request) {
	var cfg application.ControllerConfigDTO
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	if err := s.app.ConfigureController(r.Context(), cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleListControllerGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.app.ListControllerGroups(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

func (s *Server) handleSelectControllerNode(w http.ResponseWriter, r *http.Request) {
	var req application.SelectNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	if err := s.app.SelectControllerNode(r.Context(), req.Group, req.Node); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleGetSwitchPolicy(w http.ResponseWriter, r *http.Request) {
	pol, err := s.app.GetSwitchPolicy(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pol)
}

func (s *Server) handleUpdateSwitchPolicy(w http.ResponseWriter, r *http.Request) {
	var pol policy.SwitchPolicy
	if err := json.NewDecoder(r.Body).Decode(&pol); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	if err := s.app.UpdateSwitchPolicy(r.Context(), pol); err != nil {
		if monitor.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pol)
}

func (s *Server) handleGetSwitchAuditTrail(w http.ResponseWriter, r *http.Request) {
	audit, err := s.app.GetSwitchAuditTrail(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, audit)
}

// --- 24/7 Monitor Handlers ---

func (s *Server) handleCreateMonitorJob(w http.ResponseWriter, r *http.Request) {
	var req application.MonitorJobCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	created, err := s.app.CreateMonitorJobFromRequest(req)
	if err != nil {
		if monitor.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleListMonitorNodeOptions(w http.ResponseWriter, r *http.Request) {
	options, err := s.app.ListMonitorNodeOptions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, options)
}

func (s *Server) handleListMonitorJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.app.ListMonitorJobDTOs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, jobs)
}

func (s *Server) handleGetMonitorJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := s.app.GetMonitorJobDTO(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleStartMonitorJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.app.StartMonitorJob(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"started": true})
}

func (s *Server) handlePauseMonitorJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.app.PauseMonitorJob(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"paused": true})
}

func (s *Server) handleResumeMonitorJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.app.ResumeMonitorJob(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"resumed": true})
}

func (s *Server) handleStopMonitorJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.app.StopMonitorJob(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"stopped": true})
}

func (s *Server) handleTriggerMonitorJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := s.app.TriggerMonitorJob(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleQueryMonitorRuns(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Query().Get("job_id")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	runs, err := s.app.QueryMonitorRuns(r.Context(), jobID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) handleQueryMonitorSamples(w http.ResponseWriter, r *http.Request) {
	filter := monitor.SampleFilter{
		RunID:     r.URL.Query().Get("run_id"),
		NodeKey:   r.URL.Query().Get("node_key"),
		ProfileID: r.URL.Query().Get("profile_id"),
		ProbeType: r.URL.Query().Get("probe_type"),
	}
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			filter.Limit = limit
		}
	}
	if successStr := r.URL.Query().Get("success"); successStr != "" {
		val := successStr == "true" || successStr == "1"
		filter.Success = &val
	}
	samples, err := s.app.QueryMonitorSamples(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, samples)
}

func (s *Server) handleGetNodeTimelineSamples(w http.ResponseWriter, r *http.Request) {
	nodeKey := r.URL.Query().Get("node_key")
	if nodeKey == "" {
		writeError(w, http.StatusBadRequest, "node_key query param required")
		return
	}
	sinceStr := r.URL.Query().Get("since")
	since := time.Now().Add(-24 * time.Hour)
	if sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		}
	}
	samples, err := s.app.GetNodeTimelineSamples(r.Context(), nodeKey, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, samples)
}

// handleGetMonitorRecommendation serves a read-only, evidence-backed node recommendation.
//
// This endpoint is deliberately non-executing: it only reads persisted monitor evidence
// and the configured policy, and returns an explainable recommendation. It never switches
// the active node, never mutates the controller, and never triggers a monitor run.
//
// The configured orchestrator mode is respected: under monitor_only no recommendation is
// produced unless the caller explicitly opts in with ?preview=true.
func (s *Server) handleGetMonitorRecommendation(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	req := application.MonitorRecommendationRequest{
		JobID:                  q.Get("job_id"),
		CurrentNodeKey:         q.Get("current_node_key"),
		CurrentNodeIdentityKey: q.Get("current_node_identity_key"),
		Purpose:                q.Get("purpose"),
	}
	if raw := q.Get("candidate_node_keys"); raw != "" {
		for _, key := range strings.Split(raw, ",") {
			key = strings.TrimSpace(key)
			if key != "" {
				req.CandidateNodeKeys = append(req.CandidateNodeKeys, key)
			}
		}
	}
	if raw := q.Get("preview"); raw != "" {
		preview, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid preview parameter: must be a boolean")
			return
		}
		req.Preview = preview
	}

	rec, err := s.app.GetMonitorRecommendation(r.Context(), req)
	if err != nil {
		if monitor.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// handleGetMonitorFacets serves the presentation-only facet read model used to build UI filters.
func (s *Server) handleGetMonitorFacets(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var since, until *time.Time
	if v := q.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid since parameter: must be RFC3339")
			return
		}
		since = &t
	}
	if v := q.Get("until"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid until parameter: must be RFC3339")
			return
		}
		until = &t
	}

	facets, err := s.app.GetMonitorSampleFacets(r.Context(), since, until)
	if err != nil {
		if monitor.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, facets)
}

func (s *Server) handleQueryMonitorSamplesCursor(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := monitor.CursorFilter{
		NodeIdentityKey: q.Get("node_identity_key"),
		LegacyNodeKey:   q.Get("legacy_node_key"),
		NodeKey:         q.Get("node_key"),
		ProfileID:       q.Get("profile_id"),
		ProbeType:       q.Get("probe_type"),
		Target:          q.Get("target"),
		Cursor:          q.Get("cursor"),
	}

	if filter.Cursor != "" && len(filter.Cursor) > 512 {
		writeError(w, http.StatusBadRequest, "cursor token exceeds maximum length of 512 bytes")
		return
	}

	if limitStr := q.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 || limit > 1000 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 1000")
			return
		}
		filter.Limit = limit
	}
	if successStr := q.Get("success"); successStr != "" {
		val := successStr == "true" || successStr == "1"
		filter.Success = &val
	}
	if orderDescStr := q.Get("order_desc"); orderDescStr != "" {
		filter.OrderDesc = orderDescStr == "true" || orderDescStr == "1"
	} else {
		filter.OrderDesc = true // Default newest first
	}
	if sinceStr := q.Get("since"); sinceStr != "" {
		t, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid since parameter: must be RFC3339")
			return
		}
		filter.Since = &t
	}
	if untilStr := q.Get("until"); untilStr != "" {
		t, err := time.Parse(time.RFC3339, untilStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid until parameter: must be RFC3339")
			return
		}
		filter.Until = &t
	}
	if filter.Since != nil && filter.Until != nil && filter.Since.After(*filter.Until) {
		writeError(w, http.StatusBadRequest, monitor.ErrInvalidTimeRange.Error())
		return
	}

	page, err := s.app.QueryMonitorSamplesCursor(r.Context(), filter)
	if err != nil {
		if monitor.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleGetMonitorStats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := monitor.StatsQuery{
		NodeIdentityKey: q.Get("node_identity_key"),
		LegacyNodeKey:   q.Get("legacy_node_key"),
		NodeKey:         q.Get("node_key"),
		ProfileID:       q.Get("profile_id"),
		ProbeType:       q.Get("probe_type"),
		Target:          q.Get("target"),
	}
	if sinceStr := q.Get("since"); sinceStr != "" {
		t, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid since parameter: must be RFC3339")
			return
		}
		query.Since = &t
	}
	if untilStr := q.Get("until"); untilStr != "" {
		t, err := time.Parse(time.RFC3339, untilStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid until parameter: must be RFC3339")
			return
		}
		query.Until = &t
	}
	if query.Since != nil && query.Until != nil && query.Since.After(*query.Until) {
		writeError(w, http.StatusBadRequest, monitor.ErrInvalidTimeRange.Error())
		return
	}

	stats, err := s.app.GetMonitorStats(r.Context(), query)
	if err != nil {
		if monitor.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleApplyRetention(w http.ResponseWriter, r *http.Request) {
	var req monitor.RetentionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}

	result, err := s.app.ApplyRetention(r.Context(), req)
	if err != nil {
		var retErr *monitor.RetentionError
		if errors.As(err, &retErr) && retErr.Result != nil && retErr.Result.Partial {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error":           retErr.Err.Error(),
				"partial":         true,
				"samples_deleted": retErr.Result.SamplesDeleted,
				"runs_deleted":    retErr.Result.RunsDeleted,
				"cutoff":          retErr.Result.Cutoff,
				"policy":          retErr.Result.Policy,
			})
			return
		}
		if monitor.IsValidationError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

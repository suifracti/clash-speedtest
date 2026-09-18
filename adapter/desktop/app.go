package desktop

import (
	"context"
	"fmt"
	"os"
	"strings"

	"time"

	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/auth"
	"github.com/faceair/clash-speedtest/core/controller"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/policy"
	"github.com/faceair/clash-speedtest/core/profiles"
)

// App is the desktop adapter struct exposed to the Vue 3 frontend via Wails IPC.
type App struct {
	ctx          context.Context
	app          *application.AppService
	emitter      *WailsEventEmitter
	profilePaths profiles.Paths
	userAgent    string
}

// NewApp creates a new desktop Wails binding application.
func NewApp(hStore *history.Store, paths profiles.Paths, userAgent string) *App {
	emitter := NewWailsEventEmitter()
	appSvc := application.NewAppService(hStore, paths, emitter)

	return &App{
		app:          appSvc,
		emitter:      emitter,
		profilePaths: paths,
		userAgent:    userAgent,
	}
}

// Startup is called by Wails when the application starts.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.emitter.SetContext(ctx)
}

// Shutdown is called by Wails on termination.
func (a *App) Shutdown(ctx context.Context) {
	_ = a.app.Close()
}

// --- Test Operations ---

func (a *App) GetStatus() application.TestStatus {
	return a.app.Status()
}

func (a *App) StartBatch(req application.BatchTestRequest) error {
	return a.app.StartBatch(req, "")
}

func (a *App) TestSingle(req application.SingleTestRequest) (*history.RunNodeResult, error) {
	return a.app.TestSingle(req, "")
}

func (a *App) StopTest() {
	a.app.Stop()
}

// --- Airport Operations ---

func (a *App) ListAirports() ([]application.AirportDTO, error) {
	return a.app.ListAirports()
}

func (a *App) CreateAirport(name, url string) (*application.AirportDTO, error) {
	return a.app.CreateAirport(name, url, a.userAgent)
}

func (a *App) UpdateAirport(id, name, url string) (*application.AirportDTO, error) {
	return a.app.UpdateAirport(id, name, url, a.userAgent)
}

func (a *App) DeleteAirport(id string) error {
	return a.app.DeleteAirport(id)
}

func (a *App) RefreshAirport(id string) (*application.AirportDTO, error) {
	return a.app.RefreshAirport(id, a.userAgent)
}

func (a *App) GetAirportNodes(airportID string) ([]map[string]any, error) {
	return a.app.GetAirportNodes(airportID)
}

// --- History Operations ---

func (a *App) ListHistory() ([]*history.RunSummary, error) {
	return a.app.ListHistory()
}

func (a *App) GetHistory(id string) (*history.TestRun, error) {
	return a.app.GetHistory(id)
}

func (a *App) DeleteHistory(id string) error {
	return a.app.DeleteHistory(id)
}

func (a *App) GetAirportTimeline(airportID string) (*history.AirportHistory, error) {
	return a.app.GetAirportTimeline(airportID)
}

func (a *App) CompareRuns(baseID, targetID string) (*history.RunComparison, error) {
	return a.app.CompareRuns(baseID, targetID)
}

// --- Auth & Token Operations ---

func (a *App) GetTokenStatus() application.TokenStatusDTO {
	return a.app.GetTokenStatus()
}

func (a *App) SetToken(token string) error {
	parsed := auth.ParseAntigravityToken(strings.TrimSpace(token))
	if parsed == "" {
		return fmt.Errorf("Token 格式无效，请检查")
	}
	a.app.SetAntigravityToken(parsed, "手动输入")
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		_ = os.WriteFile(home+"/.antigravity_token", []byte(parsed+"\n"), 0600)
	}
	return nil
}

func (a *App) StartOAuthLogin() error {
	go func() {
		token, err := auth.StartOAuthBrowserFlow(os.Stdout)
		if err != nil {
			a.emitter.Emit(application.Event{
				Type: "antigravity_login_failed",
				Payload: map[string]any{
					"error": err.Error(),
				},
			})
			return
		}
		a.app.SetAntigravityToken(token, "Google OAuth 登录")
	}()
	return nil
}

// --- Export Operations ---

func (a *App) ExportClashConfig(airportID string, nodeNames []string) (string, error) {
	yamlBytes, err := a.app.ExportClashConfig(airportID, nodeNames)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (a *App) ExportClashConfigFromResults(runID string, nodeNames []string) (string, error) {
	yamlBytes, err := a.app.ExportClashConfigFromResults(runID, nodeNames)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

// --- Settings Operations ---

func (a *App) GetSettings() (*application.AppSettings, error) {
	return a.app.GetSettings()
}

func (a *App) SaveSettings(settings *application.AppSettings) error {
	return a.app.SaveSettings(settings)
}

func (a *App) OpenURL(targetURL string) error {
	return auth.OpenBrowser(targetURL)
}

func (a *App) context() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

// --- Controller & Smart Orchestrator Operations ---

func (a *App) GetControllerStatus() (application.ControllerStatusDTO, error) {
	return a.app.GetControllerStatus(a.context())
}

func (a *App) ConfigureController(cfg application.ControllerConfigDTO) error {
	return a.app.ConfigureController(a.context(), cfg)
}

func (a *App) ListControllerGroups() ([]controller.Group, error) {
	return a.app.ListControllerGroups(a.context())
}

func (a *App) SelectControllerNode(group string, nodeName string) error {
	return a.app.SelectControllerNode(a.context(), group, nodeName)
}

func (a *App) GetSwitchPolicy() (policy.SwitchPolicy, error) {
	return a.app.GetSwitchPolicy(a.context())
}

func (a *App) UpdateSwitchPolicy(p policy.SwitchPolicy) error {
	return a.app.UpdateSwitchPolicy(a.context(), p)
}

func (a *App) GetSwitchAuditTrail() ([]policy.SwitchEvent, error) {
	return a.app.GetSwitchAuditTrail(a.context())
}

// --- 24/7 Monitor Operations ---

func (a *App) CreateMonitorJob(job monitor.MonitorJob) (*monitor.MonitorJob, error) {
	return a.app.CreateMonitorJob(job)
}

func (a *App) StartMonitorJob(jobID string) error {
	return a.app.StartMonitorJob(jobID)
}

func (a *App) PauseMonitorJob(jobID string) error {
	return a.app.PauseMonitorJob(jobID)
}

func (a *App) ResumeMonitorJob(jobID string) error {
	return a.app.ResumeMonitorJob(jobID)
}

func (a *App) StopMonitorJob(jobID string) error {
	return a.app.StopMonitorJob(jobID)
}

func (a *App) GetMonitorJob(jobID string) (*monitor.MonitorJob, error) {
	return a.app.GetMonitorJob(jobID)
}

func (a *App) ListMonitorJobs() []monitor.MonitorJob {
	return a.app.ListMonitorJobs()
}

func (a *App) TriggerMonitorJob(jobID string) (*monitor.MonitorRun, error) {
	return a.app.TriggerMonitorJob(jobID)
}

func (a *App) QueryMonitorSamples(filter monitor.SampleFilter) ([]*monitor.MonitorSample, error) {
	return a.app.QueryMonitorSamples(a.context(), filter)
}

func (a *App) QueryMonitorRuns(jobID string, limit int) ([]*monitor.MonitorRun, error) {
	return a.app.QueryMonitorRuns(a.context(), jobID, limit)
}

func (a *App) GetNodeTimelineSamples(nodeKey string, since time.Time) ([]*monitor.MonitorSample, error) {
	return a.app.GetNodeTimelineSamples(a.context(), nodeKey, since)
}

func (a *App) QueryMonitorSamplesCursor(filter monitor.CursorFilter) (*monitor.SampleCursorPage, error) {
	return a.app.QueryMonitorSamplesCursor(a.context(), filter)
}

func (a *App) GetMonitorStats(query monitor.StatsQuery) (*monitor.DerivedStats, error) {
	return a.app.GetMonitorStats(a.context(), query)
}

func (a *App) ApplyRetention(req monitor.RetentionRequest) (*monitor.RetentionResult, error) {
	return a.app.ApplyRetention(a.context(), req)
}

// GetMonitorSampleFacets returns the distinct filter dimensions present in raw samples.
// Presentation-only read model used to populate the timeline filter controls.
func (a *App) GetMonitorSampleFacets(since, until *time.Time) (*monitor.MonitorSampleFacets, error) {
	return a.app.GetMonitorSampleFacets(a.context(), since, until)
}

// GetMonitorRecommendation returns a read-only, evidence-backed node recommendation.
//
// It consumes persisted monitor evidence and the configured policy, and produces an
// explainable recommendation. It never switches the active node (SelectNodeCalls == 0),
// never mutates the controller, and never triggers a monitor run.
func (a *App) GetMonitorRecommendation(req application.MonitorRecommendationRequest) (*policy.MonitorRecommendation, error) {
	return a.app.GetMonitorRecommendation(a.context(), req)
}

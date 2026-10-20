package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"time"

	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/appdata"
	"github.com/faceair/clash-speedtest/core/auth"
	"github.com/faceair/clash-speedtest/core/controller"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/policy"
	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/publicservice"
)

var errAppNotInitialized = fmt.Errorf("应用服务未初始化")

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
	return NewAppWithPaths(hStore, appdata.FromLegacy(paths.Dir, ""), userAgent)
}

// NewAppWithPaths wires the Wails binding to the same resolved paths used by
// the local Web adapter.
func NewAppWithPaths(hStore *history.Store, paths appdata.AppPaths, userAgent string) *App {
	return NewAppWithOptions(hStore, paths, userAgent, application.Options{})
}

func NewAppWithOptions(hStore *history.Store, paths appdata.AppPaths, userAgent string, options application.Options) *App {
	emitter := NewWailsEventEmitter()
	appSvc := application.NewAppServiceWithOptions(hStore, paths, emitter, options)

	return &App{
		app:          appSvc,
		emitter:      emitter,
		profilePaths: profiles.Paths{Dir: paths.ProfileDir},
		userAgent:    userAgent,
	}
}

// Startup is called by Wails when the application starts.
func (a *App) Startup(ctx context.Context) {
	if a == nil {
		return
	}
	a.ctx = ctx
	if a.emitter != nil {
		a.emitter.SetContext(ctx)
	}
	if a.app != nil {
		if err := a.app.RecoverMonitorJobs(ctx); err != nil {
			log.Printf("Monitor startup recovery did not run: %v", err)
		}
	}
}

// Shutdown is called by Wails on termination.
func (a *App) Shutdown(ctx context.Context) {
	if a != nil && a.app != nil {
		_ = a.app.Close()
	}
}

// --- Test Operations ---

func (a *App) GetStatus() application.TestStatus {
	if a == nil || a.app == nil {
		return application.TestStatus{}
	}
	return a.app.Status()
}

func (a *App) StartBatch(req application.BatchTestRequest) error {
	if a == nil || a.app == nil {
		return errAppNotInitialized
	}
	return a.app.StartBatch(req, "")
}

func (a *App) TestSingle(req application.SingleTestRequest) (*history.RunNodeResult, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.TestSingle(req, "")
}

// RunWorkbenchLatencyTest executes the stable-identity single-node workbench path.
func (a *App) RunWorkbenchLatencyTest(req application.WorkbenchLatencyTestRequest) (*application.WorkbenchLatencyTestDTO, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.RunWorkbenchLatencyTest(a.context(), req)
}

func (a *App) ListWorkbenchLatencyTests(query application.WorkbenchLatencyHistoryQuery) (application.WorkbenchLatencyHistoryResult, error) {
	if a == nil || a.app == nil {
		return application.WorkbenchLatencyHistoryResult{}, errAppNotInitialized
	}
	return a.app.ListWorkbenchLatencyTests(a.context(), query)
}

func (a *App) GetWorkbenchLatencyTest(query application.WorkbenchLatencyHistoryDetailQuery) (*application.WorkbenchLatencyTestDTO, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.GetWorkbenchLatencyTest(a.context(), query)
}

func (a *App) ListWorkbenchPublicServiceCatalog() []publicservice.Rule {
	if a == nil || a.app == nil {
		return nil
	}
	return a.app.ListWorkbenchPublicServiceCatalog()
}

func (a *App) StartWorkbenchPublicServiceTest(req application.WorkbenchPublicServiceTestRequest) (*history.PublicServiceAttempt, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	result, err := a.app.StartWorkbenchPublicServiceTest(a.context(), req)
	return result, desktopStartError(err)
}

func (a *App) ListWorkbenchPublicServiceTests(query application.WorkbenchPublicServiceHistoryQuery) (application.WorkbenchPublicServiceHistoryResult, error) {
	if a == nil || a.app == nil {
		return application.WorkbenchPublicServiceHistoryResult{}, errAppNotInitialized
	}
	return a.app.ListWorkbenchPublicServiceTests(a.context(), query)
}

func (a *App) GetWorkbenchPublicServiceAttempt(attemptID string, query application.WorkbenchPublicServiceHistoryQuery) (*history.PublicServiceAttempt, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.GetWorkbenchPublicServiceAttempt(a.context(), attemptID, query)
}

func (a *App) PreviousWorkbenchPublicServiceAttempt(attemptID string, query application.WorkbenchPublicServiceHistoryQuery) (*history.PublicServiceAttempt, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.PreviousWorkbenchPublicServiceAttempt(a.context(), attemptID, query)
}

func (a *App) CancelWorkbenchPublicServiceTest(attemptID string, query application.WorkbenchPublicServiceHistoryQuery) (*history.PublicServiceAttempt, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.CancelWorkbenchPublicServiceTest(a.context(), attemptID, query)
}

func (a *App) RetrySaveWorkbenchPublicServiceTest(attemptID string, query application.WorkbenchPublicServiceHistoryQuery) (*history.PublicServiceAttempt, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.RetrySaveWorkbenchPublicServiceTest(a.context(), attemptID, query)
}

func (a *App) StartWorkbenchDownloadTest(req application.WorkbenchDownloadTestRequest) (*history.WorkbenchDownloadAttempt, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	result, err := a.app.StartWorkbenchDownloadTest(a.context(), req)
	return result, desktopStartError(err)
}

func (a *App) ListWorkbenchDownloadTests(query application.WorkbenchDownloadHistoryQuery) (application.WorkbenchDownloadHistoryResult, error) {
	if a == nil || a.app == nil {
		return application.WorkbenchDownloadHistoryResult{}, errAppNotInitialized
	}
	return a.app.ListWorkbenchDownloadTests(a.context(), query)
}

func (a *App) GetWorkbenchDownloadAttempt(attemptID string, query application.WorkbenchDownloadHistoryQuery) (*history.WorkbenchDownloadAttempt, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.GetWorkbenchDownloadAttempt(a.context(), attemptID, query)
}

func (a *App) PreviousWorkbenchDownloadAttempt(attemptID string, query application.WorkbenchDownloadHistoryQuery) (*history.WorkbenchDownloadAttempt, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.PreviousWorkbenchDownloadAttempt(a.context(), attemptID, query)
}

func (a *App) CancelWorkbenchDownloadTest(attemptID string, query application.WorkbenchDownloadHistoryQuery) (*history.WorkbenchDownloadAttempt, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.CancelWorkbenchDownloadTest(a.context(), attemptID, query)
}

func (a *App) RetrySaveWorkbenchDownloadTest(attemptID string, query application.WorkbenchDownloadHistoryQuery) (*history.WorkbenchDownloadAttempt, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.RetrySaveWorkbenchDownloadTest(a.context(), attemptID, query)
}

func (a *App) ListNodeHistoryRevisions(profileID, nodeIdentityKey string) ([]history.NodeHistoryRevision, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.ListNodeHistoryRevisions(a.context(), profileID, nodeIdentityKey)
}

func (a *App) StartWorkbenchLatencyBatch(req application.WorkbenchLatencyBatchRequest) (*application.WorkbenchLatencyBatchDTO, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	result, err := a.app.StartWorkbenchLatencyBatch(a.context(), req)
	return result, desktopStartError(err)
}

func (a *App) ListWorkbenchLatencyBatches(limit int) ([]application.WorkbenchLatencyBatchDTO, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.ListWorkbenchLatencyBatches(a.context(), limit)
}

func (a *App) GetWorkbenchLatencyBatch(batchID string) (*application.WorkbenchLatencyBatchDTO, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.GetWorkbenchLatencyBatch(a.context(), batchID)
}

func (a *App) CancelWorkbenchLatencyBatch(batchID string) (*application.WorkbenchLatencyBatchDTO, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.CancelWorkbenchLatencyBatch(batchID)
}

func (a *App) RetryWorkbenchLatencyBatchItem(batchID, itemID string) (*application.WorkbenchLatencyBatchDTO, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.RetryWorkbenchLatencyBatchItem(a.context(), batchID, itemID)
}

func (a *App) StopTest() {
	if a != nil && a.app != nil {
		a.app.Stop()
	}
}

// --- Airport Operations ---

func (a *App) GetProfileSetup() (*application.ProfileSetupDTO, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.GetProfileSetup()
}

func (a *App) MigrateLegacyData() error {
	if a == nil || a.app == nil {
		return errAppNotInitialized
	}
	return a.app.MigrateLegacyData()
}

func (a *App) InspectProfileSource(path string) (*application.ProfileSourceDTO, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.InspectProfileSource(path)
}

func (a *App) InitializeEmptyProfileStore() error {
	if a == nil || a.app == nil {
		return errAppNotInitialized
	}
	return a.app.InitializeEmptyProfileStore()
}

func (a *App) ImportProfileSource(path string) error {
	if a == nil || a.app == nil {
		return errAppNotInitialized
	}
	return a.app.ImportProfileSource(a.context(), path)
}

func (a *App) DiscardProfileImport() error {
	if a == nil || a.app == nil {
		return errAppNotInitialized
	}
	return a.app.DiscardProfileImport()
}

func (a *App) ListAirports() ([]application.AirportDTO, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.ListAirports()
}

// GetAirportURL is an explicit management read used by the edit flow. The
// ordinary AirportDTO intentionally does not contain the complete source.
func (a *App) GetAirportURL(id string) (string, error) {
	if a == nil || a.app == nil {
		return "", errAppNotInitialized
	}
	return a.app.GetAirportURL(id)
}

func (a *App) CreateAirport(name, url string, optionalLinks ...string) (*application.AirportDTO, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.CreateAirport(name, url, a.userAgent, optionalLinks...)
}

func (a *App) UpdateAirport(id, name, url string, optionalLinks ...string) (*application.AirportDTO, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.UpdateAirport(id, name, url, a.userAgent, optionalLinks...)
}

func (a *App) DeleteAirport(id string) error {
	if a == nil || a.app == nil {
		return errAppNotInitialized
	}
	return a.app.DeleteAirport(id)
}

func (a *App) RefreshAirport(id string) (*application.AirportDTO, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.RefreshAirport(id, a.userAgent)
}

func (a *App) GetAirportNodes(airportID string) ([]map[string]any, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.GetAirportNodes(airportID)
}

// --- History Operations ---

func (a *App) ListHistory() ([]*history.RunSummary, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.ListHistory()
}

func (a *App) GetHistory(id string) (*history.TestRun, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.GetHistory(id)
}

func (a *App) DeleteHistory(id string) error {
	if a == nil || a.app == nil {
		return errAppNotInitialized
	}
	return a.app.DeleteHistory(id)
}

func (a *App) GetAirportTimeline(airportID string) (*history.AirportHistory, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.GetAirportTimeline(airportID)
}

func (a *App) CompareRuns(baseID, targetID string) (*history.RunComparison, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.CompareRuns(baseID, targetID)
}

// --- Auth & Token Operations ---

func (a *App) GetTokenStatus() application.TokenStatusDTO {
	if a == nil || a.app == nil {
		return application.TokenStatusDTO{}
	}
	return a.app.GetTokenStatus()
}

func (a *App) DiscoverToken() (application.TokenStatusDTO, error) {
	if a == nil || a.app == nil {
		return application.TokenStatusDTO{}, errAppNotInitialized
	}
	return a.app.DiscoverAntigravityToken()
}

func (a *App) SetToken(token string) error {
	if a == nil || a.app == nil {
		return errAppNotInitialized
	}
	parsed := auth.ParseAntigravityToken(strings.TrimSpace(token))
	if parsed == "" {
		return fmt.Errorf("Token 格式无效，请检查")
	}
	return a.app.SetAntigravityToken(parsed, "手动输入")
}

func (a *App) StartOAuthLogin() error {
	if a == nil || a.app == nil {
		return errAppNotInitialized
	}
	go func() {
		token, err := auth.StartOAuthBrowserFlow(os.Stdout)
		if err != nil {
			if a != nil && a.emitter != nil {
				a.emitter.Emit(application.Event{
					Type: "antigravity_login_failed",
					Payload: map[string]any{
						"error": err.Error(),
					},
				})
			}
			return
		}
		if a != nil && a.app != nil {
			if err := a.app.SetAntigravityToken(token, "Google OAuth 登录"); err != nil && a.emitter != nil {
				a.emitter.Emit(application.Event{Type: "antigravity_login_failed", Payload: map[string]any{"error": err.Error()}})
			}
		}
	}()
	return nil
}

// --- Export Operations ---

func (a *App) ExportClashConfig(airportID string, nodeNames []string) (string, error) {
	if a == nil || a.app == nil {
		return "", errAppNotInitialized
	}
	yamlBytes, err := a.app.ExportClashConfig(airportID, nodeNames)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (a *App) ExportClashConfigFromResults(runID string, nodeNames []string) (string, error) {
	if a == nil || a.app == nil {
		return "", errAppNotInitialized
	}
	yamlBytes, err := a.app.ExportClashConfigFromResults(runID, nodeNames)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

// --- Settings Operations ---

func (a *App) GetSettings() (*application.AppSettings, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.GetSettings()
}

func (a *App) SaveSettings(settings *application.AppSettings) error {
	if a == nil || a.app == nil {
		return errAppNotInitialized
	}
	return a.app.SaveSettings(settings)
}

func (a *App) OpenURL(targetURL string) error {
	return auth.OpenBrowser(targetURL)
}

func (a *App) context() context.Context {
	if a != nil && a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

// --- Controller & Smart Orchestrator Operations ---

func (a *App) GetControllerStatus() (application.ControllerStatusDTO, error) {
	if a == nil || a.app == nil {
		return application.ControllerStatusDTO{}, errAppNotInitialized
	}
	return a.app.GetControllerStatus(a.context())
}

func (a *App) ConfigureController(cfg application.ControllerConfigDTO) error {
	if a == nil || a.app == nil {
		return errAppNotInitialized
	}
	return a.app.ConfigureController(a.context(), cfg)
}

func (a *App) ListControllerGroups() ([]controller.Group, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.ListControllerGroups(a.context())
}

func (a *App) SelectControllerNode(group string, nodeName string) error {
	if a == nil || a.app == nil {
		return errAppNotInitialized
	}
	return a.app.SelectControllerNode(a.context(), group, nodeName)
}

func (a *App) GetSwitchPolicy() (policy.SwitchPolicy, error) {
	if a == nil || a.app == nil {
		return policy.SwitchPolicy{}, errAppNotInitialized
	}
	return a.app.GetSwitchPolicy(a.context())
}

func (a *App) UpdateSwitchPolicy(p policy.SwitchPolicy) error {
	if a == nil || a.app == nil {
		return errAppNotInitialized
	}
	return a.app.UpdateSwitchPolicy(a.context(), p)
}

func (a *App) GetSwitchAuditTrail() ([]policy.SwitchEvent, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.GetSwitchAuditTrail(a.context())
}

// --- 24/7 Monitor Operations ---

func (a *App) ListMonitorNodeOptions() ([]application.MonitorNodeOptionDTO, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.ListMonitorNodeOptions()
}

func (a *App) CreateMonitorJob(req application.MonitorJobCreateRequest) (*application.MonitorJobDTO, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.CreateMonitorJobFromRequest(req)
}

func (a *App) UpdateMonitorJobSamplingTier(jobID string, tier monitor.SamplingTier) error {
	if a == nil || a.app == nil {
		return errAppNotInitialized
	}
	return a.app.UpdateMonitorJobSamplingTier(jobID, tier)
}

func (a *App) SetMonitorJobResumeOnLaunch(jobID string, enabled bool) error {
	if a == nil || a.app == nil {
		return errAppNotInitialized
	}
	return a.app.SetMonitorJobResumeOnLaunch(jobID, enabled)
}

func (a *App) DeleteMonitorJob(jobID string) error {
	if a == nil || a.app == nil {
		return errAppNotInitialized
	}
	return a.app.DeleteMonitorJob(jobID)
}

func (a *App) StartMonitorJob(jobID string) error {
	if a == nil || a.app == nil {
		return errAppNotInitialized
	}
	return a.app.StartMonitorJob(jobID)
}

func (a *App) PauseMonitorJob(jobID string) error {
	if a == nil || a.app == nil {
		return errAppNotInitialized
	}
	return a.app.PauseMonitorJob(jobID)
}

func (a *App) ResumeMonitorJob(jobID string) error {
	if a == nil || a.app == nil {
		return errAppNotInitialized
	}
	return a.app.ResumeMonitorJob(jobID)
}

func (a *App) StopMonitorJob(jobID string) error {
	if a == nil || a.app == nil {
		return errAppNotInitialized
	}
	return a.app.StopMonitorJob(jobID)
}

func (a *App) GetMonitorJob(jobID string) (*application.MonitorJobDTO, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.GetMonitorJobDTO(jobID)
}

func (a *App) ListMonitorJobs() ([]application.MonitorJobDTO, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.ListMonitorJobDTOs()
}

func (a *App) TriggerMonitorJob(jobID string) (*monitor.MonitorRun, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.TriggerMonitorJob(jobID)
}

func (a *App) QueryMonitorSamples(filter monitor.SampleFilter) ([]*monitor.MonitorSample, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.QueryMonitorSamples(a.context(), filter)
}

func (a *App) QueryMonitorRuns(jobID string, limit int) ([]*monitor.MonitorRun, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.QueryMonitorRuns(a.context(), jobID, limit)
}

func (a *App) GetNodeTimelineSamples(nodeKey string, since time.Time) ([]*monitor.MonitorSample, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.GetNodeTimelineSamples(a.context(), nodeKey, since)
}

func (a *App) QueryMonitorSamplesCursor(filter monitor.CursorFilter) (*monitor.SampleCursorPage, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.QueryMonitorSamplesCursor(a.context(), filter)
}

func (a *App) GetMonitorStats(query monitor.StatsQuery) (*monitor.DerivedStats, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.GetMonitorStats(a.context(), query)
}

func (a *App) ApplyRetention(req monitor.RetentionRequest) (*monitor.RetentionResult, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.ApplyRetention(a.context(), req)
}

func (a *App) PreviewMonitorRetention(req monitor.RetentionRequest) (*monitor.RetentionPreview, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.PreviewMonitorRetention(a.context(), req)
}

func (a *App) GetMonitorStorageUsage() (*monitor.StorageUsage, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.GetMonitorStorageUsage()
}

func (a *App) GetMonitorBudgetStatus() (*monitor.BudgetStatus, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.GetMonitorBudgetStatus(a.context())
}

// GetMonitorSampleFacets returns the distinct filter dimensions present in raw samples.
// Presentation-only read model used to populate the timeline filter controls.
func (a *App) GetMonitorSampleFacets(since, until *time.Time) (*monitor.MonitorSampleFacets, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.GetMonitorSampleFacets(a.context(), since, until)
}

// GetMonitorRecommendation returns a read-only, evidence-backed node recommendation.
//
// It consumes persisted monitor evidence and the configured policy, and produces an
// explainable recommendation. It never switches the active node (SelectNodeCalls == 0),
// never mutates the controller, and never triggers a monitor run.
func (a *App) GetMonitorRecommendation(req application.MonitorRecommendationRequest) (*policy.MonitorRecommendation, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.GetMonitorRecommendation(a.context(), req)
}

// Wails serializes error strings. Only typed validation failures prove rejection;
// storage/transport/unknown errors must retain the original request for lookup.
func desktopStartError(err error) error {
	if err == nil || !monitor.IsValidationError(err) {
		return err
	}
	payload, _ := json.Marshal(struct {
		Admission string `json:"admission"`
		Status    int    `json:"status"`
		Message   string `json:"message"`
	}{"not_accepted", 400, err.Error()})
	return fmt.Errorf("CST_START_ERROR:%s", payload)
}

func (a *App) GetSubscriptionNodeChanges(airportID, subID string) (*profiles.SubscriptionNodeChanges, error) {
	if a == nil || a.app == nil {
		return nil, errAppNotInitialized
	}
	return a.app.GetSubscriptionNodeChanges(airportID, subID)
}

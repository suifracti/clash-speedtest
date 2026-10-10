package application

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/speedtester"
	"github.com/faceair/clash-speedtest/core/subscriptionusage"
)

type RefreshSelection struct {
	AirportID      string `json:"airport_id"`
	SubscriptionID string `json:"subscription_id"`
}

type SubscriptionRefreshRequest struct {
	RequestID  string             `json:"request_id"`
	All        bool               `json:"all"`
	Selections []RefreshSelection `json:"selections,omitempty"`
	RetryOf    string             `json:"retry_of,omitempty"`
}

type RefreshStage struct {
	State string    `json:"state"`
	At    time.Time `json:"at"`
}

type SubscriptionRefreshItem struct {
	RefreshSelection
	AirportName       string                  `json:"airport_name"`
	SubscriptionName  string                  `json:"subscription_name"`
	SourceFingerprint string                  `json:"source_fingerprint"`
	SourceVersion     int                     `json:"source_version"`
	State             string                  `json:"state"`
	ErrorCode         string                  `json:"error_code,omitempty"`
	ErrorMessage      string                  `json:"error_message,omitempty"`
	StartedAt         time.Time               `json:"started_at,omitempty"`
	FinishedAt        time.Time               `json:"finished_at,omitempty"`
	DurationMS        int64                   `json:"duration_ms"`
	NodeCount         int                     `json:"node_count"`
	Fetch             *profiles.FetchEvidence `json:"fetch,omitempty"`
	Stages            []RefreshStage          `json:"stages"`
}

type SubscriptionRefreshJob struct {
	ID               string                    `json:"id"`
	State            string                    `json:"state"`
	CreatedAt        time.Time                 `json:"created_at"`
	FinishedAt       time.Time                 `json:"finished_at,omitempty"`
	Items            []SubscriptionRefreshItem `json:"items"`
	Completed        int                       `json:"completed"`
	Success          int                       `json:"success"`
	Failed           int                       `json:"failed"`
	Cancelled        int                       `json:"cancelled"`
	NotExecuted      int                       `json:"not_executed"`
	CancelRequested  bool                      `json:"cancel_requested"`
	PersistenceError string                    `json:"persistence_error,omitempty"`
}

type refreshError struct {
	Code    string
	Message string
}

func (e *refreshError) Error() string { return e.Message }

func (s *AppService) saveSubscriptionRefreshStore(store *profiles.Store) error {
	if s.subscriptionRefreshStoreSaveHook != nil {
		return s.subscriptionRefreshStoreSaveHook(s.profilePaths.StoreFile(), store)
	}
	return profiles.SaveStore(s.profilePaths.StoreFile(), store)
}

func refreshFailure(err error) *refreshError {
	var own *refreshError
	if errors.As(err, &own) {
		return own
	}
	if errors.Is(err, context.Canceled) {
		return &refreshError{Code: "cancelled", Message: "已取消当前请求；旧缓存保留"}
	}
	var body *profiles.FetchBodyError
	if errors.As(err, &body) {
		return &refreshError{Code: "parse", Message: "订阅响应为空或超过 8 MiB 上限；旧缓存保留"}
	}
	var httpErr *profiles.FetchHTTPError
	if errors.As(err, &httpErr) {
		message := fmt.Sprintf("订阅服务器返回 HTTP %d", httpErr.Status)
		switch {
		case httpErr.Status == http.StatusUnauthorized:
			message = "订阅认证失败（HTTP 401）"
		case httpErr.Status == http.StatusForbidden:
			message = "订阅访问被拒绝（HTTP 403）"
		case httpErr.Status == http.StatusTooManyRequests:
			message = "订阅源限流（HTTP 429，非节点失效）"
		case httpErr.Status >= 500:
			message = fmt.Sprintf("订阅服务器错误（HTTP %d）", httpErr.Status)
		}
		return &refreshError{Code: fmt.Sprintf("http_%d", httpErr.Status), Message: message + "；旧缓存保留"}
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return &refreshError{Code: "dns", Message: "订阅域名解析失败；旧缓存保留"}
	}
	var cert x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var host x509.HostnameError
	var tlsVerify *tls.CertificateVerificationError
	if errors.As(err, &cert) || errors.As(err, &invalid) || errors.As(err, &host) || errors.As(err, &tlsVerify) {
		return &refreshError{Code: "tls", Message: "订阅 HTTPS 证书验证失败；旧缓存保留"}
	}
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
		return &refreshError{Code: "timeout", Message: "订阅请求超时；旧缓存保留"}
	}
	return &refreshError{Code: "connect", Message: "订阅连接或传输失败；旧缓存保留"}
}

func refreshSourceFingerprint(rawURL string) string {
	value := sha256.Sum256([]byte("subscription-refresh-source-v1\x00" + strings.TrimSpace(rawURL)))
	return hex.EncodeToString(value[:])
}

type refreshRecoveryCopy struct {
	target  string
	path    string
	existed bool
}

func createRefreshRecoveryCopy(target string) (*refreshRecoveryCopy, error) {
	copy := &refreshRecoveryCopy{target: target}
	source, err := os.Open(target)
	if os.IsNotExist(err) {
		return copy, nil
	}
	if err != nil {
		return nil, err
	}
	defer source.Close()

	backup, err := os.CreateTemp(filepath.Dir(target), ".refresh-recovery-*")
	if err != nil {
		return nil, err
	}
	backupPath := backup.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(backupPath)
		}
	}()
	if err = backup.Chmod(0o600); err != nil {
		_ = backup.Close()
		return nil, err
	}
	if _, err = io.Copy(backup, source); err == nil {
		err = backup.Sync()
	}
	closeErr := backup.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if err = syncRefreshDirectory(filepath.Dir(target)); err != nil {
		return nil, err
	}
	copy.path = backupPath
	copy.existed = true
	keep = true
	return copy, nil
}

func (copy *refreshRecoveryCopy) discard() {
	if copy == nil || copy.path == "" {
		return
	}
	if err := os.Remove(copy.path); err == nil {
		_ = syncRefreshDirectory(filepath.Dir(copy.path))
	}
}

func (s *AppService) restoreRefreshRecoveryCopy(kind string, copy *refreshRecoveryCopy) error {
	if copy == nil {
		return fmt.Errorf("missing recovery copy")
	}
	if s.subscriptionRefreshRecoveryHook != nil {
		if err := s.subscriptionRefreshRecoveryHook(kind, copy.path, copy.target); err != nil {
			return err
		}
	}
	if !copy.existed {
		if err := os.Remove(copy.target); err != nil && !os.IsNotExist(err) {
			return err
		}
		return syncRefreshDirectory(filepath.Dir(copy.target))
	}

	source, err := os.Open(copy.path)
	if err != nil {
		return err
	}
	restored, err := os.CreateTemp(filepath.Dir(copy.target), ".refresh-restore-*")
	if err != nil {
		_ = source.Close()
		return err
	}
	restoredPath := restored.Name()
	defer os.Remove(restoredPath)
	if err = restored.Chmod(0o600); err != nil {
		_ = source.Close()
		_ = restored.Close()
		return err
	}
	if _, err = io.Copy(restored, source); err == nil {
		err = restored.Sync()
	}
	_ = source.Close()
	closeErr := restored.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(restoredPath, copy.target); err != nil {
		return err
	}
	if err = syncRefreshDirectory(filepath.Dir(copy.target)); err != nil {
		return err
	}
	copy.discard()
	return nil
}

func (s *AppService) restoreRefreshStoreBytes(original []byte) error {
	storePath := s.profilePaths.StoreFile()
	if s.subscriptionRefreshRecoveryHook != nil {
		if err := s.subscriptionRefreshRecoveryHook("store", "", storePath); err != nil {
			return err
		}
	}
	file, err := os.CreateTemp(filepath.Dir(storePath), ".refresh-restore-*")
	if err != nil {
		return err
	}
	temporaryPath := file.Name()
	defer os.Remove(temporaryPath)
	if err = file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err = file.Write(original); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(temporaryPath, storePath); err != nil {
		return err
	}
	return syncRefreshDirectory(filepath.Dir(storePath))
}

func syncRefreshDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	err = directory.Sync()
	closeErr := directory.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func refreshRollbackError(message string, rollbackErr error) *refreshError {
	if rollbackErr != nil {
		return &refreshError{Code: "rollback_unconfirmed", Message: "保存失败且恢复状态无法确认；请勿重复刷新并联系支持"}
	}
	return &refreshError{Code: "persist", Message: message}
}

func refreshRequestID(requestID string) string {
	value := sha256.Sum256([]byte("subscription-refresh-request-v1\x00" + requestID))
	return hex.EncodeToString(value[:])
}

func refreshTerminal(state string) bool {
	switch state {
	case "succeeded", "failed", "cancelled", "not_executed", "interrupted":
		return true
	default:
		return false
	}
}

func summarizeRefresh(job *SubscriptionRefreshJob) {
	job.Completed, job.Success, job.Failed, job.Cancelled, job.NotExecuted = 0, 0, 0, 0, 0
	for _, item := range job.Items {
		if refreshTerminal(item.State) {
			job.Completed++
		}
		switch item.State {
		case "succeeded":
			job.Success++
		case "failed", "interrupted":
			job.Failed++
		case "cancelled":
			job.Cancelled++
		case "not_executed":
			job.NotExecuted++
		}
	}
}

func markRefreshItemsNotExecuted(job *SubscriptionRefreshJob, first int, code, message string, at time.Time) {
	for index := first; index < len(job.Items); index++ {
		item := &job.Items[index]
		if refreshTerminal(item.State) {
			continue
		}
		if len(item.Stages) > 0 && item.Stages[len(item.Stages)-1].State == "fetching" {
			item.Stages = item.Stages[:len(item.Stages)-1]
		}
		item.StartedAt = time.Time{}
		item.DurationMS = 0
		item.State = "not_executed"
		item.ErrorCode = code
		item.ErrorMessage = message
		item.FinishedAt = at
		item.Stages = append(item.Stages, RefreshStage{State: item.State, At: at})
	}
}

func cloneRefresh(job *SubscriptionRefreshJob) *SubscriptionRefreshJob {
	data, _ := json.Marshal(job)
	var clone SubscriptionRefreshJob
	_ = json.Unmarshal(data, &clone)
	return &clone
}

func (s *AppService) refreshFile() string {
	return filepath.Join(s.profilePaths.Dir, "subscription-refresh-jobs.json")
}

func (s *AppService) saveRefreshLocked() error {
	data, err := json.MarshalIndent(s.refreshJobs, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(s.profilePaths.Dir, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(s.profilePaths.Dir, ".refresh-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), s.refreshFile())
}

func (s *AppService) loadRefreshLocked() error {
	if s.refreshLoaded {
		return nil
	}
	data, err := os.ReadFile(s.refreshFile())
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("刷新任务存储读取失败")
	}
	if len(data) > 0 {
		if err = json.Unmarshal(data, &s.refreshJobs); err != nil {
			return fmt.Errorf("刷新任务存储无效")
		}
	}
	s.refreshLoaded = true
	changed := false
	for _, job := range s.refreshJobs {
		if job.State != "running" {
			continue
		}
		changed = true
		job.State = "interrupted"
		job.FinishedAt = time.Now()
		for index := range job.Items {
			item := &job.Items[index]
			if refreshTerminal(item.State) {
				continue
			}
			state := "interrupted"
			if item.State == "queued" {
				state = "not_executed"
			}
			item.State = state
			item.ErrorCode = "interrupted"
			item.ErrorMessage = "程序上次中断；没有自动重发订阅请求"
			item.FinishedAt = job.FinishedAt
			item.Stages = append(item.Stages, RefreshStage{State: state, At: job.FinishedAt})
		}
		summarizeRefresh(job)
	}
	if changed {
		return s.saveRefreshLocked()
	}
	return nil
}

func (s *AppService) findRefreshLocked(id string) *SubscriptionRefreshJob {
	for _, job := range s.refreshJobs {
		if job.ID == id {
			return job
		}
	}
	return nil
}

func (s *AppService) ListSubscriptionRefreshJobs() ([]*SubscriptionRefreshJob, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	if err := s.loadRefreshLocked(); err != nil {
		return nil, err
	}
	jobs := make([]*SubscriptionRefreshJob, 0, len(s.refreshJobs))
	for index := len(s.refreshJobs) - 1; index >= 0; index-- {
		jobs = append(jobs, cloneRefresh(s.refreshJobs[index]))
	}
	return jobs, nil
}

func (s *AppService) GetSubscriptionRefresh(id string) (*SubscriptionRefreshJob, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	if err := s.loadRefreshLocked(); err != nil {
		return nil, err
	}
	job := s.findRefreshLocked(id)
	if job == nil {
		return nil, fmt.Errorf("刷新任务不存在")
	}
	return cloneRefresh(job), nil
}

func (s *AppService) StartSubscriptionRefreshJob(req SubscriptionRefreshRequest, userAgent string) (*SubscriptionRefreshJob, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	if s.refreshClosed {
		return nil, fmt.Errorf("程序正在关闭")
	}
	if err := s.loadRefreshLocked(); err != nil {
		return nil, err
	}
	if req.RequestID == "" || len(req.RequestID) > 120 || strings.ContainsAny(req.RequestID, "\\\n\r") {
		return nil, fmt.Errorf("刷新任务需要有效的请求标识")
	}
	jobID := refreshRequestID(req.RequestID)
	if existing := s.findRefreshLocked(jobID); existing != nil {
		return cloneRefresh(existing), nil
	}
	for _, existing := range s.refreshJobs {
		if existing.State == "running" {
			return nil, fmt.Errorf("已有刷新任务运行中，请查看进度")
		}
	}
	if req.RetryOf != "" && (req.All || len(req.Selections) > 0) {
		return nil, fmt.Errorf("重试失败项时不能指定其他刷新范围")
	}
	selections := append([]RefreshSelection(nil), req.Selections...)
	if req.RetryOf != "" {
		prior := s.findRefreshLocked(req.RetryOf)
		if prior == nil {
			return nil, fmt.Errorf("原刷新任务不存在")
		}
		selections = nil
		for _, item := range prior.Items {
			if item.State == "failed" {
				selections = append(selections, item.RefreshSelection)
			}
		}
		if len(selections) == 0 {
			return nil, fmt.Errorf("没有可重试的失败项")
		}
	}
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return nil, fmt.Errorf("订阅存储读取失败")
	}
	if req.All {
		if len(selections) > 0 {
			return nil, fmt.Errorf("刷新范围冲突")
		}
		for _, airport := range store.Airports {
			if airport == nil {
				continue
			}
			for _, sub := range airport.Subscriptions {
				if sub != nil && strings.TrimSpace(sub.URL) != "" {
					selections = append(selections, RefreshSelection{AirportID: airport.ID, SubscriptionID: sub.ID})
				}
			}
		}
	}
	if len(selections) == 0 || len(selections) > 500 {
		return nil, fmt.Errorf("请选择 1–500 个订阅")
	}
	job := &SubscriptionRefreshJob{ID: jobID, State: "running", CreatedAt: time.Now()}
	seen := make(map[string]bool, len(selections))
	for _, selection := range selections {
		selection.AirportID = strings.TrimSpace(selection.AirportID)
		selection.SubscriptionID = strings.TrimSpace(selection.SubscriptionID)
		key := selection.AirportID + "\x00" + selection.SubscriptionID
		if selection.AirportID == "" || selection.SubscriptionID == "" {
			return nil, fmt.Errorf("刷新范围缺少机场或订阅标识")
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		airport := store.Get(selection.AirportID)
		if airport == nil {
			return nil, fmt.Errorf("所选机场已不存在")
		}
		sub := airport.GetSubscription(selection.SubscriptionID)
		if sub == nil {
			return nil, fmt.Errorf("所选订阅已不存在")
		}
		job.Items = append(job.Items, SubscriptionRefreshItem{
			RefreshSelection:  selection,
			AirportName:       airport.Name,
			SubscriptionName:  sub.Name,
			SourceFingerprint: refreshSourceFingerprint(sub.URL),
			SourceVersion:     1,
			State:             "queued",
			Stages:            []RefreshStage{{State: "queued", At: job.CreatedAt}},
		})
	}
	if len(job.Items) == 0 {
		return nil, fmt.Errorf("没有可刷新的订阅")
	}
	s.refreshJobs = append(s.refreshJobs, job)
	if err = s.saveRefreshLocked(); err != nil {
		s.refreshJobs = s.refreshJobs[:len(s.refreshJobs)-1]
		return nil, fmt.Errorf("刷新任务无法保存；未发送请求")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.refreshCancel = cancel
	s.refreshWG.Add(1)
	go s.runSubscriptionRefresh(ctx, job, userAgent)
	return cloneRefresh(job), nil
}

func (s *AppService) runSubscriptionRefresh(ctx context.Context, job *SubscriptionRefreshJob, userAgent string) {
	defer s.refreshWG.Done()
	for index := range job.Items {
		s.refreshMu.Lock()
		if ctx.Err() != nil {
			code, message := "cancelled", "任务已取消；此订阅没有发出请求"
			if job.PersistenceError != "" {
				code, message = "persist", "刷新进度写入失败；此订阅没有发出请求"
			}
			markRefreshItemsNotExecuted(job, index, code, message, time.Now())
			summarizeRefresh(job)
			_ = s.saveRefreshLocked()
			s.refreshMu.Unlock()
			break
		}
		item := &job.Items[index]
		item.StartedAt = time.Now()
		item.State = "fetching"
		item.Stages = append(item.Stages, RefreshStage{State: "fetching", At: item.StartedAt})
		if err := s.saveRefreshLocked(); err != nil {
			job.PersistenceError = "刷新进度写入失败；请求已停止"
			markRefreshItemsNotExecuted(job, index, "persist", "刷新进度写入失败；此订阅没有发出请求", time.Now())
			summarizeRefresh(job)
			_ = s.saveRefreshLocked()
			s.refreshMu.Unlock()
			break
		}
		selection := item.RefreshSelection
		fingerprint := item.SourceFingerprint
		s.refreshMu.Unlock()

		itemCtx, cancel := context.WithTimeout(ctx, 75*time.Second)
		evidence := &profiles.FetchEvidence{}
		itemCtx = profiles.WithFetchEvidence(itemCtx, evidence)
		dto, refreshErr := s.refreshSubscriptionContext(itemCtx, selection.AirportID, selection.SubscriptionID, userAgent, fingerprint, func(state string) error {
			s.refreshMu.Lock()
			defer s.refreshMu.Unlock()
			item := &job.Items[index]
			item.State = state
			item.Stages = append(item.Stages, RefreshStage{State: state, At: time.Now()})
			if err := s.saveRefreshLocked(); err != nil {
				job.PersistenceError = "刷新进度写入失败；请求已停止"
				if s.refreshCancel != nil {
					s.refreshCancel()
				}
				return &refreshError{Code: "persist", Message: job.PersistenceError}
			}
			return nil
		})
		cancel()

		s.refreshMu.Lock()
		item = &job.Items[index]
		item.Fetch = evidence
		item.FinishedAt = time.Now()
		item.DurationMS = item.FinishedAt.Sub(item.StartedAt).Milliseconds()
		item.State = "succeeded"
		if refreshErr != nil {
			failure := refreshFailure(refreshErr)
			item.State = "failed"
			if failure.Code == "cancelled" {
				item.State = "cancelled"
			} else if failure.Code == "source_cooldown" || failure.Code == "configuration_changed" {
				item.State = "not_executed"
			}
			item.ErrorCode, item.ErrorMessage = failure.Code, failure.Message
		} else if dto != nil {
			item.NodeCount = dto.NodeCount
		}
		item.Stages = append(item.Stages, RefreshStage{State: item.State, At: item.FinishedAt})
		summarizeRefresh(job)
		if err := s.saveRefreshLocked(); err != nil {
			job.PersistenceError = "刷新结果写入失败，请核对磁盘"
			if s.refreshCancel != nil {
				s.refreshCancel()
			}
		}
		s.refreshMu.Unlock()
	}

	s.refreshMu.Lock()
	stopCode, stopMessage := "not_executed", "刷新任务提前结束；此订阅没有发出请求"
	if ctx.Err() != nil {
		stopCode, stopMessage = "cancelled", "任务已取消；此订阅没有发出请求"
	}
	if job.PersistenceError != "" {
		stopCode, stopMessage = "persist", "刷新进度写入失败；此订阅没有发出请求"
	}
	markRefreshItemsNotExecuted(job, 0, stopCode, stopMessage, time.Now())
	job.FinishedAt = time.Now()
	job.State = "finished"
	if ctx.Err() != nil {
		job.State = "cancelled"
	}
	summarizeRefresh(job)
	if err := s.saveRefreshLocked(); err != nil {
		job.PersistenceError = "最终刷新记录无法保存"
	}
	s.refreshCancel = nil
	s.refreshMu.Unlock()
}

func (s *AppService) CancelSubscriptionRefresh(id string) (*SubscriptionRefreshJob, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	if err := s.loadRefreshLocked(); err != nil {
		return nil, err
	}
	job := s.findRefreshLocked(id)
	if job == nil {
		return nil, fmt.Errorf("刷新任务不存在")
	}
	if job.State == "running" {
		job.CancelRequested = true
		if err := s.saveRefreshLocked(); err != nil {
			return nil, fmt.Errorf("取消请求无法保存")
		}
		if s.refreshCancel != nil {
			s.refreshCancel()
		}
	}
	return cloneRefresh(job), nil
}

func (s *AppService) closeSubscriptionRefreshJobs() {
	s.refreshMu.Lock()
	s.refreshClosed = true
	if s.refreshCancel != nil {
		s.refreshCancel()
	}
	s.refreshMu.Unlock()
	s.refreshWG.Wait()
}

func (s *AppService) refreshSubscriptionContext(ctx context.Context, airportID, subID, userAgent, expectedFingerprint string, stage func(string) error) (*SubscriptionDTO, error) {
	s.profileWriteMu.Lock()
	defer s.profileWriteMu.Unlock()
	return s.refreshSubscriptionLocked(ctx, airportID, subID, userAgent, expectedFingerprint, stage)
}

func (s *AppService) refreshSubscriptionLocked(ctx context.Context, airportID, subID, userAgent, expectedFingerprint string, stage func(string) error) (*SubscriptionDTO, error) {
	advance := func(name string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if stage != nil {
			return stage(name)
		}
		return nil
	}
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return nil, &refreshError{Code: "persist", Message: "读取订阅存储失败；旧缓存保留"}
	}
	airport := store.Get(airportID)
	if airport == nil {
		code := "configuration"
		if expectedFingerprint != "" {
			code = "configuration_changed"
		}
		return nil, &refreshError{Code: code, Message: "机场已不存在或配置已变化；请重新提交刷新"}
	}
	sub := airport.GetSubscription(subID)
	if sub == nil || strings.TrimSpace(sub.URL) == "" {
		code := "configuration"
		if expectedFingerprint != "" {
			code = "configuration_changed"
		}
		return nil, &refreshError{Code: code, Message: "订阅已不存在或配置已变化；请重新提交刷新"}
	}
	if expectedFingerprint != "" && refreshSourceFingerprint(sub.URL) != expectedFingerprint {
		return nil, &refreshError{Code: "configuration_changed", Message: "排队期间订阅配置已变化；请重新提交刷新"}
	}
	if profiles.IsHTTPURL(sub.URL) {
		origin := profiles.SourceOrigin(sub.URL)
		for _, candidateAirport := range store.Airports {
			if candidateAirport == nil {
				continue
			}
			for _, candidate := range candidateAirport.Subscriptions {
				if candidate == nil || profiles.SourceOrigin(candidate.URL) != origin || candidate.LastFailureCode != "http_429" {
					continue
				}
				retryAt := candidate.LastFailureRetryAt
				if retryAt.IsZero() {
					retryAt = candidate.LastFailureAt.Add(30 * time.Minute)
				}
				if !retryAt.IsZero() && time.Now().Before(retryAt) {
					return nil, &refreshError{Code: "source_cooldown", Message: "同源 HTTP 429 冷却中，未发出请求；请在冷却结束后重新提交刷新"}
				}
			}
		}
	}

	fail := func(cause error) (*SubscriptionDTO, error) {
		failure := refreshFailure(cause)
		if failure.Code == "cancelled" || failure.Code == "configuration_changed" || failure.Code == "source_cooldown" {
			return nil, failure
		}
		now := time.Now()
		sub.LastFailureAt = now
		sub.LastFailureRetryAt = time.Time{}
		sub.LastFailureCode = failure.Code
		sub.LastFailureMessage = failure.Message
		var status *profiles.FetchHTTPError
		if errors.As(cause, &status) && status.Status == http.StatusTooManyRequests {
			sub.LastFailureRetryAt = status.RetryAfterUntil
		}
		if err := profiles.SaveStore(s.profilePaths.StoreFile(), store); err != nil {
			if failure.Code == "rollback_unconfirmed" {
				return nil, failure
			}
			return nil, &refreshError{Code: "persist", Message: "失败信息无法保存；旧缓存保留"}
		}
		snapshot := usageSnapshot(airport, sub, "refresh_failed", "subscription_update", now)
		if err := s.saveUsageObservations(context.WithoutCancel(ctx), []subscriptionusage.Snapshot{snapshot}); err != nil {
			if failure.Code == "rollback_unconfirmed" {
				return nil, failure
			}
			return nil, &refreshError{Code: "persist", Message: "刷新失败记录无法保存；旧缓存保留"}
		}
		return nil, failure
	}

	if err := s.seedSubscriptionUsageLocked(ctx, store); err != nil {
		return fail(&refreshError{Code: "persist", Message: "用量快照初始化失败；旧缓存保留"})
	}
	if err := advance("fetching"); err != nil {
		return fail(err)
	}
	latestStore, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return fail(&refreshError{Code: "persist", Message: "读取订阅配置失败；旧缓存保留"})
	}
	airport = latestStore.Get(airportID)
	if airport == nil {
		return nil, &refreshError{Code: "configuration_changed", Message: "订阅配置已变化；请重新提交刷新"}
	}
	sub = airport.GetSubscription(subID)
	if sub == nil || strings.TrimSpace(sub.URL) == "" {
		return nil, &refreshError{Code: "configuration_changed", Message: "订阅配置已变化；请重新提交刷新"}
	}
	if expectedFingerprint != "" && refreshSourceFingerprint(sub.URL) != expectedFingerprint {
		return nil, &refreshError{Code: "configuration_changed", Message: "排队期间订阅配置已变化；请重新提交刷新"}
	}
	store = latestStore
	fetchFingerprint := refreshSourceFingerprint(sub.URL)
	var body []byte
	var usage *profiles.SubscriptionUsage
	if profiles.IsHTTPURL(sub.URL) {
		body, usage, err = profiles.FetchSubscriptionWithUsageContext(ctx, sub.URL, userAgent)
	} else {
		body, err = os.ReadFile(profiles.ExpandLocalPath(sub.URL))
		if err != nil {
			err = &refreshError{Code: "local_file", Message: "本地订阅文件读取失败；旧缓存保留"}
		}
	}
	if err != nil {
		return fail(err)
	}
	if err = advance("parsing"); err != nil {
		return fail(err)
	}
	if len(body) == 0 || len(body) > profiles.MaxSubscriptionBytes || speedtester.ValidateSubscriptionBody(body) != nil {
		return fail(&refreshError{Code: "parse", Message: "订阅内容无法解析为节点配置；旧缓存保留"})
	}
	latestStore, err = profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return fail(&refreshError{Code: "persist", Message: "读取订阅配置失败；旧缓存保留"})
	}
	airport = latestStore.Get(airportID)
	if airport == nil {
		return nil, &refreshError{Code: "configuration_changed", Message: "抓取期间订阅配置已变化；旧缓存保留，请重新提交刷新"}
	}
	sub = airport.GetSubscription(subID)
	if sub == nil || refreshSourceFingerprint(sub.URL) != fetchFingerprint {
		return nil, &refreshError{Code: "configuration_changed", Message: "抓取期间订阅配置已变化；旧缓存保留，请重新提交刷新"}
	}
	store = latestStore
	if err = advance("saving"); err != nil {
		return fail(err)
	}
	cachePath := s.profilePaths.CacheFile(sub.ID)
	cacheRecovery, err := createRefreshRecoveryCopy(cachePath)
	if err != nil {
		return fail(&refreshError{Code: "persist", Message: "旧节点缓存恢复副本无法创建；旧缓存保留"})
	}
	oldStoreBytes, err := os.ReadFile(s.profilePaths.StoreFile())
	if err != nil {
		cacheRecovery.discard()
		return fail(&refreshError{Code: "persist", Message: "原订阅状态无法读取；旧缓存保留"})
	}
	oldUsage, oldUpdatedAt, oldAirportUpdatedAt := sub.Usage, sub.UpdatedAt, airport.UpdatedAt
	oldFailureAt, oldRetryAt, oldFailureCode, oldFailureMessage := sub.LastFailureAt, sub.LastFailureRetryAt, sub.LastFailureCode, sub.LastFailureMessage
	if err = s.profilePaths.WriteCache(sub.ID, body); err != nil {
		cacheRecovery.discard()
		return fail(&refreshError{Code: "cache_write", Message: "节点缓存写入失败；旧缓存保留"})
	}
	now := time.Now()
	sub.Usage = usage
	sub.UpdatedAt = now
	airport.UpdatedAt = now
	sub.LastFailureRetryAt = time.Time{}
	sub.LastFailureCode = ""
	sub.LastFailureMessage = ""
	if err = s.saveSubscriptionRefreshStore(store); err != nil {
		cacheRollbackErr := s.restoreRefreshRecoveryCopy("cache", cacheRecovery)
		storeRollbackErr := s.restoreRefreshStoreBytes(oldStoreBytes)
		sub.Usage, sub.UpdatedAt, airport.UpdatedAt = oldUsage, oldUpdatedAt, oldAirportUpdatedAt
		sub.LastFailureAt, sub.LastFailureRetryAt, sub.LastFailureCode, sub.LastFailureMessage = oldFailureAt, oldRetryAt, oldFailureCode, oldFailureMessage
		return fail(refreshRollbackError("订阅更新保存失败；旧缓存和订阅状态已恢复", errors.Join(cacheRollbackErr, storeRollbackErr)))
	}
	status := "ok"
	if !profiles.IsHTTPURL(sub.URL) {
		status = "local_file"
	} else if usage == nil {
		status = "missing"
	}
	snapshot := usageSnapshot(airport, sub, status, "subscription_update", now)
	if err = s.saveUsageObservations(context.WithoutCancel(ctx), []subscriptionusage.Snapshot{snapshot}); err != nil {
		cacheRollbackErr := s.restoreRefreshRecoveryCopy("cache", cacheRecovery)
		storeRollbackErr := s.restoreRefreshStoreBytes(oldStoreBytes)
		sub.Usage, sub.UpdatedAt, airport.UpdatedAt = oldUsage, oldUpdatedAt, oldAirportUpdatedAt
		sub.LastFailureAt, sub.LastFailureRetryAt, sub.LastFailureCode, sub.LastFailureMessage = oldFailureAt, oldRetryAt, oldFailureCode, oldFailureMessage
		return fail(refreshRollbackError("用量历史保存失败；旧缓存和订阅状态已恢复", errors.Join(cacheRollbackErr, storeRollbackErr)))
	}
	cacheRecovery.discard()
	nodeCount := s.CountCachedNodes(sub.ID)
	dto := subscriptionDTO(airport.ID, sub, nodeCount, true)
	return &dto, nil
}

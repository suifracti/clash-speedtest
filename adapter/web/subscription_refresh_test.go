package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/subscriptionusage"
)

type refreshJobResponse struct {
	ID          string `json:"id"`
	State       string `json:"state"`
	Completed   int    `json:"completed"`
	Success     int    `json:"success"`
	Failed      int    `json:"failed"`
	Cancelled   int    `json:"cancelled"`
	NotExecuted int    `json:"not_executed"`
	Items       []struct {
		AirportID      string `json:"airport_id"`
		SubscriptionID string `json:"subscription_id"`
		State          string `json:"state"`
		ErrorCode      string `json:"error_code"`
		ErrorMessage   string `json:"error_message"`
		Fetch          *struct {
			RequestCount int `json:"request_count"`
		} `json:"fetch"`
	} `json:"items"`
}

const validRefreshBody = "proxies:\n - {name: fixture, type: ss, server: 192.0.2.1, port: 443, cipher: aes-128-gcm, password: fixture}"

func performRefreshRequest(handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Host = "127.0.0.1:8080"
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func startRefreshJob(t *testing.T, handler http.Handler, body string) refreshJobResponse {
	t.Helper()
	response := performRefreshRequest(handler, http.MethodPost, "/api/subscription-refresh-jobs", body)
	if response.Code != http.StatusAccepted {
		t.Fatalf("refresh start status = %d, body = %s", response.Code, response.Body.String())
	}
	var job refreshJobResponse
	if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	return job
}

func waitRefreshJob(t *testing.T, handler http.Handler, id string) refreshJobResponse {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var job refreshJobResponse
	for time.Now().Before(deadline) {
		response := performRefreshRequest(handler, http.MethodGet, "/api/subscription-refresh-jobs/"+id, "")
		if response.Code != http.StatusOK {
			t.Fatalf("refresh job read status = %d, body = %s", response.Code, response.Body.String())
		}
		if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
			t.Fatal(err)
		}
		if job.State != "running" {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("refresh job %q did not finish", id)
	return job
}

func TestRefreshJobRejectsQueuedSourceDriftWithoutLeakingURL(t *testing.T) {
	firstEntered := make(chan struct{}, 1)
	releaseFirst := make(chan struct{})
	var changedRequests atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/first":
			firstEntered <- struct{}{}
			<-releaseFirst
			w.Header().Set("Subscription-Userinfo", "upload=1;download=2;total=100")
			_, _ = w.Write([]byte("proxies:\n - {name: fixture, type: ss, server: 192.0.2.1, port: 443, cipher: aes-128-gcm, password: fixture}"))
		case "/changed":
			changedRequests.Add(1)
			w.Header().Set("Subscription-Userinfo", "upload=1;download=2;total=100")
			_, _ = w.Write([]byte(validRefreshBody))
		default:
			t.Errorf("unexpected fixture request path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fixture.Close()

	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	if err := profiles.SaveStore(paths.StoreFile(), &profiles.Store{Airports: []*profiles.Airport{{
		ID: "airport-a", Name: "Fixture Airport", Subscriptions: []*profiles.Subscription{
			{ID: "sub-one", Name: "First", URL: fixture.URL + "/first?token=FIRST_SECRET"},
			{ID: "sub-two", Name: "Queued", URL: fixture.URL + "/queued?token=QUEUED_SECRET"},
		},
	}}}); err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(ServerConfig{ProfilePaths: paths, HistoryDir: filepath.Join(root, "history")})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	handler := server.Handler()

	requestBody := `{"request_id":"drift-fixture","selections":[{"airport_id":"airport-a","subscription_id":"sub-one"},{"airport_id":"airport-a","subscription_id":"sub-two"}]}`
	started := performRefreshRequest(handler, http.MethodPost, "/api/subscription-refresh-jobs", requestBody)
	if started.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, body = %s", started.Code, started.Body.String())
	}
	var startedJob refreshJobResponse
	if err := json.Unmarshal(started.Body.Bytes(), &startedJob); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(started.Body.String(), "FIRST_SECRET") || strings.Contains(started.Body.String(), "QUEUED_SECRET") {
		t.Fatal("job response leaked a subscription token")
	}

	select {
	case <-firstEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("first local fixture request did not start")
	}
	store, err := profiles.LoadStore(paths.StoreFile())
	if err != nil {
		t.Fatal(err)
	}
	store.Get("airport-a").GetSubscription("sub-two").URL = fixture.URL + "/changed?token=CHANGED_SECRET"
	if err := profiles.SaveStore(paths.StoreFile(), store); err != nil {
		t.Fatal(err)
	}
	close(releaseFirst)

	var job refreshJobResponse
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response := performRefreshRequest(handler, http.MethodGet, "/api/subscription-refresh-jobs/"+startedJob.ID, "")
		if response.Code != http.StatusOK {
			t.Fatalf("job read status = %d, body = %s", response.Code, response.Body.String())
		}
		if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
			t.Fatal(err)
		}
		if job.State != "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if job.State != "finished" || len(job.Items) != 2 {
		t.Fatalf("refresh job did not finish with two selections: %+v", job)
	}
	if job.Items[1].SubscriptionID != "sub-two" || job.Items[1].State != "not_executed" || job.Items[1].ErrorCode != "configuration_changed" {
		t.Fatalf("queued source drift was not rejected: %+v", job.Items[1])
	}
	if !strings.Contains(job.Items[1].ErrorMessage, "重新提交") {
		t.Fatalf("drift message must ask for a new refresh: %q", job.Items[1].ErrorMessage)
	}
	if changedRequests.Load() != 0 {
		t.Fatalf("edited queued URL received %d requests", changedRequests.Load())
	}

	persisted, err := os.ReadFile(filepath.Join(paths.Dir, "subscription-refresh-jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"FIRST_SECRET", "QUEUED_SECRET", "CHANGED_SECRET", fixture.URL} {
		if strings.Contains(string(persisted), secret) {
			t.Fatalf("persisted refresh job contains source credential or URL: %q", secret)
		}
	}
	if strings.Contains(string(persisted), "drift-fixture") {
		t.Fatal("persisted refresh job contains raw request ID")
	}
}

func TestRefreshJobIdempotencyKeepsOneRequestPerRequestID(t *testing.T) {
	var requests atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Subscription-Userinfo", "upload=1;download=2;total=100")
		_, _ = fmt.Fprint(w, validRefreshBody)
	}))
	defer fixture.Close()
	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	if err := profiles.SaveStore(paths.StoreFile(), &profiles.Store{Airports: []*profiles.Airport{{ID: "a", Name: "A", Subscriptions: []*profiles.Subscription{{ID: "one", Name: "One", URL: fixture.URL}}}}}); err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(ServerConfig{ProfilePaths: paths, HistoryDir: filepath.Join(root, "history")})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	handler := server.Handler()
	body := `{"request_id":"same-request","all":true}`
	first := performRefreshRequest(handler, http.MethodPost, "/api/subscription-refresh-jobs", body)
	if first.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, body = %s", first.Code, first.Body.String())
	}
	second := performRefreshRequest(handler, http.MethodPost, "/api/subscription-refresh-jobs", body)
	if second.Code != http.StatusAccepted {
		t.Fatalf("duplicate start status = %d, body = %s", second.Code, second.Body.String())
	}
	var firstJob, secondJob refreshJobResponse
	if err := json.Unmarshal(first.Body.Bytes(), &firstJob); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondJob); err != nil {
		t.Fatal(err)
	}
	if firstJob.ID != secondJob.ID {
		t.Fatalf("idempotent start returned different jobs: %q and %q", firstJob.ID, secondJob.ID)
	}
	if job := waitRefreshJob(t, handler, firstJob.ID); job.State != "finished" {
		t.Fatalf("refresh job state = %q", job.State)
	}
	if requests.Load() != 1 {
		t.Fatalf("same request ID issued %d source requests, want one", requests.Load())
	}
}

func TestFailedRefreshPreservesCacheAndUsageAndAddsFailureHistory(t *testing.T) {
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer fixture.Close()
	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	oldAt := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	total := int64(100)
	store := &profiles.Store{Airports: []*profiles.Airport{{ID: "a", Name: "A", Subscriptions: []*profiles.Subscription{{
		ID: "one", Name: "One", URL: fixture.URL + "/sub?token=FAIL_SECRET", UpdatedAt: oldAt,
		Usage: &profiles.SubscriptionUsage{Upload: 2, Download: 9, Total: &total, UpdatedAt: oldAt},
	}}}}}
	if err := profiles.SaveStore(paths.StoreFile(), store); err != nil {
		t.Fatal(err)
	}
	if err := paths.WriteCache("one", []byte("old cache")); err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(ServerConfig{ProfilePaths: paths, HistoryDir: filepath.Join(root, "history")})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	job := startRefreshJob(t, server.Handler(), `{"request_id":"failure","all":true}`)
	job = waitRefreshJob(t, server.Handler(), job.ID)
	if len(job.Items) != 1 || job.Items[0].State != "failed" || job.Items[0].ErrorCode != "http_429" {
		t.Fatalf("unexpected failed refresh item: %+v", job.Items)
	}
	cached, err := os.ReadFile(paths.CacheFile("one"))
	if err != nil || string(cached) != "old cache" {
		t.Fatalf("failed refresh replaced old cache: %q, %v", cached, err)
	}
	saved, err := profiles.LoadStore(paths.StoreFile())
	if err != nil {
		t.Fatal(err)
	}
	sub := saved.Get("a").GetSubscription("one")
	if sub.Usage == nil || sub.Usage.Download != 9 || !sub.UpdatedAt.Equal(oldAt) {
		t.Fatalf("failed refresh changed the last successful usage: %+v", sub)
	}
	day := time.Now().In(subscriptionusage.Location).Format("2006-01-02")
	report, err := server.app.GetSubscriptionUsage(context.Background(), day, day, "day", "")
	if err != nil {
		t.Fatal(err)
	}
	foundFailure := false
	for _, interval := range report.Intervals {
		if interval.Status == "refresh_failed" {
			foundFailure = true
		}
	}
	if !foundFailure {
		t.Fatalf("failed refresh was not added to usage history: %+v", report.Intervals)
	}
}

func TestCancellingRefreshMarksQueuedSubscriptionsNotExecuted(t *testing.T) {
	entered := make(chan struct{}, 1)
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-r.Context().Done()
	}))
	defer fixture.Close()
	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	store := &profiles.Store{Airports: []*profiles.Airport{{ID: "a", Name: "A", Subscriptions: []*profiles.Subscription{
		{ID: "one", Name: "One", URL: fixture.URL + "/one"},
		{ID: "two", Name: "Two", URL: fixture.URL + "/two"},
	}}}}
	if err := profiles.SaveStore(paths.StoreFile(), store); err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(ServerConfig{ProfilePaths: paths, HistoryDir: filepath.Join(root, "history")})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	handler := server.Handler()
	job := startRefreshJob(t, handler, `{"request_id":"cancel","all":true}`)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first fixture request did not start")
	}
	cancelled := performRefreshRequest(handler, http.MethodPost, "/api/subscription-refresh-jobs/"+job.ID+"/cancel", "")
	if cancelled.Code != http.StatusOK {
		t.Fatalf("cancel status = %d, body = %s", cancelled.Code, cancelled.Body.String())
	}
	job = waitRefreshJob(t, handler, job.ID)
	if job.State != "cancelled" || job.Items[0].State != "cancelled" || job.Items[1].State != "not_executed" {
		t.Fatalf("cancelled job did not stop remaining work: %+v", job)
	}
}

func TestSameSource429CooldownSkipsRequestAndLeavesZeroCount(t *testing.T) {
	var requests atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Retry-After", "900")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer fixture.Close()
	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	store := &profiles.Store{Airports: []*profiles.Airport{{ID: "a", Name: "A", Subscriptions: []*profiles.Subscription{
		{ID: "one", Name: "One", URL: fixture.URL + "/one"},
		{ID: "two", Name: "Two", URL: fixture.URL + "/two"},
	}}}}
	if err := profiles.SaveStore(paths.StoreFile(), store); err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(ServerConfig{ProfilePaths: paths, HistoryDir: filepath.Join(root, "history")})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	job := startRefreshJob(t, server.Handler(), `{"request_id":"cooldown","all":true}`)
	job = waitRefreshJob(t, server.Handler(), job.ID)
	if requests.Load() != 1 || len(job.Items) != 2 || job.Items[1].State != "not_executed" || job.Items[1].ErrorCode != "source_cooldown" {
		t.Fatalf("cooldown was not skipped accurately: requests=%d job=%+v", requests.Load(), job)
	}
	if job.Items[1].Fetch == nil || job.Items[1].Fetch.RequestCount != 0 {
		t.Fatalf("unissued cooldown item counted as a request: %+v", job.Items[1].Fetch)
	}
}

func TestRetryRefreshJobIncludesOnlyPriorFailedSelections(t *testing.T) {
	var badFails atomic.Bool
	badFails.Store(true)
	var goodRequests, badRequests atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/good" {
			goodRequests.Add(1)
		} else {
			badRequests.Add(1)
			if badFails.Load() {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Subscription-Userinfo", "upload=1;download=2;total=100")
		_, _ = fmt.Fprint(w, validRefreshBody)
	}))
	defer fixture.Close()
	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	store := &profiles.Store{Airports: []*profiles.Airport{{ID: "a", Name: "A", Subscriptions: []*profiles.Subscription{
		{ID: "good", Name: "Good", URL: fixture.URL + "/good"},
		{ID: "bad", Name: "Bad", URL: fixture.URL + "/bad"},
	}}}}
	if err := profiles.SaveStore(paths.StoreFile(), store); err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(ServerConfig{ProfilePaths: paths, HistoryDir: filepath.Join(root, "history")})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	handler := server.Handler()
	first := startRefreshJob(t, handler, `{"request_id":"first","all":true}`)
	first = waitRefreshJob(t, handler, first.ID)
	if first.Success != 1 || first.Failed != 1 {
		t.Fatalf("unexpected first refresh result: %+v", first)
	}
	badFails.Store(false)
	retry := startRefreshJob(t, handler, fmt.Sprintf(`{"request_id":"retry","retry_of":%q}`, first.ID))
	retry = waitRefreshJob(t, handler, retry.ID)
	if len(retry.Items) != 1 || retry.Items[0].SubscriptionID != "bad" || retry.Success != 1 {
		t.Fatalf("retry expanded its prior failed scope: %+v", retry)
	}
	if goodRequests.Load() != 1 || badRequests.Load() != 2 {
		t.Fatalf("retry request counts = good:%d bad:%d", goodRequests.Load(), badRequests.Load())
	}
}

func TestLegacyRefreshSelectionUsesAirportIDAsSubscriptionID(t *testing.T) {
	var requests atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Subscription-Userinfo", "upload=1;download=2;total=100")
		_, _ = fmt.Fprint(w, validRefreshBody)
	}))
	defer fixture.Close()
	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	if err := profiles.SaveStore(paths.StoreFile(), &profiles.Store{Airports: []*profiles.Airport{{ID: "legacy", Name: "Legacy", URL: fixture.URL}}}); err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(ServerConfig{ProfilePaths: paths, HistoryDir: filepath.Join(root, "history")})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	job := startRefreshJob(t, server.Handler(), `{"request_id":"legacy","selections":[{"airport_id":"legacy","subscription_id":"legacy"}]}`)
	job = waitRefreshJob(t, server.Handler(), job.ID)
	if len(job.Items) != 1 || job.Items[0].SubscriptionID != "legacy" || job.Items[0].State != "succeeded" || requests.Load() != 1 {
		t.Fatalf("legacy fallback lost airport/subscription identity: requests=%d job=%+v", requests.Load(), job)
	}
}

func TestRefreshJobKeepsAirportAndSubscriptionIdentityAcrossAirports(t *testing.T) {
	var requests atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Subscription-Userinfo", "upload=1;download=2;total=100")
		_, _ = fmt.Fprint(w, validRefreshBody)
	}))
	defer fixture.Close()
	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	store := &profiles.Store{Airports: []*profiles.Airport{
		{ID: "airport-a", Name: "A", Subscriptions: []*profiles.Subscription{
			{ID: "account-a1", Name: "A1", URL: fixture.URL + "/a1"},
			{ID: "account-a2", Name: "A2", URL: fixture.URL + "/a2"},
		}},
		{ID: "airport-b", Name: "B", Subscriptions: []*profiles.Subscription{
			{ID: "account-b1", Name: "B1", URL: fixture.URL + "/b1"},
		}},
	}}
	if err := profiles.SaveStore(paths.StoreFile(), store); err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(ServerConfig{ProfilePaths: paths, HistoryDir: filepath.Join(root, "history")})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	job := startRefreshJob(t, server.Handler(), `{"request_id":"multiple-airports","all":true}`)
	job = waitRefreshJob(t, server.Handler(), job.ID)
	got := make(map[string]bool, len(job.Items))
	for _, item := range job.Items {
		got[item.AirportID+"\x00"+item.SubscriptionID] = item.State == "succeeded"
	}
	want := []string{"airport-a\x00account-a1", "airport-a\x00account-a2", "airport-b\x00account-b1"}
	if len(got) != len(want) || requests.Load() != int32(len(want)) {
		t.Fatalf("refresh selection count = %d, requests = %d; want %d distinct airport/account pairs", len(got), requests.Load(), len(want))
	}
	for _, pair := range want {
		if !got[pair] {
			t.Errorf("refresh did not succeed for airport/account pair %q", pair)
		}
	}
}

func TestInvalidSubscriptionBodyDoesNotReplaceExistingCache(t *testing.T) {
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Subscription-Userinfo", "upload=1;download=2;total=100")
		_, _ = fmt.Fprint(w, "<html>please sign in</html>")
	}))
	defer fixture.Close()
	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	if err := profiles.SaveStore(paths.StoreFile(), &profiles.Store{Airports: []*profiles.Airport{{ID: "a", Name: "A", Subscriptions: []*profiles.Subscription{{ID: "one", Name: "One", URL: fixture.URL}}}}}); err != nil {
		t.Fatal(err)
	}
	if err := paths.WriteCache("one", []byte("preserved cache")); err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(ServerConfig{ProfilePaths: paths, HistoryDir: filepath.Join(root, "history")})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	job := startRefreshJob(t, server.Handler(), `{"request_id":"invalid-body","all":true}`)
	job = waitRefreshJob(t, server.Handler(), job.ID)
	if len(job.Items) != 1 || job.Items[0].State != "failed" || job.Items[0].ErrorCode != "parse" {
		t.Fatalf("invalid content was not rejected: %+v", job.Items)
	}
	cached, err := os.ReadFile(paths.CacheFile("one"))
	if err != nil || string(cached) != "preserved cache" {
		t.Fatalf("invalid response replaced cache: %q, %v", cached, err)
	}
}

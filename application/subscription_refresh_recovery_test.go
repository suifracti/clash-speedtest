package application

import (
	"context"
	"errors"
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

const rollbackFixtureBody = "proxies:\n  - name: replacement-node\n    type: ss\n    server: 192.0.2.2\n    port: 8388\n    cipher: aes-128-gcm\n    password: replacement-password\n"

func TestRefreshWorkerProgressSaveFailureTerminatesQueuedItems(t *testing.T) {
	profileDir := filepath.Join(t.TempDir(), "profiles")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}
	job := &SubscriptionRefreshJob{ID: "persist-failure", State: "running", Items: []SubscriptionRefreshItem{
		{RefreshSelection: RefreshSelection{AirportID: "a", SubscriptionID: "one"}, State: "queued"},
		{RefreshSelection: RefreshSelection{AirportID: "a", SubscriptionID: "two"}, State: "queued"},
		{RefreshSelection: RefreshSelection{AirportID: "b", SubscriptionID: "three"}, State: "queued"},
	}}
	service := &AppService{
		profilePaths:  profiles.Paths{Dir: profileDir},
		refreshJobs:   []*SubscriptionRefreshJob{job},
		refreshLoaded: true,
	}
	if err := os.Chmod(profileDir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(profileDir, 0o700) }()

	service.refreshWG.Add(1)
	service.runSubscriptionRefresh(context.Background(), job, "fixture-agent")

	if job.State != "finished" {
		t.Fatalf("job state = %q, want finished", job.State)
	}
	if job.Completed != len(job.Items) || job.NotExecuted != len(job.Items) {
		t.Fatalf("finished job has incomplete summary: completed=%d not_executed=%d items=%d", job.Completed, job.NotExecuted, len(job.Items))
	}
	for _, item := range job.Items {
		if item.State != "not_executed" || item.ErrorCode != "persist" || item.FinishedAt.IsZero() {
			t.Errorf("item %s/%s was not terminal with persistence cause: %+v", item.AirportID, item.SubscriptionID, item)
		}
		if !strings.Contains(item.ErrorMessage, "没有发出请求") {
			t.Errorf("item %s/%s is missing the no-request reason: %q", item.AirportID, item.SubscriptionID, item.ErrorMessage)
		}
		if item.Fetch != nil {
			t.Errorf("item %s/%s unexpectedly issued a fetch: %+v", item.AirportID, item.SubscriptionID, item.Fetch)
		}
	}
	if job.FinishedAt.Before(job.CreatedAt) || job.FinishedAt.IsZero() {
		t.Fatalf("job finished timestamp = %v", job.FinishedAt.Format(time.RFC3339Nano))
	}
}

func TestRefreshRollbackFailuresKeepRecoveryCopiesAndReportUnconfirmedState(t *testing.T) {
	tests := []struct {
		name               string
		failStoreSave      bool
		failUsageSave      bool
		failedRestoreKinds []string
		expectedCode       string
		expectedCache      []byte
		cacheBackupRemains bool
		expectedStoreCode  string
	}{
		{name: "store save and cache restore fail", failStoreSave: true, failedRestoreKinds: []string{"cache"}, expectedCode: "rollback_unconfirmed", expectedCache: []byte(rollbackFixtureBody), cacheBackupRemains: true, expectedStoreCode: "rollback_unconfirmed"},
		{name: "history save and cache and store restore fail", failUsageSave: true, failedRestoreKinds: []string{"cache", "store"}, expectedCode: "rollback_unconfirmed", expectedCache: []byte(rollbackFixtureBody), cacheBackupRemains: true, expectedStoreCode: "rollback_unconfirmed"},
		{name: "store save failure restores original cache", failStoreSave: true, expectedCode: "persist", expectedCache: []byte("proxies:\n  - name: previous-node\n    type: ss\n    server: 192.0.2.1\n    port: 8388\n    cipher: aes-128-gcm\n    password: previous-password\n"), expectedStoreCode: "persist"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				_, _ = w.Write([]byte(rollbackFixtureBody))
			}))
			defer server.Close()

			service, paths := newAirportURLTestService(t)
			secretURL := server.URL + "/subscription?token=" + fakeAirportSecret
			store := &profiles.Store{Airports: []*profiles.Airport{{
				ID: "rollback-airport", Name: "Fixture airport", Subscriptions: []*profiles.Subscription{{
					ID: "rollback-sub", Name: "Fixture account", URL: secretURL,
				}},
			}}}
			if err := profiles.SaveStore(paths.StoreFile(), store); err != nil {
				t.Fatal(err)
			}
			oldCache := []byte("proxies:\n  - name: previous-node\n    type: ss\n    server: 192.0.2.1\n    port: 8388\n    cipher: aes-128-gcm\n    password: previous-password\n")
			if err := paths.WriteCache("rollback-sub", oldCache); err != nil {
				t.Fatal(err)
			}

			if tt.failStoreSave {
				service.subscriptionRefreshStoreSaveHook = func(string, *profiles.Store) error {
					return errors.New("injected store failure")
				}
			}
			if tt.failUsageSave {
				service.subscriptionRefreshUsageSaveHook = func(context.Context, []subscriptionusage.Snapshot) error {
					return errors.New("injected history failure")
				}
			}
			failedRestoreKinds := make(map[string]bool, len(tt.failedRestoreKinds))
			for _, kind := range tt.failedRestoreKinds {
				failedRestoreKinds[kind] = true
			}
			recoveryCopies := map[string]string{}
			storeRestoreCalled := false
			service.subscriptionRefreshRecoveryHook = func(kind, backupPath, _ string) error {
				recoveryCopies[kind] = backupPath
				if kind == "store" {
					storeRestoreCalled = true
					if backupPath != "" {
						return errors.New("store recovery must not persist a credential-bearing backup")
					}
				}
				if failedRestoreKinds[kind] {
					return errors.New("injected recovery failure")
				}
				return nil
			}

			_, err := service.refreshSubscriptionContext(context.Background(), "rollback-airport", "rollback-sub", "fixture-agent", refreshSourceFingerprint(secretURL), nil)
			if err == nil {
				t.Fatal("expected persistence and recovery failures")
			}
			var refreshErr *refreshError
			if !errors.As(err, &refreshErr) || refreshErr.Code != tt.expectedCode {
				t.Fatalf("expected %s error, got %#v (%v)", tt.expectedCode, refreshErr, err)
			}
			if tt.expectedCode == "rollback_unconfirmed" && (!strings.Contains(refreshErr.Message, "无法确认") || strings.Contains(refreshErr.Message, "已恢复")) {
				t.Fatalf("error does not clearly report the unconfirmed state: %q", refreshErr.Message)
			}
			if tt.expectedCode == "persist" && !strings.Contains(refreshErr.Message, "已恢复") {
				t.Fatalf("error should confirm successful restoration: %q", refreshErr.Message)
			}
			if strings.Contains(err.Error(), secretURL) || strings.Contains(err.Error(), fakeAirportSecret) || strings.Contains(err.Error(), paths.Dir) {
				t.Fatalf("recovery error leaked URL credentials or local paths: %v", err)
			}
			if requests.Load() != 1 {
				t.Fatalf("expected one fixture request, got %d", requests.Load())
			}

			actualCache, err := os.ReadFile(paths.CacheFile("rollback-sub"))
			if err != nil {
				t.Fatal(err)
			}
			if string(actualCache) != string(tt.expectedCache) {
				t.Fatalf("unexpected cache after rollback: %q", actualCache)
			}
			if got := recoveryCopies["cache"]; got == "" {
				t.Fatal("cache recovery copy was not offered to the restore operation")
			} else if tt.cacheBackupRemains {
				info, err := os.Stat(got)
				if err != nil || info.Mode().Perm() != 0o600 {
					t.Fatalf("old cache recovery copy missing or not private: stat error=%v info=%v", err, info)
				}
				data, err := os.ReadFile(got)
				if err != nil || string(data) != string(oldCache) {
					t.Fatalf("old cache recovery copy invalid: read error=%v data=%q", err, data)
				}
			} else if _, err := os.Stat(got); !os.IsNotExist(err) {
				t.Fatalf("successful cache restoration should consume its recovery copy: stat error=%v", err)
			}
			if tt.failUsageSave {
				if !storeRestoreCalled {
					t.Fatal("store restoration failure was not injected")
				}
			}
			persistedStore, err := profiles.LoadStore(paths.StoreFile())
			if err != nil {
				t.Fatal(err)
			}
			persistedSub := persistedStore.Get("rollback-airport").GetSubscription("rollback-sub")
			if persistedSub == nil || persistedSub.Usage != nil || persistedSub.LastFailureCode != tt.expectedStoreCode {
				t.Fatalf("stored state does not preserve old usage and report uncertain rollback: %+v", persistedSub)
			}
			if strings.Contains(fmt.Sprint(recoveryCopies), fakeAirportSecret) {
				t.Fatal("recovery copy names must not contain subscription credentials")
			}
		})
	}
}

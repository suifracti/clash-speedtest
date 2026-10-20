package profiles

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestUsageFallbackReadsHeadersAndKeepsPrimaryBody(t *testing.T) {
	for _, primaryUA := range []string{DefaultUserAgent(), "clash.meta"} {
		t.Run(primaryUA, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.UserAgent() == primaryUA {
					_, _ = w.Write([]byte("primary nodes"))
					return
				}
				expected := "clash.meta"
				if primaryUA == expected {
					expected = "mihomo"
				}
				if r.UserAgent() != expected {
					t.Errorf("fallback UA=%q want %q", r.UserAgent(), expected)
				}
				w.Header().Set("Subscription-Userinfo", "upload=1;download=2;total=10")
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				// No body arrives. Completion proves the fallback did not read it.
				<-r.Context().Done()
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			body, usage, evidence, err := FetchSubscriptionObserved(ctx, server.URL, primaryUA)
			if err != nil || string(body) != "primary nodes" || usage == nil || usage.Upload == nil || *usage.Upload != 1 || usage.Download == nil || *usage.Download != 2 {
				t.Fatalf("fallback body/counters: %q %+v %v", body, usage, err)
			}
			if calls.Load() != 2 || evidence.RequestCount != 2 || evidence.BytesRead != len(body) || evidence.BodySHA256 != fmt.Sprintf("%x", sha256.Sum256(body)) {
				t.Fatalf("fallback changed primary evidence: calls=%d %+v", calls.Load(), evidence)
			}
		})
	}
}

func TestUsageFallbackFailurePreservesPrimaryAndReportsCooldown(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte("primary nodes"))
			return
		}
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	body, usage, err := FetchSubscriptionWithUsage(server.URL, "")
	if err != nil || string(body) != "primary nodes" || calls.Load() != 2 {
		t.Fatalf("optional header failure lost primary content: %q %+v %v", body, usage, err)
	}
	raw, _ := json.Marshal(usage)
	var got struct {
		Fallback struct {
			HTTPStatus      int       `json:"http_status"`
			RetryAfterUntil time.Time `json:"retry_after_until"`
		} `json:"fallback"`
	}
	_ = json.Unmarshal(raw, &got)
	if got.Fallback.HTTPStatus != 429 || !got.Fallback.RetryAfterUntil.After(time.Now().Add(time.Minute)) {
		t.Fatalf("missing fallback cooldown: %s", raw)
	}
}

func TestUsageFallbackCancellationAndPrimaryFailures(t *testing.T) {
	t.Run("cancel header request", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				_, _ = w.Write([]byte("primary nodes"))
				return
			}
			cancel()
			<-r.Context().Done()
		}))
		defer server.Close()
		_, _, err := FetchSubscriptionWithUsageContext(ctx, server.URL, "")
		if !errors.Is(err, context.Canceled) || calls.Load() != 2 {
			t.Fatalf("fallback ignored cancellation: calls=%d err=%v", calls.Load(), err)
		}
	})
	t.Run("primary failure", func(t *testing.T) {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(http.StatusForbidden) }))
		defer server.Close()
		_, _, err := FetchSubscriptionWithUsage(server.URL, "")
		if err == nil || calls.Load() != 1 {
			t.Fatalf("failed primary triggered fallback: %d %v", calls.Load(), err)
		}
	})
}

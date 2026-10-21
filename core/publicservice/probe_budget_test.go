package publicservice

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
)

func TestServiceSocketBudgetFollowsExistingCompositeAndSamplePlan(t *testing.T) {
	cases := []struct {
		id            string
		samples, want int
	}{
		{"exit_ipv4", 1, 4}, {"exit_ipv6", 4, 5},
		{"netflix_unlock", 1, 13}, {"netflix_unlock", 4, 46}, {"netflix_unlock", 10, 112},
		{"ippure_ip_quality", 4, 89}, {"youtube_premium", 4, 21},
		{"paramount_web", 4, 17}, {"abema_unlock", 4, 6}, {"ipok_ipv4", 10, 4},
	}
	for _, tc := range cases {
		t.Run(tc.id+"/"+time.Duration(tc.samples).String(), func(t *testing.T) {
			rule, ok := RuleFor(tc.id)
			if !ok {
				t.Fatalf("missing rule %s", tc.id)
			}
			if got := serviceSocketBudget(rule, tc.samples); got != tc.want {
				t.Fatalf("socket budget=%d want=%d", got, tc.want)
			}
		})
	}
	for _, rule := range Catalog() {
		for _, samples := range []int{0, 1, 2, 4, 10} {
			if got := serviceSocketBudget(rule, samples); got < 1 || got > 221 {
				t.Fatalf("invalid physical budget %s/%d=%d", rule.ServiceID, samples, got)
			}
		}
	}
}

func TestCompositeFourSampleRedirectPlanIsNotCutOffAtFourConnections(t *testing.T) {
	rule, _ := RuleFor("paramount_web")
	limit := serviceSocketBudget(rule, 4)
	var calls atomic.Int32
	checker := Checker{DriftSamples: 4, ClientFactory: fixtureClient(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) > int32(limit) {
			return nil, errors.New("fixture physical socket budget exhausted")
		}
		if r.URL.Path == "/" {
			response := fixtureResponse(r, 302, "text/html", "")
			response.Header.Set("Location", "https://www.paramountplus.com/intl/")
			return response, nil
		}
		return fixtureResponse(r, 200, "text/html", "<html><body>Public entry</body></html>"), nil
	})}
	result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, time.Second)
	if result.Outcome != "reachable" || calls.Load() != 8 || result.Details["sample_success"] != "4" {
		t.Fatalf("existing 4-observation plan was truncated: calls=%d result=%+v", calls.Load(), result)
	}
}

func TestCheckerInvalidObservationPlanDoesNotCallFactory(t *testing.T) {
	rule, _ := RuleFor("cloudflare_204")
	for _, count := range []int{-1, 11, 100000} {
		var called bool
		checker := Checker{DriftSamples: count, ClientFactory: func(monitor.MonitoredNode, time.Duration) (*http.Client, error) { called = true; return nil, nil }}
		result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, time.Second)
		if called || result.Outcome != "not_executed" || result.RequestCount != 0 {
			t.Fatalf("invalid plan started: %+v", result)
		}
	}
}

func TestCheckerDoesNotMutateOrCrossCountSharedClient(t *testing.T) {
	var calls atomic.Int32
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return fixtureResponse(r, 204, "", ""), nil
	})
	originalRedirect := func(*http.Request, []*http.Request) error { return errors.New("original redirect policy") }
	client := &http.Client{Transport: transport, CheckRedirect: originalRedirect}
	checker := Checker{DriftSamples: 1, ClientFactory: func(monitor.MonitoredNode, time.Duration) (*http.Client, error) { return client, nil }}
	rule, _ := RuleFor("cloudflare_204")
	results := make(chan Result, 4)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- checker.Check(context.Background(), monitor.MonitoredNode{}, rule, time.Second)
		}()
	}
	wg.Wait()
	close(results)
	for result := range results {
		if result.RequestCount != 1 || result.Outcome != "matched" {
			t.Fatalf("attempt trace leaked: %+v", result)
		}
	}
	if calls.Load() != 4 || reflect.ValueOf(client.Transport).Pointer() != reflect.ValueOf(transport).Pointer() || reflect.ValueOf(client.CheckRedirect).Pointer() != reflect.ValueOf(originalRedirect).Pointer() {
		t.Fatal("caller-owned client mutated")
	}
}

func TestExitAddressHasExplicitAddressOnlySourceWithoutBorrowedGeography(t *testing.T) {
	for _, tc := range []struct {
		id, ip, family string
		ok             bool
	}{
		{"exit_ipv4", "198.51.100.7", "IPv4", true},
		{"exit_ipv6", "2001:db8::7", "IPv6", true},
		{"exit_ipv4", "2001:db8::7", "IPv4", false},
		{"exit_ipv6", "198.51.100.7", "IPv6", false},
	} {
		t.Run(tc.id+tc.ip, func(t *testing.T) {
			rule, _ := RuleFor(tc.id)
			checker := Checker{DriftSamples: 1, ClientFactory: fixtureClient(func(r *http.Request) (*http.Response, error) {
				return fixtureResponse(r, 200, "application/json", `{"ip":"`+tc.ip+`"}`), nil
			})}
			result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, time.Second)
			if (result.Outcome == "profiled") != tc.ok || result.Details["ip_family"] != tc.family || result.Details["profile_fields"] != "ip_only" || result.Details["provider"] != "ipify" {
				t.Fatalf("bad exit scope: %+v", result)
			}
			if result.Details["country"] != "" || result.Details["country_code"] != "" || result.Details["risk_score"] != "" {
				t.Fatal("address query fabricated geography/risk")
			}
			if _, err := time.Parse(time.RFC3339Nano, result.Details["query_at"]); err != nil {
				t.Fatal(err)
			}
			if !tc.ok && result.Details["exit_observed_at"] != "" {
				t.Fatal("failed lookup has confirmed exit time")
			}
		})
	}
}

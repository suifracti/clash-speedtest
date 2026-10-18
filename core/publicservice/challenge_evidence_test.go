package publicservice

import (
	"context"
	"net/http"
	"testing"

	"github.com/faceair/clash-speedtest/core/monitor"
)

func TestWeakChallengeTextDoesNotBecomeConfirmedServiceFailure(t *testing.T) {
	for _, service := range []string{"viu_web", "youtube_premium", "netflix_unlock"} {
		for _, body := range []string{
			`<title>Just a moment...</title>`,
			`<html><body>Unusual traffic from your computer network</body></html>`,
		} {
			t.Run(service+"/"+body, func(t *testing.T) {
				rule, _ := RuleFor(service)
				// Exercise the page classifier without the separate selected-node DNS fixture.
				if service == "netflix_unlock" {
					rule.RuleVersion = 1
				}
				calls := 0
				checker := Checker{DriftSamples: 1, ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
					calls++
					return fixtureResponse(req, http.StatusOK, "text/html", body), nil
				})}
				result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
				if result.Outcome != "suspected_challenge" || isFailureOutcome(result) || result.Details["sample_failure"] != "0" || result.Details["sample_unknown"] != "1" {
					t.Fatalf("weak text became confirmed rejection: outcome=%s failure=%s unknown=%s", result.Outcome, result.Details["sample_failure"], result.Details["sample_unknown"])
				}
				maximum := 1
				if service == "netflix_unlock" {
					maximum = 2
				}
				if calls < 1 || calls > maximum || result.HTTPStatus == nil || *result.HTTPStatus != http.StatusOK {
					t.Fatalf("request/status evidence changed: calls=%d status=%v", calls, result.HTTPStatus)
				}
			})
		}
	}
}

func TestExplicitChallengeEvidenceStillCountsAsObservedBarrier(t *testing.T) {
	for _, service := range []string{"viu_web", "youtube_premium", "netflix_unlock"} {
		for _, evidence := range []string{"header", "platform"} {
			t.Run(service+"/"+evidence, func(t *testing.T) {
				rule, _ := RuleFor(service)
				if service == "netflix_unlock" {
					rule.RuleVersion = 1
				}
				checker := Checker{DriftSamples: 1, ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
					body := `<title>Just a moment...</title>`
					if evidence == "platform" {
						body += `<script src="/cdn-cgi/challenge-platform/test"></script>`
					}
					r := fixtureResponse(req, http.StatusOK, "text/html", body)
					if evidence == "header" {
						r.Header.Set("cf-mitigated", "challenge")
					}
					return r, nil
				})}
				result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
				if result.Outcome != "challenge" || !isFailureOutcome(result) || result.Details["sample_failure"] != "1" {
					t.Fatalf("lost explicit barrier evidence: outcome=%s failure=%s", result.Outcome, result.Details["sample_failure"])
				}
			})
		}
	}
}

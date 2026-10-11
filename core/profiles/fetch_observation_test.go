package profiles

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchSubscriptionObservedRecordsOneGETAndBodyDigest(t *testing.T) {
	const body = "proxies:\n - {name: fixture, type: ss, server: 192.0.2.1, port: 443, cipher: aes-128-gcm, password: fixture}"
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.UserAgent() != "cst-fixture" {
			t.Errorf("request = %s %q", r.Method, r.UserAgent())
		}
		w.Header().Set("Subscription-Userinfo", "upload=2;download=3;total=10")
		_, _ = fmt.Fprint(w, body)
	}))
	defer server.Close()

	data, usage, evidence, err := FetchSubscriptionObserved(context.Background(), server.URL+"/sub?token=fixture-secret", "cst-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != body || usage == nil || usage.Upload != 2 || usage.Download != 3 {
		t.Fatalf("response body or usage mismatch: body=%q usage=%+v", data, usage)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
	if requests != 1 || evidence.RequestCount != 1 || evidence.Method != http.MethodGet || evidence.UserAgent != "cst-fixture" || evidence.HTTPStatus != http.StatusOK || evidence.BytesRead != len(body) || evidence.BodySHA256 != digest {
		t.Fatalf("unexpected fetch evidence: requests=%d evidence=%+v", requests, evidence)
	}
}

func TestFetchSubscriptionObservedCapsResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, MaxSubscriptionBytes+1))
	}))
	defer server.Close()

	_, _, evidence, err := FetchSubscriptionObserved(context.Background(), server.URL, "cst-fixture")
	var bodyError *FetchBodyError
	if err == nil || !errors.As(err, &bodyError) {
		t.Fatalf("oversized body error = %v", err)
	}
	if evidence.BytesRead != MaxSubscriptionBytes+1 || evidence.RequestCount != 1 {
		t.Fatalf("oversized response evidence = %+v", evidence)
	}
}

func TestFetchSubscriptionObservedRecords429RetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	before := time.Now()
	_, _, evidence, err := FetchSubscriptionObserved(context.Background(), server.URL, "cst-fixture")
	var httpError *FetchHTTPError
	if !errors.As(err, &httpError) || httpError.Status != http.StatusTooManyRequests {
		t.Fatalf("429 response error = %v", err)
	}
	if evidence.RequestCount != 1 || evidence.HTTPStatus != http.StatusTooManyRequests || evidence.RetryAfterUntil.Before(before.Add(119*time.Second)) || evidence.RetryAfterUntil.After(before.Add(121*time.Second)) {
		t.Fatalf("429 retry-after evidence = %+v", evidence)
	}
}

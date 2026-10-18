package speedtester

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptrace"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestDownloadFailedTLSCompletionRetainsTLSPhaseAndZeroBody(t *testing.T) {
	st := newDownloadStreamTester(t)
	client := &http.Client{Transport: downloadRoundTripper(func(r *http.Request) (*http.Response, error) {
		trace := httptrace.ContextClientTrace(r.Context())
		trace.TLSHandshakeStart()
		trace.TLSHandshakeDone(tls.ConnectionState{}, context.DeadlineExceeded)
		return nil, context.DeadlineExceeded
	})}
	result := st.MeasureDownloadStream(context.Background(), client, DownloadStreamOptions{MaximumBytes: 1024, MaximumDuration: time.Second}, nil)
	if result.Outcome != "time_limit" || result.FailurePhase != "tls" || result.BytesRead != 0 || result.HTTPStatus != nil || len(result.Samples) != 0 || result.PartialMeasurement {
		t.Fatalf("TLS failed before HTTP/body, not a zero-speed measurement: %+v", result)
	}
}

func TestDownloadDoesNotMutateSharedClientOrRedirectPolicy(t *testing.T) {
	st := newDownloadStreamTester(t)
	originalRedirect := func(*http.Request, []*http.Request) error { return errors.New("original redirect policy") }
	transport := downloadRoundTripper(func(r *http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })
	client := &http.Client{Transport: transport, CheckRedirect: originalRedirect}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			st.MeasureDownloadStream(context.Background(), client, DownloadStreamOptions{MaximumBytes: 1024, MaximumDuration: time.Second}, nil)
		}()
	}
	wg.Wait()
	if reflect.ValueOf(client.Transport).Pointer() != reflect.ValueOf(transport).Pointer() || reflect.ValueOf(client.CheckRedirect).Pointer() != reflect.ValueOf(originalRedirect).Pointer() {
		t.Fatal("download overwrote caller-owned client")
	}
}

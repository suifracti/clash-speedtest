package application

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/core/autopick"
)

func TestAutoPickSubscriptionServesOnlyTheTokenPath(t *testing.T) {
	svc := autoPickFixture(t)
	content := []byte("proxies:\n  - name: n1\n    type: ss\n    server: 1.1.1.1\n    port: 8388\n    cipher: aes-128-gcm\n    password: pw\n")
	if err := writeAutoPickFileAtomic(svc.autoPickSubscriptionPath(), content); err != nil {
		t.Fatalf("write subscription: %v", err)
	}
	svc.startAutoPickSubscriptionServer()
	defer svc.stopAutoPick()

	state := svc.autoPick()
	state.mu.Lock()
	port := state.port
	state.mu.Unlock()
	if port == 0 {
		t.Fatal("subscription server did not start")
	}
	token := svc.autoPickToken()
	base := fmt.Sprintf("http://127.0.0.1:%d/sub/", port)

	res, err := http.Get(base + token + ".yaml")
	if err != nil {
		t.Fatalf("get subscription: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || string(body) != string(content) {
		t.Fatalf("status=%d body=%q", res.StatusCode, body)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "yaml") {
		t.Fatalf("content type: %q", ct)
	}

	for _, path := range []string{"wrong-token.yaml", "../config.json", token + ".txt"} {
		res, err := http.Get(base + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("%s: status=%d, want 404", path, res.StatusCode)
		}
	}

	status := svc.autoPickSubscriptionStatus(autopick.Config{})
	if !status.Exists || !strings.HasSuffix(status.URL, "/sub/"+token+".yaml") || status.Path != svc.autoPickSubscriptionPath() {
		t.Fatalf("subscription status: %+v", status)
	}
	if _, err := os.Stat(status.Path); err != nil {
		t.Fatalf("fixed subscription path missing: %v", err)
	}
}

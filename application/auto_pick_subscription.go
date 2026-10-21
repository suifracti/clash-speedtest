package application

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/faceair/clash-speedtest/core/autopick"
)

func (s *AppService) autoPickToken() string {
	path := filepath.Join(s.autoPickRoot(), autoPickTokenFile)
	if raw, err := os.ReadFile(path); err == nil {
		if token := strings.TrimSpace(string(raw)); len(token) >= 16 {
			return token
		}
	}
	token := randomToken(16)
	_ = writeAutoPickFileAtomic(path, []byte(token))
	return token
}

func randomToken(bytesCount int) string {
	buf := make([]byte, bytesCount)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(buf)
}

func (s *AppService) autoPickSubscriptionStatus(cfg autopick.Config) AutoPickSubscription {
	path := s.autoPickSubscriptionPath()
	status := AutoPickSubscription{Path: path}
	token := s.autoPickToken()
	state := s.autoPick()
	state.mu.Lock()
	port := state.port
	state.mu.Unlock()
	if port == 0 {
		port = autoPickReadPort(path, autoPickDefaultPort)
	}
	status.URL = fmt.Sprintf("http://127.0.0.1:%d/sub/%s.yaml", port, token)
	if info, err := os.Stat(path); err == nil {
		status.Exists = true
		updated := info.ModTime().UTC()
		status.UpdatedAt = &updated
		if count, ok := s.autoPickSubscriptionNodeCount(); ok {
			status.NodeCount = count
		}
	}
	return status
}

func autoPickReadPort(path string, fallback int) int {
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(path), autoPickPortFile))
	if err != nil {
		return fallback
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || port <= 0 || port > 65535 {
		return fallback
	}
	return port
}

func (s *AppService) autoPickSubscriptionNodeCount() (int, bool) {
	report := s.latestSuccessfulAutoPickReport("")
	if report == nil || report.Subscription == "" {
		return 0, false
	}
	return report.SelectedCount, true
}

func (s *AppService) startAutoPickSubscriptionServer() {
	state := s.autoPick()
	state.mu.Lock()
	if state.server != nil {
		state.mu.Unlock()
		return
	}
	token := s.autoPickToken()
	preferred := autoPickReadPort(s.autoPickSubscriptionPath(), autoPickDefaultPort)
	var listener net.Listener
	var err error
	for _, port := range []int{preferred, 0} {
		listener, err = net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			break
		}
	}
	if err != nil {
		state.mu.Unlock()
		return
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = writeAutoPickFileAtomic(filepath.Join(s.autoPickRoot(), autoPickPortFile), []byte(strconv.Itoa(port)))
	subscriptionPath := s.autoPickSubscriptionPath()
	mux := http.NewServeMux()
	mux.HandleFunc("/sub/{name}", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		if r.PathValue("name") != token+".yaml" {
			http.NotFound(w, r)
			return
		}
		if _, err := os.Stat(subscriptionPath); err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, subscriptionPath)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	state.server = server
	state.port = port
	state.mu.Unlock()
	state.wg.Add(1)
	go func() {
		defer state.wg.Done()
		_ = server.Serve(listener)
	}()
}

//go:build windows

package auth

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"
)

func newOAuthHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			Proxy: oauthProxyURL,
		},
	}
}

func oauthProxyURL(req *http.Request) (*url.URL, error) {
	if proxyURL, err := http.ProxyFromEnvironment(req); proxyURL != nil || err != nil {
		return proxyURL, err
	}
	if req == nil || req.URL == nil {
		return nil, nil
	}
	return readWindowsSystemProxy(req.URL.Scheme), nil
}

func readWindowsSystemProxy(scheme string) *url.URL {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer key.Close()

	enabled, _, err := key.GetIntegerValue("ProxyEnable")
	if err != nil || enabled != 1 {
		return nil
	}
	server, _, err := key.GetStringValue("ProxyServer")
	if err != nil {
		return nil
	}
	return parseWindowsProxyURL(enabled, server, scheme)
}

func parseWindowsProxyURL(enabled uint64, proxyServer, scheme string) *url.URL {
	if enabled != 1 || (scheme != "http" && scheme != "https") {
		return nil
	}
	proxyServer = strings.TrimSpace(proxyServer)
	if proxyServer == "" {
		return nil
	}
	rawTarget := ""
	if strings.Contains(proxyServer, "=") {
		for _, entry := range strings.FieldsFunc(proxyServer, func(r rune) bool { return r == ';' || r == '\n' || r == '\r' }) {
			kv := strings.SplitN(strings.TrimSpace(entry), "=", 2)
			if len(kv) == 2 && strings.EqualFold(strings.TrimSpace(kv[0]), scheme) {
				rawTarget = strings.TrimSpace(kv[1])
				break
			}
		}
		if rawTarget == "" {
			return nil
		}
	} else {
		rawTarget = proxyServer
	}
	lower := strings.ToLower(rawTarget)
	switch {
	case strings.HasPrefix(lower, "http://"):
		rawTarget = rawTarget[len("http://"):]
	case strings.HasPrefix(lower, "https://"):
		rawTarget = rawTarget[len("https://"):]
	case strings.Contains(rawTarget, "://"):
		return nil
	}
	if strings.ContainsAny(rawTarget, "/@?#\\ \t\r\n") {
		return nil
	}
	host, portStr, err := net.SplitHostPort(rawTarget)
	if err != nil || host == "" {
		return nil
	}
	if port, err := strconv.Atoi(portStr); err != nil || port < 1 || port > 65535 {
		return nil
	}
	return &url.URL{Scheme: "http", Host: net.JoinHostPort(host, portStr)}
}

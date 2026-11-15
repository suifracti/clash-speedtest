//go:build windows

package profiles

import (
	"context"
	"net"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func normalSystemProxy(ctx context.Context, scheme string) (*url.URL, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return nil, nil
	}
	defer key.Close()

	enabled, _, err := key.GetIntegerValue("ProxyEnable")
	if err != nil || enabled != 1 {
		return nil, nil
	}
	server, _, err := key.GetStringValue("ProxyServer")
	if err != nil {
		return nil, nil
	}
	return parseWindowsProxyServer(enabled, server, scheme), nil
}

func parseWindowsProxyServer(enabled uint64, proxyServer, scheme string) *url.URL {
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

	lowerTarget := strings.ToLower(rawTarget)
	if strings.HasPrefix(lowerTarget, "http://") {
		rawTarget = rawTarget[len("http://"):]
	} else if strings.HasPrefix(lowerTarget, "https://") {
		rawTarget = rawTarget[len("https://"):]
	} else if strings.Contains(rawTarget, "://") {
		return nil
	}

	if strings.ContainsAny(rawTarget, "/@?#\\ \t\r\n") {
		return nil
	}
	host, portStr, err := net.SplitHostPort(rawTarget)
	if err != nil || host == "" || strings.ContainsAny(host, "/@?#\\ \t\r\n") {
		return nil
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return nil
	}
	return &url.URL{Scheme: "http", Host: net.JoinHostPort(host, strconv.Itoa(port))}
}

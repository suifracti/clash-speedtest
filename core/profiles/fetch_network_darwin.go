//go:build darwin

package profiles

import (
	"context"
	"net"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func normalSystemProxy(ctx context.Context, scheme string) (*url.URL, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "/usr/sbin/scutil", "--proxy").Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, nil
	}
	return parseSystemProxySnapshot(b, scheme), nil
}

func parseSystemProxySnapshot(b []byte, scheme string) *url.URL {
	if scheme != "http" && scheme != "https" {
		return nil
	}
	fields := map[string]string{}
	depth := 0
	structured := strings.Contains(string(b), "<dictionary>")
	for _, l := range strings.Split(string(b), "\n") {
		if strings.Contains(l, "{") {
			depth += strings.Count(l, "{")
			continue
		}
		if strings.Contains(l, "}") {
			depth -= strings.Count(l, "}")
			continue
		}
		if structured && depth != 1 {
			continue
		}
		p := strings.SplitN(l, ":", 2)
		if len(p) == 2 {
			fields[strings.TrimSpace(p[0])] = strings.TrimSpace(p[1])
		}
	}
	prefix := "HTTP"
	if scheme == "https" {
		prefix = "HTTPS"
	}
	if fields[prefix+"Enable"] != "1" {
		return nil
	}
	port, err := strconv.Atoi(fields[prefix+"Port"])
	if err != nil || port < 1 || port > 65535 || fields[prefix+"Proxy"] == "" || strings.ContainsAny(fields[prefix+"Proxy"], "/@?#\\ \t\n") {
		return nil
	}
	return &url.URL{Scheme: "http", Host: net.JoinHostPort(fields[prefix+"Proxy"], strconv.Itoa(port))}
}

package speedtester

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/metacubex/mihomo/common/convert"
	"gopkg.in/yaml.v2"
)

var shareSchemeRe = regexp.MustCompile(`(?i)(https?|socks5h?|socks|ssr|ss|vmess|vless|trojan|hysteria2|hysteria|hy2|tuic|anytls|snell|wireguard|wg)://`)

func parseConfigBody(body []byte) (*RawConfig, error) {
	original := normalizeSubscription(body)
	candidates := [][]byte{original}
	if decoded := tryBase64Payload(original); len(decoded) > 0 && !bytes.Equal(decoded, original) {
		candidates = append(candidates, normalizeSubscription(decoded))
	}

	var shareErr error
	for _, candidate := range candidates {
		if cfg, ok := tryClashLike(candidate); ok {
			return cfg, nil
		}
		if cfg, ok := tryProxyList(candidate); ok {
			return cfg, nil
		}
		if cfg, ok := tryJSONFormats(candidate); ok {
			return cfg, nil
		}
		if cfg, err := tryShareLinks(candidate); err == nil {
			return cfg, nil
		} else {
			shareErr = err
		}
		if cfg, ok := tryQuantumult(candidate); ok {
			return cfg, nil
		}
		if cfg, ok := trySurge(candidate); ok {
			return cfg, nil
		}
	}

	if shareErr != nil {
		return nil, fmt.Errorf("unsupported subscription format (Clash YAML/JSON, SIP008, sing-box, Quantumult X, Surge, or vless/vmess/ss/hysteria2/snell share links)")
	}
	return nil, fmt.Errorf("subscription contained no proxies")
}

func normalizeSubscription(body []byte) []byte {
	body = bytes.TrimSpace(body)
	if len(body) >= 3 && body[0] == 0xEF && body[1] == 0xBB && body[2] == 0xBF {
		body = bytes.TrimSpace(body[3:])
	}
	body = bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n"))
	body = bytes.ReplaceAll(body, []byte{'\r'}, []byte{'\n'})
	return body
}

func tryBase64Payload(body []byte) []byte {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || looksStructured(trimmed) {
		return nil
	}
	decoded, err := convert.TryDecodeBase64(string(trimmed))
	if err != nil {
		decoded = convert.DecodeBase64(trimmed)
		if bytes.Equal(decoded, trimmed) {
			return nil
		}
	}
	decoded = bytes.TrimSpace(decoded)
	if len(decoded) == 0 {
		return nil
	}
	return decoded
}

func looksStructured(body []byte) bool {
	t := bytes.TrimSpace(body)
	if len(t) == 0 {
		return false
	}
	switch t[0] {
	case '{', '[':
		return true
	}
	if bytes.Contains(t, []byte("://")) {
		return true
	}
	lower := bytes.ToLower(t)
	return bytes.Contains(lower, []byte("proxies:")) ||
		bytes.Contains(lower, []byte("outbounds")) ||
		bytes.Contains(lower, []byte("proxy-providers")) ||
		bytes.Contains(lower, []byte("[proxy]"))
}

func tryClashLike(body []byte) (*RawConfig, bool) {
	var generic map[string]any
	if err := yaml.Unmarshal(body, &generic); err != nil || generic == nil {
		return nil, false
	}
	proxies := firstSlice(generic, "proxies", "Proxy", "proxy")
	providers := firstMapMap(generic, "proxy-providers", "proxy-provider", "Proxy Provider")
	parsed := asProxyMaps(proxies)
	if len(parsed) == 0 && len(providers) == 0 {
		return nil, false
	}
	return &RawConfig{Proxies: parsed, Providers: providers}, true
}

func tryProxyList(body []byte) (*RawConfig, bool) {
	var listed []any
	if err := yaml.Unmarshal(body, &listed); err != nil || len(listed) == 0 {
		return nil, false
	}
	proxies := asProxyMaps(listed)
	if len(proxies) == 0 {
		return nil, false
	}
	return &RawConfig{Proxies: proxies}, true
}

func tryShareLinks(body []byte) (*RawConfig, error) {
	prepared := []byte(splitConcatenatedShareLinks(string(body)))
	proxies, err := convert.ConvertsV2Ray(prepared)
	if err != nil {
		proxies = nil
	}
	enrichHysteria2Ports(proxies, prepared)
	extra := parseExtraShareLines(prepared)
	merged := append(proxies, extra...)
	if len(merged) == 0 {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("no share links")
	}
	return &RawConfig{Proxies: merged}, nil
}

func splitConcatenatedShareLinks(s string) string {
	s = strings.ReplaceAll(s, "|", " ")
	matches := shareSchemeRe.FindAllStringIndex(s, -1)
	if len(matches) < 2 {
		return s
	}
	var b strings.Builder
	prev := 0
	for i, match := range matches {
		if i == 0 {
			prev = match[0]
			if match[0] > 0 {
				b.WriteString(s[:match[0]])
			}
			continue
		}
		chunk := strings.TrimSpace(s[prev:match[0]])
		b.WriteString(chunk)
		if !strings.HasSuffix(chunk, "\n") {
			b.WriteByte('\n')
		}
		prev = match[0]
	}
	b.WriteString(s[prev:])
	return b.String()
}

func enrichHysteria2Ports(proxies []map[string]any, body []byte) {
	if len(proxies) == 0 {
		return
	}
	decoded := string(convert.DecodeBase64(bytes.TrimSpace(body)))
	decoded = strings.ReplaceAll(decoded, "\r\n", "\n")
	for _, line := range strings.Split(decoded, "\n") {
		line = strings.TrimSpace(line)
		u, err := url.Parse(line)
		if err != nil {
			continue
		}
		scheme := strings.ToLower(u.Scheme)
		if scheme != "hysteria2" && scheme != "hy2" {
			continue
		}
		mport := u.Query().Get("mport")
		if mport == "" {
			mport = u.Query().Get("ports")
		}
		if mport == "" {
			continue
		}
		host, port := u.Hostname(), u.Port()
		for _, proxy := range proxies {
			if fmt.Sprint(proxy["type"]) != "hysteria2" {
				continue
			}
			if fmt.Sprint(proxy["server"]) == host && fmt.Sprint(proxy["port"]) == port {
				proxy["ports"] = mport
			}
		}
	}
}

func parseExtraShareLines(body []byte) []map[string]any {
	decoded := string(convert.DecodeBase64(bytes.TrimSpace(body)))
	decoded = splitConcatenatedShareLinks(strings.ReplaceAll(decoded, "\r\n", "\n"))
	names := make(map[string]int)
	var proxies []map[string]any
	for _, line := range strings.Split(decoded, "\n") {
		line = strings.TrimSpace(line)
		scheme, _, ok := strings.Cut(line, "://")
		if !ok {
			continue
		}
		switch strings.ToLower(scheme) {
		case "snell":
			if proxy, ok := parseSnellURI(line, names); ok {
				proxies = append(proxies, proxy)
			}
		case "wg", "wireguard":
			if proxy, ok := parseWireGuardURI(line, names); ok {
				proxies = append(proxies, proxy)
			}
		}
	}
	return proxies
}

func parseSnellURI(line string, names map[string]int) (map[string]any, bool) {
	u, err := url.Parse(line)
	if err != nil || u.Hostname() == "" || u.Port() == "" {
		return nil, false
	}
	psk := u.User.Username()
	if psk == "" {
		return nil, false
	}
	query := u.Query()
	name := uniqueProxyName(names, u.Fragment)
	if name == "" {
		name = uniqueProxyName(names, u.Hostname())
	}
	proxy := map[string]any{
		"name":   name,
		"type":   "snell",
		"server": u.Hostname(),
		"port":   u.Port(),
		"psk":    psk,
		"udp":    true,
	}
	if version := query.Get("version"); version != "" {
		if n, err := strconv.Atoi(version); err == nil {
			proxy["version"] = n
		}
	}
	obfs := query.Get("obfs")
	if obfs == "" {
		obfs = query.Get("obfs-mode")
	}
	if obfs != "" {
		opts := map[string]any{"mode": obfs}
		if host := query.Get("obfs-host"); host != "" {
			opts["host"] = host
		}
		proxy["obfs-opts"] = opts
	}
	return proxy, true
}

func parseWireGuardURI(line string, names map[string]int) (map[string]any, bool) {
	u, err := url.Parse(line)
	if err != nil || u.Hostname() == "" {
		return nil, false
	}
	query := u.Query()
	privateKey := u.User.Username()
	if privateKey == "" {
		privateKey = firstQuery(query, "privatekey", "private-key", "private_key")
	}
	publicKey := firstQuery(query, "publickey", "public-key", "public_key")
	if privateKey == "" || publicKey == "" {
		return nil, false
	}
	port := u.Port()
	if port == "" {
		port = "51820"
	}
	name := uniqueProxyName(names, u.Fragment)
	if name == "" {
		name = uniqueProxyName(names, u.Hostname())
	}
	proxy := map[string]any{
		"name":        name,
		"type":        "wireguard",
		"server":      u.Hostname(),
		"port":        port,
		"private-key": privateKey,
		"public-key":  publicKey,
		"udp":         true,
	}
	if ip := firstQuery(query, "address", "ip"); ip != "" {
		if strings.Contains(ip, ":") && !strings.Contains(ip, ".") {
			proxy["ipv6"] = ip
		} else {
			proxy["ip"] = strings.Split(ip, ",")[0]
		}
	}
	if psk := firstQuery(query, "presharedkey", "pre-shared-key", "psk"); psk != "" {
		proxy["pre-shared-key"] = psk
	}
	if mtu := query.Get("mtu"); mtu != "" {
		if n, err := strconv.Atoi(mtu); err == nil {
			proxy["mtu"] = n
		}
	}
	if reserved := query.Get("reserved"); reserved != "" {
		proxy["reserved"] = parseReserved(reserved)
	}
	return proxy, true
}

func parseReserved(raw string) any {
	if !strings.Contains(raw, ",") {
		return raw
	}
	parts := strings.Split(raw, ",")
	out := make([]any, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return raw
		}
		out = append(out, n)
	}
	return out
}

func uniqueProxyName(names map[string]int, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "proxy"
	}
	if index, ok := names[name]; ok {
		index++
		names[name] = index
		return fmt.Sprintf("%s-%02d", name, index)
	}
	names[name] = 0
	return name
}

func looksLikeProxy(m map[string]any) bool {
	if m == nil {
		return false
	}
	_, hasName := m["name"]
	_, hasType := m["type"]
	_, hasServer := m["server"]
	return hasName && (hasType || hasServer)
}

func firstQuery(query url.Values, keys ...string) string {
	for _, key := range keys {
		if value := query.Get(key); value != "" {
			return value
		}
	}
	return ""
}

package speedtester

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func tryJSONFormats(body []byte) (*RawConfig, bool) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil, false
	}
	if trimmed[0] != '{' && trimmed[0] != '[' {
		return nil, false
	}

	var object map[string]any
	if err := json.Unmarshal(body, &object); err == nil && object != nil {
		if proxies := asProxyMaps(firstSlice(object, "proxies", "Proxy", "proxy")); len(proxies) > 0 {
			return &RawConfig{Proxies: proxies, Providers: firstMapMap(object, "proxy-providers")}, true
		}
		if servers := asSlice(object["servers"]); len(servers) > 0 {
			if proxies := convertSIP008(servers); len(proxies) > 0 {
				return &RawConfig{Proxies: proxies}, true
			}
		}
		if outbounds := asSlice(object["outbounds"]); len(outbounds) > 0 {
			if proxies := convertSingBoxOutbounds(outbounds); len(proxies) > 0 {
				return &RawConfig{Proxies: proxies}, true
			}
		}
	}

	var listed []any
	if err := json.Unmarshal(body, &listed); err == nil && len(listed) > 0 {
		if proxies := asProxyMaps(listed); len(proxies) > 0 {
			return &RawConfig{Proxies: proxies}, true
		}
		if proxies := convertSIP008(listed); len(proxies) > 0 {
			return &RawConfig{Proxies: proxies}, true
		}
		if proxies := convertSingBoxOutbounds(listed); len(proxies) > 0 {
			return &RawConfig{Proxies: proxies}, true
		}
	}
	return nil, false
}

func convertSIP008(servers []any) []map[string]any {
	proxies := make([]map[string]any, 0, len(servers))
	names := make(map[string]int)
	for _, item := range servers {
		server, ok := asStringMap(item)
		if !ok {
			continue
		}
		host := firstString(server, "server", "host")
		port := firstAny(server, "server_port", "port", "serverPort")
		password := firstString(server, "password")
		method := firstString(server, "method", "cipher")
		if host == "" || port == nil || password == "" || method == "" {
			continue
		}
		name := firstString(server, "remarks", "name", "remark")
		if name == "" {
			name = host
		}
		proxy := map[string]any{
			"name":     uniqueProxyName(names, name),
			"type":     "ss",
			"server":   host,
			"port":     port,
			"cipher":   method,
			"password": password,
			"udp":      true,
		}
		if plugin := firstString(server, "plugin"); plugin != "" {
			proxy["plugin"] = plugin
			if opts := firstString(server, "plugin_opts", "plugin-opts"); opts != "" {
				proxy["plugin-opts"] = parseSIP008PluginOpts(plugin, opts)
			}
		}
		proxies = append(proxies, proxy)
	}
	return proxies
}

func parseSIP008PluginOpts(plugin, opts string) map[string]any {
	out := map[string]any{}
	for _, part := range strings.Split(opts, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "obfs":
			out["mode"] = value
		case "obfs-host", "host":
			out["host"] = value
		case "path", "obfs-uri":
			out["path"] = value
		case "mode":
			out["mode"] = value
		case "tls":
			out["tls"] = value == "true" || value == "1"
		default:
			out[key] = value
		}
	}
	if strings.Contains(plugin, "obfs") && out["mode"] == nil {
		out["mode"] = "http"
	}
	return out
}

func convertSingBoxOutbounds(outbounds []any) []map[string]any {
	proxies := make([]map[string]any, 0, len(outbounds))
	names := make(map[string]int)
	for _, item := range outbounds {
		ob, ok := asStringMap(item)
		if !ok {
			continue
		}
		if proxy, ok := convertSingBoxOutbound(ob, names); ok {
			proxies = append(proxies, proxy)
		}
	}
	return proxies
}

func convertSingBoxOutbound(ob map[string]any, names map[string]int) (map[string]any, bool) {
	typ := strings.ToLower(firstString(ob, "type"))
	switch typ {
	case "direct", "block", "dns", "selector", "urltest", "pass", "tor", "shadowtls", "naive", "tun", "redirect", "tproxy", "":
		return nil, false
	}
	name := firstString(ob, "tag", "name")
	if name == "" {
		name = fmt.Sprintf("%s-%s", typ, firstString(ob, "server"))
	}
	proxy := map[string]any{
		"name": uniqueProxyName(names, name),
		"type": mapSingBoxType(typ),
		"udp":  true,
	}
	if server := firstString(ob, "server"); server != "" {
		proxy["server"] = server
	}
	if port := firstAny(ob, "server_port", "port"); port != nil {
		proxy["port"] = port
	}

	switch typ {
	case "shadowsocks", "ss":
		proxy["cipher"] = firstString(ob, "method", "cipher")
		proxy["password"] = firstString(ob, "password")
		if plugin := firstString(ob, "plugin"); plugin != "" {
			proxy["plugin"] = plugin
			if opts := firstString(ob, "plugin_opts", "plugin-opts"); opts != "" {
				proxy["plugin-opts"] = parseSIP008PluginOpts(plugin, opts)
			}
		}
	case "vmess":
		proxy["uuid"] = firstString(ob, "uuid")
		proxy["alterId"] = firstAny(ob, "alter_id", "alterId")
		if proxy["alterId"] == nil {
			proxy["alterId"] = 0
		}
		cipher := firstString(ob, "security", "cipher")
		if cipher == "" {
			cipher = "auto"
		}
		proxy["cipher"] = cipher
	case "vless":
		proxy["uuid"] = firstString(ob, "uuid")
		if flow := firstString(ob, "flow"); flow != "" {
			proxy["flow"] = flow
		}
		if encoding := firstString(ob, "packet_encoding", "packet-encoding"); encoding != "" {
			proxy["packet-encoding"] = encoding
		}
	case "trojan":
		proxy["password"] = firstString(ob, "password")
	case "hysteria":
		proxy["auth_str"] = firstString(ob, "auth_str", "auth-str", "auth")
		proxy["up"] = firstAny(ob, "up_mbps", "up")
		proxy["down"] = firstAny(ob, "down_mbps", "down")
		if obfs := firstString(ob, "obfs"); obfs != "" {
			proxy["obfs"] = obfs
		}
	case "hysteria2":
		proxy["password"] = firstString(ob, "password")
		if up := firstAny(ob, "up_mbps", "up"); up != nil {
			proxy["up"] = up
		}
		if down := firstAny(ob, "down_mbps", "down"); down != nil {
			proxy["down"] = down
		}
		if obfs, ok := asStringMap(ob["obfs"]); ok {
			if mode := firstString(obfs, "type", "mode"); mode != "" {
				proxy["obfs"] = mode
			}
			if password := firstString(obfs, "password"); password != "" {
				proxy["obfs-password"] = password
			}
		}
	case "tuic":
		proxy["uuid"] = firstString(ob, "uuid")
		proxy["password"] = firstString(ob, "password")
		if cc := firstString(ob, "congestion_control", "congestion-controller"); cc != "" {
			proxy["congestion-controller"] = cc
		}
		if mode := firstString(ob, "udp_relay_mode", "udp-relay-mode"); mode != "" {
			proxy["udp-relay-mode"] = mode
		}
	case "socks", "socks5":
		proxy["username"] = firstString(ob, "username")
		proxy["password"] = firstString(ob, "password")
	case "http":
		proxy["username"] = firstString(ob, "username")
		proxy["password"] = firstString(ob, "password")
	case "anytls":
		proxy["password"] = firstString(ob, "password")
	case "ssh":
		proxy["username"] = firstString(ob, "user", "username")
		proxy["password"] = firstString(ob, "password")
		if key := firstString(ob, "private_key", "private-key"); key != "" {
			proxy["private-key"] = key
		}
	case "wireguard":
		applySingBoxWireGuard(proxy, ob)
	default:
		if proxy["server"] == nil {
			return nil, false
		}
	}

	if proxy["server"] == nil && typ != "wireguard" {
		return nil, false
	}
	applySingBoxTLS(proxy, ob)
	applySingBoxTransport(proxy, ob)
	return proxy, true
}

func mapSingBoxType(typ string) string {
	switch typ {
	case "shadowsocks", "ss":
		return "ss"
	case "socks":
		return "socks5"
	default:
		return typ
	}
}

func applySingBoxTLS(proxy, ob map[string]any) {
	tls, ok := asStringMap(ob["tls"])
	if !ok {
		return
	}
	enabled, _ := tls["enabled"].(bool)
	if enabled || tls["server_name"] != nil || tls["reality"] != nil {
		switch fmt.Sprint(proxy["type"]) {
		case "hysteria", "hysteria2", "tuic":
		default:
			proxy["tls"] = true
		}
	}
	if sni := firstString(tls, "server_name", "servername"); sni != "" {
		switch fmt.Sprint(proxy["type"]) {
		case "vless", "vmess":
			proxy["servername"] = sni
		default:
			proxy["sni"] = sni
		}
	}
	if insecure, ok := tls["insecure"].(bool); ok {
		proxy["skip-cert-verify"] = insecure
	}
	if alpn := tls["alpn"]; alpn != nil {
		proxy["alpn"] = alpn
	}
	if utls, ok := asStringMap(tls["utls"]); ok {
		if fp := firstString(utls, "fingerprint"); fp != "" {
			proxy["client-fingerprint"] = fp
		}
	}
	if reality, ok := asStringMap(tls["reality"]); ok {
		pub := firstString(reality, "public_key", "public-key")
		if pub != "" {
			proxy["tls"] = true
			proxy["reality-opts"] = map[string]any{
				"public-key": pub,
				"short-id":   firstString(reality, "short_id", "short-id"),
			}
		}
	}
}

func applySingBoxTransport(proxy, ob map[string]any) {
	tr, ok := asStringMap(ob["transport"])
	if !ok {
		return
	}
	switch strings.ToLower(firstString(tr, "type")) {
	case "ws":
		proxy["network"] = "ws"
		opts := map[string]any{}
		if path := firstString(tr, "path"); path != "" {
			opts["path"] = path
		}
		if headers, ok := asStringMap(tr["headers"]); ok {
			opts["headers"] = headers
		}
		proxy["ws-opts"] = opts
	case "httpupgrade":
		proxy["network"] = "ws"
		opts := map[string]any{"v2ray-http-upgrade": true}
		if path := firstString(tr, "path"); path != "" {
			opts["path"] = path
		}
		if headers, ok := asStringMap(tr["headers"]); ok {
			opts["headers"] = headers
		}
		proxy["ws-opts"] = opts
	case "grpc":
		proxy["network"] = "grpc"
		proxy["grpc-opts"] = map[string]any{
			"grpc-service-name": firstString(tr, "service_name", "serviceName"),
		}
	case "http", "h2":
		proxy["network"] = "http"
		opts := map[string]any{}
		if path := firstString(tr, "path"); path != "" {
			opts["path"] = []any{path}
		}
		if host := tr["host"]; host != nil {
			opts["host"] = host
		}
		proxy["http-opts"] = opts
	}
}

func applySingBoxWireGuard(proxy, ob map[string]any) {
	proxy["private-key"] = firstString(ob, "private_key", "private-key")
	if addrs := asSlice(ob["local_address"]); len(addrs) > 0 {
		proxy["ip"] = fmt.Sprint(addrs[0])
		if len(addrs) > 1 {
			proxy["ipv6"] = fmt.Sprint(addrs[1])
		}
	} else if ip := firstString(ob, "local_address", "address", "ip"); ip != "" {
		proxy["ip"] = ip
	}
	if mtu := firstAny(ob, "mtu"); mtu != nil {
		proxy["mtu"] = mtu
	}
	peers := asSlice(ob["peers"])
	if len(peers) > 0 {
		if peer, ok := asStringMap(peers[0]); ok {
			if server := firstString(peer, "server"); server != "" {
				proxy["server"] = server
			}
			if port := firstAny(peer, "server_port", "port"); port != nil {
				proxy["port"] = port
			}
			proxy["public-key"] = firstString(peer, "public_key", "public-key")
			if psk := firstString(peer, "pre_shared_key", "pre-shared-key"); psk != "" {
				proxy["pre-shared-key"] = psk
			}
			if reserved := peer["reserved"]; reserved != nil {
				proxy["reserved"] = reserved
			}
			if allowed := peer["allowed_ips"]; allowed != nil {
				proxy["allowed-ips"] = allowed
			}
		}
	}
	if proxy["public-key"] == nil {
		proxy["public-key"] = firstString(ob, "peer_public_key", "public_key", "public-key")
	}
}

func tryQuantumult(body []byte) (*RawConfig, bool) {
	text := string(body)
	if !looksLikeQuantumult(text) {
		return nil, false
	}
	proxies := make([]map[string]any, 0)
	names := make(map[string]int)
	for _, line := range strings.Split(text, "\n") {
		if proxy, ok := parseQuantumultLine(line, names); ok {
			proxies = append(proxies, proxy)
		}
	}
	if len(proxies) == 0 {
		return nil, false
	}
	return &RawConfig{Proxies: proxies}, true
}

func looksLikeQuantumult(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "shadowsocks=") ||
		strings.Contains(lower, "vmess=") ||
		strings.Contains(lower, "trojan=") ||
		strings.Contains(lower, "http=") ||
		strings.Contains(lower, "socks5=")
}

func parseQuantumultLine(line string, names map[string]int) (map[string]any, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
		return nil, false
	}
	proto, rest, ok := strings.Cut(line, "=")
	if !ok {
		return nil, false
	}
	proto = strings.ToLower(strings.TrimSpace(proto))
	fields := splitCommaFields(rest)
	if len(fields) == 0 {
		return nil, false
	}
	host, port, ok := splitHostPortField(fields[0])
	if !ok {
		return nil, false
	}
	opts := parseKeyValues(fields[1:])
	rawName := firstMapString(opts, "tag", "name")
	if rawName == "" {
		rawName = host
	}
	name := uniqueProxyName(names, rawName)
	switch proto {
	case "shadowsocks", "shadowsocks2018":
		method := firstMapString(opts, "method")
		password := firstMapString(opts, "password")
		if method == "" || password == "" {
			return nil, false
		}
		proxy := map[string]any{
			"name":     name,
			"type":     "ss",
			"server":   host,
			"port":     port,
			"cipher":   method,
			"password": password,
			"udp":      true,
		}
		applyQuantumultObfs(proxy, opts)
		return proxy, true
	case "vmess":
		uuid := firstMapString(opts, "password", "uuid")
		if uuid == "" {
			return nil, false
		}
		cipher := firstMapString(opts, "method")
		if cipher == "" {
			cipher = "auto"
		}
		proxy := map[string]any{
			"name":    name,
			"type":    "vmess",
			"server":  host,
			"port":    port,
			"uuid":    uuid,
			"alterId": 0,
			"cipher":  cipher,
			"udp":     true,
		}
		applyQuantumultObfs(proxy, opts)
		return proxy, true
	case "trojan":
		password := firstMapString(opts, "password")
		if password == "" {
			return nil, false
		}
		proxy := map[string]any{
			"name":     name,
			"type":     "trojan",
			"server":   host,
			"port":     port,
			"password": password,
			"udp":      true,
		}
		if overTLS(opts) {
			proxy["sni"] = firstMapString(opts, "tls-host", "obfs-host", "sni")
			proxy["skip-cert-verify"] = strings.EqualFold(firstMapString(opts, "tls-verification"), "false")
		}
		applyQuantumultObfs(proxy, opts)
		return proxy, true
	case "http":
		proxy := map[string]any{
			"name":     name,
			"type":     "http",
			"server":   host,
			"port":     port,
			"username": firstMapString(opts, "username"),
			"password": firstMapString(opts, "password"),
		}
		if overTLS(opts) {
			proxy["tls"] = true
		}
		return proxy, true
	case "socks5", "socks":
		return map[string]any{
			"name":     name,
			"type":     "socks5",
			"server":   host,
			"port":     port,
			"username": firstMapString(opts, "username"),
			"password": firstMapString(opts, "password"),
			"udp":      true,
		}, true
	default:
		return nil, false
	}
}

func applyQuantumultObfs(proxy map[string]any, opts map[string]string) {
	obfs := strings.ToLower(firstMapString(opts, "obfs"))
	host := firstMapString(opts, "obfs-host", "host")
	uri := firstMapString(opts, "obfs-uri", "uri")
	switch obfs {
	case "ws", "wss":
		proxy["network"] = "ws"
		ws := map[string]any{}
		if uri != "" {
			ws["path"] = uri
		}
		if host != "" {
			ws["headers"] = map[string]any{"Host": host}
		}
		proxy["ws-opts"] = ws
		if obfs == "wss" || overTLS(opts) {
			if fmt.Sprint(proxy["type"]) == "vmess" || fmt.Sprint(proxy["type"]) == "vless" {
				proxy["tls"] = true
				if host != "" {
					proxy["servername"] = host
				}
			} else {
				proxy["tls"] = true
				if host != "" {
					proxy["sni"] = host
				}
			}
		}
	case "over-tls", "tls":
		proxy["tls"] = true
		if host != "" {
			if fmt.Sprint(proxy["type"]) == "ss" {
				proxy["plugin"] = "obfs"
				proxy["plugin-opts"] = map[string]any{"mode": "tls", "host": host}
				delete(proxy, "tls")
			} else if fmt.Sprint(proxy["type"]) == "vmess" {
				proxy["servername"] = host
			} else {
				proxy["sni"] = host
			}
		}
	case "http":
		if fmt.Sprint(proxy["type"]) == "ss" {
			proxy["plugin"] = "obfs"
			optsMap := map[string]any{"mode": "http"}
			if host != "" {
				optsMap["host"] = host
			}
			proxy["plugin-opts"] = optsMap
		}
	}
}

func overTLS(opts map[string]string) bool {
	value := strings.ToLower(firstMapString(opts, "over-tls"))
	return value == "true" || value == "1"
}

func trySurge(body []byte) (*RawConfig, bool) {
	text := string(body)
	section := extractINISection(text, "Proxy")
	if section == "" {
		if !looksLikeSurge(text) {
			return nil, false
		}
		section = text
	}
	proxies := make([]map[string]any, 0)
	names := make(map[string]int)
	for _, line := range strings.Split(section, "\n") {
		if proxy, ok := parseSurgeLine(line, names); ok {
			proxies = append(proxies, proxy)
		}
	}
	if len(proxies) == 0 {
		return nil, false
	}
	return &RawConfig{Proxies: proxies}, true
}

func looksLikeSurge(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "[proxy]") ||
		strings.Contains(lower, "encrypt-method=") ||
		strings.Contains(lower, " = ss,") ||
		strings.Contains(lower, " = vmess,") ||
		strings.Contains(lower, " = trojan,")
}

func extractINISection(text, name string) string {
	lines := strings.Split(text, "\n")
	header := "[" + strings.ToLower(name) + "]"
	var b strings.Builder
	inSection := false
	found := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inSection = strings.ToLower(trimmed) == header
			if inSection {
				found = true
			}
			continue
		}
		if inSection {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	if !found {
		return ""
	}
	return b.String()
}

func parseSurgeLine(line string, names map[string]int) (map[string]any, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
		return nil, false
	}
	name, rest, ok := strings.Cut(line, "=")
	if !ok {
		return nil, false
	}
	name = strings.TrimSpace(name)
	fields := splitCommaFields(rest)
	if len(fields) < 3 {
		return nil, false
	}
	typ := strings.ToLower(strings.TrimSpace(fields[0]))
	server := strings.TrimSpace(fields[1])
	port := strings.TrimSpace(fields[2])
	opts := parseKeyValues(fields[3:])
	display := uniqueProxyName(names, name)
	switch typ {
	case "ss", "custom":
		method := firstMapString(opts, "encrypt-method", "method")
		password := firstMapString(opts, "password")
		if method == "" || password == "" {
			return nil, false
		}
		proxy := map[string]any{
			"name":     display,
			"type":     "ss",
			"server":   server,
			"port":     port,
			"cipher":   method,
			"password": password,
			"udp":      true,
		}
		if obfs := firstMapString(opts, "obfs"); obfs != "" {
			proxy["plugin"] = "obfs"
			plugin := map[string]any{"mode": obfs}
			if host := firstMapString(opts, "obfs-host"); host != "" {
				plugin["host"] = host
			}
			proxy["plugin-opts"] = plugin
		}
		return proxy, true
	case "vmess":
		uuid := firstMapString(opts, "username", "uuid")
		if uuid == "" {
			return nil, false
		}
		proxy := map[string]any{
			"name":    display,
			"type":    "vmess",
			"server":  server,
			"port":    port,
			"uuid":    uuid,
			"alterId": 0,
			"cipher":  "auto",
			"udp":     true,
		}
		if strings.EqualFold(firstMapString(opts, "tls"), "true") {
			proxy["tls"] = true
		}
		if sni := firstMapString(opts, "sni"); sni != "" {
			proxy["servername"] = sni
		}
		if strings.EqualFold(firstMapString(opts, "ws"), "true") {
			proxy["network"] = "ws"
			ws := map[string]any{}
			if path := firstMapString(opts, "ws-path"); path != "" {
				ws["path"] = path
			}
			if headers := firstMapString(opts, "ws-headers"); headers != "" {
				ws["headers"] = parseSurgeHeaders(headers)
			}
			proxy["ws-opts"] = ws
		}
		return proxy, true
	case "trojan":
		password := firstMapString(opts, "password")
		if password == "" {
			return nil, false
		}
		proxy := map[string]any{
			"name":     display,
			"type":     "trojan",
			"server":   server,
			"port":     port,
			"password": password,
			"udp":      true,
		}
		if sni := firstMapString(opts, "sni"); sni != "" {
			proxy["sni"] = sni
		}
		if strings.EqualFold(firstMapString(opts, "skip-cert-verify"), "true") {
			proxy["skip-cert-verify"] = true
		}
		return proxy, true
	case "socks5", "socks5-tls":
		proxy := map[string]any{
			"name":     display,
			"type":     "socks5",
			"server":   server,
			"port":     port,
			"username": firstMapString(opts, "username"),
			"password": firstMapString(opts, "password"),
			"udp":      true,
		}
		if typ == "socks5-tls" {
			proxy["tls"] = true
		}
		return proxy, true
	case "http", "https":
		proxy := map[string]any{
			"name":     display,
			"type":     "http",
			"server":   server,
			"port":     port,
			"username": firstMapString(opts, "username"),
			"password": firstMapString(opts, "password"),
		}
		if typ == "https" {
			proxy["tls"] = true
		}
		return proxy, true
	case "snell":
		psk := firstMapString(opts, "psk", "password")
		if psk == "" {
			return nil, false
		}
		proxy := map[string]any{
			"name":   display,
			"type":   "snell",
			"server": server,
			"port":   port,
			"psk":    psk,
			"udp":    true,
		}
		if version := firstMapString(opts, "version"); version != "" {
			if n, err := strconv.Atoi(version); err == nil {
				proxy["version"] = n
			}
		}
		return proxy, true
	case "hysteria2", "hy2":
		password := firstMapString(opts, "password")
		if password == "" {
			return nil, false
		}
		proxy := map[string]any{
			"name":     display,
			"type":     "hysteria2",
			"server":   server,
			"port":     port,
			"password": password,
		}
		if sni := firstMapString(opts, "sni"); sni != "" {
			proxy["sni"] = sni
		}
		if strings.EqualFold(firstMapString(opts, "skip-cert-verify"), "true") {
			proxy["skip-cert-verify"] = true
		}
		return proxy, true
	default:
		return nil, false
	}
}

func parseSurgeHeaders(raw string) map[string]any {
	headers := map[string]any{}
	for _, part := range strings.Split(raw, "|") {
		key, value, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		headers[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return headers
}

func splitCommaFields(raw string) []string {
	var fields []string
	var b strings.Builder
	inQuote := false
	for _, r := range raw {
		switch r {
		case '"':
			inQuote = !inQuote
		case ',':
			if inQuote {
				b.WriteRune(r)
				continue
			}
			fields = append(fields, strings.TrimSpace(b.String()))
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		fields = append(fields, strings.TrimSpace(b.String()))
	}
	return fields
}

func parseKeyValues(fields []string) map[string]string {
	out := make(map[string]string, len(fields))
	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		out[strings.ToLower(strings.TrimSpace(key))] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	return out
}

func splitHostPortField(raw string) (string, string, bool) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "[") {
		end := strings.Index(raw, "]")
		if end < 1 {
			return "", "", false
		}
		host := raw[1:end]
		port := strings.TrimPrefix(raw[end+1:], ":")
		if host == "" || port == "" {
			return "", "", false
		}
		return host, port, true
	}
	i := strings.LastIndex(raw, ":")
	if i <= 0 || i == len(raw)-1 {
		return "", "", false
	}
	return raw[:i], raw[i+1:], true
}

func firstSlice(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := m[key]; ok && value != nil {
			return value
		}
	}
	return nil
}

func firstMapMap(m map[string]any, keys ...string) map[string]map[string]any {
	for _, key := range keys {
		value, ok := m[key]
		if !ok || value == nil {
			continue
		}
		if typed, ok := value.(map[string]map[string]any); ok {
			return typed
		}
		outer, ok := asStringMap(value)
		if !ok {
			continue
		}
		out := make(map[string]map[string]any, len(outer))
		for name, item := range outer {
			if inner, ok := asStringMap(item); ok {
				out[name] = inner
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func asProxyMaps(v any) []map[string]any {
	slice := asSlice(v)
	out := make([]map[string]any, 0, len(slice))
	for _, item := range slice {
		m, ok := asStringMap(item)
		if !ok || !looksLikeProxy(m) {
			continue
		}
		out = append(out, m)
	}
	return out
}

func asSlice(v any) []any {
	switch s := v.(type) {
	case []any:
		return s
	case []map[string]any:
		out := make([]any, len(s))
		for i := range s {
			out[i] = s[i]
		}
		return out
	default:
		return nil
	}
}

func asStringMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[any]any:
		out := make(map[string]any, len(m))
		for key, value := range m {
			out[fmt.Sprint(key)] = value
		}
		return out, true
	default:
		return nil, false
	}
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key]; ok && value != nil {
			if s, ok := value.(string); ok {
				return s
			}
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}

func firstAny(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := m[key]; ok && value != nil {
			return value
		}
	}
	return nil
}

func firstMapString(m map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := m[key]; value != "" {
			return value
		}
	}
	return ""
}

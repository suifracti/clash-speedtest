package publicservice

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const netflixIPv4Service = "netflix_unlock_ipv4"
const netflixIPv4DNS = "https://cloudflare-dns.com/dns-query?name=www.netflix.com&type=A"

func netflixUsesResolvedIPv4(rule Rule) bool {
	return rule.ServiceID == netflixIPv4Service || (rule.ServiceID == "netflix_unlock" && rule.RuleVersion >= 3)
}

var errNetflixDNSHost = errors.New("当前解析结果仅适用于 www.netflix.com:443；不回退到其它解析方式")

// One lookup belongs to one checker/selected node. It is never a global DNS
// override, an exit IP, or a cache shared across nodes and historical attempts.
type netflixIPv4Evidence struct {
	phases                             *requestPhases
	ip                                 string
	ttl                                uint32
	expires                            time.Time
	dnsBytes                           int64
	dnsAttempts, dnsWritten, dnsStatus int
}

func (e *netflixIPv4Evidence) address(address string) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || !strings.EqualFold(host, "www.netflix.com") || port != "443" {
		return "", errNetflixDNSHost
	}
	return net.JoinHostPort(e.ip, port), nil
}

func (e *netflixIPv4Evidence) attach(result *Result) {
	if result.Details == nil {
		result.Details = map[string]string{}
	}
	d := result.Details
	d["target_dns_mode"] = "selected_node_doh_ipv4_v1"
	d["target_dns_source"] = netflixIPv4DNS
	d["target_dns_http_attempts"] = strconv.Itoa(e.dnsAttempts)
	d["target_dns_scope"] = "仅本次 Netflix 的目标解析；约束为同一被测节点代理，无系统 DNS 或直连回退"
	d["playback_verified"] = "false"
	d["account_verified"] = "false"
	if e.dnsStatus > 0 {
		d["target_dns_http_status"] = strconv.Itoa(e.dnsStatus)
	}
	if e.ip != "" {
		d["target_ipv4"] = e.ip
		d["target_dns_ttl_seconds"] = strconv.FormatUint(uint64(e.ttl), 10)
		d["target_dns_valid_until"] = e.expires.UTC().Format(time.RFC3339Nano)
		finished := result.FinishedAt
		if finished.IsZero() {
			finished = time.Now()
		}
		d["target_dns_answer_fresh_at_finish"] = strconv.FormatBool(finished.Before(e.expires))
		d["target_address_policy"] = "one_lookup_frozen_for_this_attempt"
		d["target_dns_cache_scope"] = "this_attempt_only"
	}
	e.phases.mu.Lock()
	attempts, written := e.phases.requests, e.phases.written
	e.phases.mu.Unlock()
	d["target_http_attempts"] = strconv.Itoa(max(0, attempts-e.dnsAttempts))
	d["target_http_requests_written"] = strconv.Itoa(max(0, written-e.dnsWritten))
	result.BytesRead += e.dnsBytes
}

// Clone only the selected proxy's existing transport. Rewriting DialContext
// leaves the URL, HTTP Host, SNI and normal certificate verification intact.
func (e *netflixIPv4Evidence) transport(original http.RoundTripper) (http.RoundTripper, bool) {
	switch t := original.(type) {
	case phaseTransport:
		base, ok := e.transport(t.base)
		if !ok {
			return nil, false
		}
		return phaseTransport{base: base, phases: t.phases}, true
	case *http.Transport:
		if t == nil || t.DialContext == nil || t.Proxy != nil || t.DialTLSContext != nil || t.DialTLS != nil || (t.TLSClientConfig != nil && (t.TLSClientConfig.InsecureSkipVerify || t.TLSClientConfig.ServerName != "")) {
			return nil, false
		}
		clone := t.Clone()
		dial := t.DialContext
		clone.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			target, err := e.address(address)
			if err != nil {
				return nil, err
			}
			return dial(ctx, network, target)
		}
		return clone, true
	default:
		return nil, false
	}
}

func prepareNetflixIPv4(ctx context.Context, client *http.Client, phases *requestPhases) (*http.Client, *netflixIPv4Evidence, *Result) {
	evidence := &netflixIPv4Evidence{phases: phases}
	transport, ok := evidence.transport(client.Transport)
	if !ok {
		r := Result{Outcome: "not_executed", FailurePhase: "target_dns_setup", ErrorMessage: "当前客户端无法保持所选节点及 TLS 语义；未发起 IPv4 解析或片目请求"}
		return nil, evidence, &r
	}
	// No redirect/retry of the DNS lookup. Its time is part of the original
	// checker deadline, not a new per-phase extension.
	dnsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	dnsClient := *client
	dnsClient.Jar = nil
	dnsClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	r, body, valid := boundedGET(dnsCtx, &dnsClient, netflixIPv4DNS, 16*1024, "application/dns-json")
	cancel()
	evidence.dnsBytes = r.BytesRead
	phases.mu.Lock()
	evidence.dnsAttempts, evidence.dnsWritten = phases.requests, phases.written
	phases.mu.Unlock()
	if r.HTTPStatus != nil {
		evidence.dnsStatus = *r.HTTPStatus
	}
	fail := func(message string) (*http.Client, *netflixIPv4Evidence, *Result) {
		r.BytesRead = 0 // attach adds the actual DNS body exactly once.
		if r.Outcome != "cancelled" && r.Outcome != "rate_limited" {
			r.Outcome = "unknown"
			r.HTTPStatus = nil
		}
		r.FailurePhase = "target_dns"
		r.Summary = "经节点的 DNS 查询未完成；Netflix 片目请求未发起"
		r.ErrorMessage = message
		return nil, evidence, &r
	}
	if !valid {
		return fail("经节点的 DNS 查询未完成；" + r.ErrorMessage)
	}
	var payload struct {
		Status *int
		Answer []struct {
			Type int
			TTL  uint32
			Data string
		}
	}
	if json.Unmarshal(body, &payload) != nil || payload.Status == nil || *payload.Status != 0 {
		return fail("DNS 响应格式或状态不符合规则；未发起 Netflix 片目请求")
	}
	for _, answer := range payload.Answer {
		ip, public := publicAddress(answer.Data)
		if answer.Type == 1 && public && ip.Is4() {
			evidence.ip, evidence.ttl = ip.String(), answer.TTL
			evidence.expires = time.Now().Add(time.Duration(answer.TTL) * time.Second)
			break
		}
	}
	if evidence.ip == "" {
		return fail("DNS 未返回可用的公网 IPv4；不回退到系统 DNS、IPv6或节点默认解析")
	}
	clone := *client
	clone.Transport = transport
	clone.Jar = nil
	return &clone, evidence, nil
}

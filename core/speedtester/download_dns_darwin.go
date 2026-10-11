//go:build darwin

package speedtester

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/miekg/dns"
)

// A completed wire response is required; a legacy DNSError.IsNotFound is not
// evidence of NOERROR/NODATA. These private fields do not change the saved DTO.
type physicalDNSResponseError struct{ cause error }

func (e *physicalDNSResponseError) Error() string { return e.cause.Error() }
func (e *physicalDNSResponseError) Unwrap() error { return e.cause }

type physicalDNSAnswer struct {
	RCode                                      int
	AnswerCount, AnswerRecordCount, CNAMECount int
	Addresses                                  []netip.Addr
}

func parsePhysicalDNSResponse(response *dns.Msg, qtype uint16) (physicalDNSAnswer, error) {
	if response == nil {
		return physicalDNSAnswer{}, fmt.Errorf("empty DNS response")
	}
	if len(response.Question) != 1 {
		return physicalDNSAnswer{}, fmt.Errorf("DNS response must contain exactly one question")
	}
	if response.Question[0].Qtype != qtype || response.Question[0].Qclass != dns.ClassINET {
		return physicalDNSAnswer{}, fmt.Errorf("DNS response question type does not match the requested record type")
	}
	answer := physicalDNSAnswer{RCode: response.Rcode}
	canonical := func(name string) string { return strings.ToLower(dns.Fqdn(name)) }
	questionName := canonical(response.Question[0].Name)
	targets := make(map[string]string)
	addresses := make(map[string][]netip.Addr)
	addressRecordCounts := make(map[string]int)
	for _, rr := range response.Answer {
		answer.AnswerRecordCount++
		if rr == nil || rr.Header() == nil || rr.Header().Class != dns.ClassINET {
			return answer, fmt.Errorf("DNS answer record invalid class")
		}
		owner := canonical(rr.Header().Name)
		switch record := rr.(type) {
		case *dns.CNAME:
			target := canonical(record.Target)
			if prior, exists := targets[owner]; exists && prior != target {
				return answer, fmt.Errorf("DNS response contains conflicting CNAME targets")
			}
			targets[owner] = target
		case *dns.A:
			if qtype != dns.TypeA {
				return answer, fmt.Errorf("unexpected DNS address type")
			}
			addressRecordCounts[owner]++
			if ip, ok := netip.AddrFromSlice(record.A); ok {
				addresses[owner] = append(addresses[owner], ip.Unmap())
			}
		case *dns.AAAA:
			if qtype != dns.TypeAAAA {
				return answer, fmt.Errorf("unexpected DNS address type")
			}
			addressRecordCounts[owner]++
			if ip, ok := netip.AddrFromSlice(record.AAAA); ok {
				addresses[owner] = append(addresses[owner], ip)
			}
		}
	}
	visited := make(map[string]struct{})
	for {
		if _, exists := visited[questionName]; exists {
			return answer, fmt.Errorf("DNS response contains a CNAME loop")
		}
		visited[questionName] = struct{}{}
		if target, exists := targets[questionName]; exists {
			if answer.CNAMECount >= 16 {
				return answer, fmt.Errorf("DNS response CNAME chain exceeds 16 aliases")
			}
			if addressRecordCounts[questionName] > 0 {
				return answer, fmt.Errorf("DNS CNAME owner also contains a requested address record")
			}
			answer.CNAMECount++
			questionName = target
			continue
		}
		answer.AnswerCount = addressRecordCounts[questionName]
		answer.Addresses = append(answer.Addresses, addresses[questionName]...)
		break
	}
	for owner, count := range addressRecordCounts {
		if count == 0 {
			continue
		}
		if _, inChain := visited[owner]; !inChain {
			return answer, fmt.Errorf("DNS response contains a requested address outside the CNAME chain")
		}
	}
	for owner := range targets {
		if _, inChain := visited[owner]; !inChain {
			return answer, fmt.Errorf("DNS CNAME outside requested chain")
		}
	}
	if answer.AnswerRecordCount != answer.CNAMECount+answer.AnswerCount {
		return answer, fmt.Errorf("DNS answer contains unexpected records")
	}
	return answer, nil
}

// Exactly one UDP exchange, with at most one same-qtype TCP exchange when
// truncated. No retries, search suffixes, alternate resolver or family races.
func (e *PhysicalDownloadEgress) lookupPhysicalDNS(ctx context.Context, host string, qtype uint16) (physicalDNSAnswer, error) {
	q := new(dns.Msg)
	q.SetQuestion(dns.Fqdn(host), qtype)
	q.SetEdns0(1232, false)
	for _, network := range []string{"udp", "tcp"} {
		if err := ctx.Err(); err != nil {
			return physicalDNSAnswer{}, err
		}
		// Preflight both budgets before the next dial. Write reserves its own
		// allowance again, so cancellation/budget races cannot write a new packet.
		if err := e.dnsRequests.check(ctx); err != nil {
			return physicalDNSAnswer{}, err
		}
		if err := e.dnsDials.reserve(ctx); err != nil {
			return physicalDNSAnswer{}, err
		}
		if e.dnsDial == nil {
			return physicalDNSAnswer{}, fmt.Errorf("physical DNS dial unavailable")
		}
		conn, err := e.dnsDial(ctx, network)
		if err != nil {
			return physicalDNSAnswer{}, err
		}
		conn = wrapPhysicalDNSConn(conn, ctx, e.dnsRequests)
		stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
		client := &dns.Client{Net: network, Timeout: physicalDownloadLookupTimeout, UDPSize: 1232}
		r, _, exchangeErr := client.ExchangeWithConnContext(ctx, q, &dns.Conn{Conn: conn, UDPSize: 1232})
		stop()
		conn.Close()
		if exchangeErr != nil {
			if errors.Is(exchangeErr, dns.ErrId) {
				return physicalDNSAnswer{}, &physicalDNSResponseError{cause: exchangeErr}
			}
			return physicalDNSAnswer{}, exchangeErr
		}
		if r == nil || !r.Response || r.Opcode != dns.OpcodeQuery || r.Id != q.Id || len(r.Question) != 1 ||
			!strings.EqualFold(r.Question[0].Name, q.Question[0].Name) || r.Question[0].Qtype != qtype || r.Question[0].Qclass != dns.ClassINET {
			return physicalDNSAnswer{}, &physicalDNSResponseError{cause: fmt.Errorf("physical DNS response identity invalid")}
		}
		// Even a truncated non-NOERROR response is terminal, never permission for
		// an extra TCP exchange or AAAA.
		if r.Rcode != dns.RcodeSuccess {
			return physicalDNSAnswer{RCode: r.Rcode}, nil
		}
		if r.Truncated {
			if network == "udp" {
				continue
			}
			return physicalDNSAnswer{}, &physicalDNSResponseError{cause: fmt.Errorf("physical DNS TCP response truncated")}
		}
		answer, parseErr := parsePhysicalDNSResponse(r, qtype)
		if parseErr != nil {
			return physicalDNSAnswer{}, &physicalDNSResponseError{cause: parseErr}
		}
		return answer, nil
	}
	return physicalDNSAnswer{}, fmt.Errorf("physical DNS response unavailable")
}

func (e *PhysicalDownloadEgress) resolvePhysicalDNS(ctx context.Context, host string) (netip.Addr, error) {
	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
		label := strings.ToLower(dns.TypeToString[qtype])
		if err := ctx.Err(); err != nil {
			e.setFailure(label + "_" + physicalFailureReason(ctx, err))
			return netip.Addr{}, err
		}
		answer, err := e.lookupPhysicalDNS(ctx, host, qtype)
		if err != nil {
			// Keep explicit binding failures set by the production dialer.
			if e.Snapshot().FailureReason == "" {
				e.setFailure(label + "_" + physicalFailureReason(ctx, err))
			}
			return netip.Addr{}, err
		}
		if answer.RCode != dns.RcodeSuccess {
			name, ok := dns.RcodeToString[answer.RCode]
			if !ok {
				name = fmt.Sprint(answer.RCode)
			}
			e.setFailure(label + "_rcode_" + strings.ToLower(name))
			return netip.Addr{}, fmt.Errorf("physical DNS %s returned %s", label, name)
		}
		for _, ip := range answer.Addresses {
			ip = ip.Unmap()
			if usablePhysicalAddress(ip) && ((qtype == dns.TypeA && ip.Is4()) || (qtype == dns.TypeAAAA && ip.Is6())) {
				family, source := "ipv4", "physical_ipv4_dns"
				if qtype == dns.TypeAAAA {
					family, source = "ipv6", "physical_ipv6_dns"
				}
				e.setResolution(family, source)
				return ip, nil
			}
		}
		// Unusable/fake/malformed records are not proven NODATA. Fail closed.
		if answer.AnswerCount > 0 || (answer.AnswerRecordCount > 0 && answer.CNAMECount == 0) {
			e.setFailure(label + "_records_unusable")
			return netip.Addr{}, fmt.Errorf("physical DNS %s records unusable", label)
		}
		if qtype == dns.TypeAAAA {
			e.setFailure("aaaa_nodata")
			return netip.Addr{}, fmt.Errorf("physical DNS AAAA NOERROR/NODATA")
		}
	}
	return netip.Addr{}, fmt.Errorf("physical DNS unavailable")
}

func (b *physicalRequestBudget) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.used >= b.limit {
		return errPhysicalDownloadBudgetExceeded
	}
	return nil
}

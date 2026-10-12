//go:build darwin

package speedtester

import (
	"context"
	"errors"
	"github.com/miekg/dns"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestStrictPhysicalDNSNODATAAllowsAAAA(t *testing.T) {
	server, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	done := make(chan []uint16, 1)
	go func() {
		var types []uint16
		defer func() { done <- types }()
		buf := make([]byte, 1232)
		for {
			n, peer, err := server.ReadFrom(buf)
			if err != nil {
				return
			}
			q := new(dns.Msg)
			if q.Unpack(buf[:n]) != nil {
				return
			}
			types = append(types, q.Question[0].Qtype)
			r := new(dns.Msg)
			r.SetReply(q)
			r.Authoritative = true
			if q.Question[0].Qtype == dns.TypeAAAA {
				r.Answer = []dns.RR{&dns.AAAA{Hdr: dns.RR_Header{Name: q.Question[0].Name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 60}, AAAA: net.ParseIP("2001:db8::9")}}
			}
			b, _ := r.Pack()
			server.WriteTo(b, peer)
		}
	}()
	e := newPhysicalDownloadEgress()
	e.dnsDial = func(ctx context.Context, network string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp4", server.LocalAddr().String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ip, err := e.resolve(ctx, "strict.fixture.example")
	server.Close()
	types := <-done
	if err != nil || ip != netip.MustParseAddr("2001:db8::9") || len(types) != 2 || types[0] != dns.TypeA || types[1] != dns.TypeAAAA {
		t.Fatalf("strict NODATA: ip=%v err=%v qtypes=%v path=%+v", ip, err, types, e.Snapshot())
	}
}

// net.Pipe speaks DNS stream framing even for the simulated UDP leg; the
// separate loopback UDP regression above exercises real packet framing.
type cancelAfterDNSBody struct {
	net.Conn
	cancel context.CancelFunc
}

func (c *cancelAfterDNSBody) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 2 {
		c.cancel()
	}
	return n, err
}

func TestStrictPhysicalDNSBranchesAndBudgets(t *testing.T) {
	type step struct {
		rcode           int
		records         []string
		truncate        bool
		fail            error
		cancelAfter     bool
		stall           bool
		invalidIdentity bool
		cancelBefore    bool
	}
	tests := []struct {
		name                       string
		steps                      []step
		ip, family, source, reason string
		dials, writes              int
		requestLimit, dialLimit    int
		cancelled                  bool
	}{
		{name: "A preferred", steps: []step{{records: []string{"node.example. 60 IN A 203.0.113.9"}}}, ip: "203.0.113.9", family: "ipv4", source: "physical_ipv4_dns", dials: 1, writes: 1},
		{name: "NODATA then AAAA", steps: []step{{}, {records: []string{"node.example. 60 IN AAAA 2001:db8::9"}}}, ip: "2001:db8::9", family: "ipv6", source: "physical_ipv6_dns", dials: 2, writes: 2},
		{name: "CNAME A", steps: []step{{records: []string{"node.example. 60 IN CNAME canonical.example.", "canonical.example. 60 IN A 203.0.113.9"}}}, ip: "203.0.113.9", family: "ipv4", source: "physical_ipv4_dns", dials: 1, writes: 1},
		{name: "CNAME NODATA", steps: []step{{records: []string{"node.example. 60 IN CNAME canonical.example."}}, {records: []string{"node.example. 60 IN AAAA 2001:db8::9"}}}, ip: "2001:db8::9", family: "ipv6", source: "physical_ipv6_dns", dials: 2, writes: 2},
		{name: "NXDOMAIN", steps: []step{{rcode: dns.RcodeNameError}}, reason: "a_rcode_nxdomain", dials: 1, writes: 1},
		{name: "SERVFAIL", steps: []step{{rcode: dns.RcodeServerFailure}}, reason: "a_rcode_servfail", dials: 1, writes: 1},
		{name: "REFUSED source limit", steps: []step{{rcode: dns.RcodeRefused}}, reason: "a_rcode_refused", dials: 1, writes: 1},
		{name: "legacy not found unknown", steps: []step{{fail: &net.DNSError{IsNotFound: true}}}, reason: "a_ipv4_dns_not_found", dials: 1},
		{name: "timeout", steps: []step{{stall: true}}, reason: "a_deadline_exceeded", dials: 1, writes: 1},
		{name: "cancel before A", cancelled: true, reason: "user_cancelled"},
		{name: "cancel after AAAA dial before write", steps: []step{{}, {cancelBefore: true}}, reason: "aaaa_user_cancelled", dials: 2, writes: 1},
		{name: "cancel between A AAAA", steps: []step{{cancelAfter: true}}, reason: "aaaa_user_cancelled", dials: 1, writes: 1},
		{name: "write budget before AAAA dial", steps: []step{{}}, requestLimit: 1, reason: "aaaa_physical_request_budget_exhausted", dials: 1, writes: 1},
		{name: "dial budget before AAAA", steps: []step{{}}, dialLimit: 1, reason: "aaaa_physical_request_budget_exhausted", dials: 1, writes: 1},
		{name: "UDP TC TCP failed", steps: []step{{truncate: true}, {fail: errors.New("fixture TCP failure")}}, reason: "a_physical_dns_unavailable", dials: 2, writes: 1},
		{name: "UDP TC TCP A", steps: []step{{truncate: true}, {records: []string{"node.example. 60 IN A 203.0.113.9"}}}, ip: "203.0.113.9", family: "ipv4", source: "physical_ipv4_dns", dials: 2, writes: 2},
		{name: "cancel before TC TCP", steps: []step{{truncate: true, cancelAfter: true}}, reason: "a_user_cancelled", dials: 1, writes: 1},
		{name: "TC write budget", steps: []step{{truncate: true}}, requestLimit: 1, reason: "a_physical_request_budget_exhausted", dials: 1, writes: 1},
		{name: "truncated NXDOMAIN", steps: []step{{truncate: true, rcode: dns.RcodeNameError}}, reason: "a_rcode_nxdomain", dials: 1, writes: 1},
		{name: "fake A no fallback", steps: []step{{records: []string{"node.example. 60 IN A 198.18.0.9"}}}, reason: "a_records_unusable", dials: 1, writes: 1},
		{name: "fake AAAA rejected", steps: []step{{}, {records: []string{"node.example. 60 IN AAAA fdfe:dcba:9876::9"}}}, reason: "aaaa_records_unusable", dials: 2, writes: 2},
		{name: "AAAA NODATA", steps: []step{{}, {}}, reason: "aaaa_nodata", dials: 2, writes: 2},
		{name: "identity mismatch", steps: []step{{invalidIdentity: true}}, reason: "a_physical_dns_invalid_response", dials: 1, writes: 1},
		{name: "DNS bind failure", steps: []step{{fail: errors.New("fixture bind error")}}, reason: "dns_socket_bind_failed", dials: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if tt.name == "timeout" {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 40*time.Millisecond)
				defer cancel()
			}
			e := newPhysicalDownloadEgress()
			if tt.requestLimit > 0 {
				e.dnsRequests = newPhysicalRequestBudget(tt.requestLimit)
			}
			if tt.dialLimit > 0 {
				e.dnsDials = newPhysicalRequestBudget(tt.dialLimit)
			}
			var networks []string
			var questions []chan uint16
			e.dnsDial = func(ctx context.Context, network string) (net.Conn, error) {
				i := len(networks)
				networks = append(networks, network)
				if i >= len(tt.steps) {
					t.Errorf("unexpected DNS dial %s", network)
					return nil, errors.New("unplanned dial")
				}
				s := tt.steps[i]
				if s.fail != nil {
					if tt.name == "DNS bind failure" {
						e.setFailure("dns_socket_bind_failed")
					}
					return nil, s.fail
				}
				c, peer := net.Pipe()
				ch := make(chan uint16, 1)
				questions = append(questions, ch)
				go func() {
					defer peer.Close()
					defer close(ch)
					dc := &dns.Conn{Conn: peer}
					q, err := dc.ReadMsg()
					if err != nil {
						return
					}
					ch <- q.Question[0].Qtype
					if s.stall {
						<-ctx.Done()
						return
					}
					r := new(dns.Msg)
					r.SetReply(q)
					r.Rcode = s.rcode
					r.Truncated = s.truncate
					if s.invalidIdentity {
						r.Id++
					}
					for _, record := range s.records {
						rr, err := dns.NewRR(record)
						if err != nil {
							panic(err)
						}
						r.Answer = append(r.Answer, rr)
					}
					_ = dc.WriteMsg(r)
				}()
				if s.cancelBefore {
					cancel()
				}
				if s.cancelAfter {
					return &cancelAfterDNSBody{Conn: c, cancel: cancel}, nil
				}
				return c, nil
			}
			if tt.cancelled {
				cancel()
			}
			ip, err := e.resolve(ctx, "node.example")
			p := e.Snapshot()
			if tt.ip != "" {
				if err != nil || ip.String() != tt.ip || p.AddressFamily != tt.family || p.AddressSource != tt.source {
					t.Fatalf("ip=%v err=%v path=%+v", ip, err, p)
				}
			} else {
				if err == nil || ip.IsValid() || p.FailureReason != tt.reason || p.AddressFamily != "unknown" || p.AddressSource != "unobserved" {
					t.Fatalf("expected %s; ip=%v err=%v path=%+v", tt.reason, ip, err, p)
				}
			}
			if len(networks) != tt.dials || p.DNSDialAttempts != tt.dials || p.DNSRequests != tt.writes || p.SocketBindVerified || p.TUNEvidence != "packet_route_not_observed" {
				t.Fatalf("network=%v path=%+v expected dial/write=%d/%d", networks, p, tt.dials, tt.writes)
			}
			var qtypes []uint16
			for _, ch := range questions {
				for qt := range ch {
					qtypes = append(qtypes, qt)
				}
			}
			for i, qt := range qtypes {
				want := uint16(dns.TypeA)
				if tt.family == "ipv6" || strings.HasPrefix(tt.name, "AAAA") || tt.name == "fake AAAA rejected" {
					if i == 1 {
						want = dns.TypeAAAA
					}
				}
				if qt != want {
					t.Fatalf("qtypes=%v", qtypes)
				}
			}
			if len(networks) > 1 && tt.steps[0].truncate && networks[1] != "tcp" {
				t.Fatalf("TC must repeat same qtype over TCP: %v", networks)
			}
		})
	}
}

func TestStrictPhysicalDNSMalformedAnswers(t *testing.T) {
	tests := []struct {
		name    string
		qt      uint16
		records []string
	}{
		{"unrelated A", dns.TypeA, []string{"other.example. 60 IN A 203.0.113.9"}},
		{"unrelated CNAME", dns.TypeA, []string{"other.example. 60 IN CNAME final.example."}},
		{"wrong qtype", dns.TypeA, []string{"node.example. 60 IN AAAA 2001:db8::9"}},
		{"CNAME loop", dns.TypeA, []string{"node.example. 60 IN CNAME other.example.", "other.example. 60 IN CNAME node.example."}},
		{"CNAME conflict", dns.TypeA, []string{"node.example. 60 IN CNAME a.example.", "node.example. 60 IN CNAME b.example."}},
		{"CNAME and A", dns.TypeA, []string{"node.example. 60 IN CNAME a.example.", "node.example. 60 IN A 203.0.113.9"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := new(dns.Msg)
			r.SetQuestion("node.example.", tt.qt)
			for _, s := range tt.records {
				rr, _ := dns.NewRR(s)
				r.Answer = append(r.Answer, rr)
			}
			if _, err := parsePhysicalDNSResponse(r, tt.qt); err == nil {
				t.Fatal("malformed response admitted")
			}
		})
	}
}

func TestStrictPhysicalDNSAConnectFailureNeverChangesFamily(t *testing.T) {
	e := newPhysicalDownloadEgress()
	e.iface = "fixture-no-such-interface"
	e.host = "node.example"
	e.nodeIP = netip.MustParseAddr("203.0.113.9")
	e.setResolution("ipv4", "physical_ipv4_dns")
	e.dnsDial = func(context.Context, string) (net.Conn, error) {
		t.Fatal("frozen node re-resolved after connection failure")
		return nil, nil
	}
	_, err := e.DialContext(context.Background(), "tcp", "node.example:443")
	p := e.Snapshot()
	if err == nil || p.AddressFamily != "ipv4" || p.AddressSource != "physical_ipv4_dns" || p.DNSRequests != 0 || p.DNSDialAttempts != 0 || p.FailureReason != "physical_socket_connect_failed" {
		t.Fatalf("err=%v path=%+v", err, p)
	}
}

func TestStrictPhysicalDNSFrozenIPv6PreservesIdentityAndSource(t *testing.T) {
	e := newPhysicalDownloadEgress()
	e.host = "node.example"
	e.nodeIP = netip.MustParseAddr("2001:db8::9")
	e.setResolution("ipv6", "physical_ipv6_dns")
	for _, host := range []string{e.host, e.nodeIP.String()} {
		ip, err := e.resolve(context.Background(), host)
		if err != nil || ip != e.nodeIP {
			t.Fatalf("frozen host=%s ip=%v err=%v", host, ip, err)
		}
	}
	original := map[string]any{"type": "hysteria2", "server": e.host, "password": "fixture-only"}
	config, err := clonePhysicalProxyConfig(original, e.nodeIP, "en1")
	if err != nil || config["server"] != e.nodeIP.String() || config["sni"] != e.host || original["server"] != e.host || e.Snapshot().AddressSource != "physical_ipv6_dns" {
		t.Fatalf("identity/source changed: config=%v err=%v", config, err)
	}
	if _, err = e.resolve(context.Background(), "other.example"); err == nil || e.Snapshot().DNSRequests != 0 {
		t.Fatal("frozen node allowed another destination")
	}
}

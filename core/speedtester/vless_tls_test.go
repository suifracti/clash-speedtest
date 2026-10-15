package speedtester

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/metacubex/mihomo/adapter"
	C "github.com/metacubex/mihomo/constant"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

type tlsFixtureDialer struct{ net.Dialer }

func (d *tlsFixtureDialer) ListenPacket(ctx context.Context, network, address string, remote netip.AddrPort) (net.PacketConn, error) {
	return nil, fmt.Errorf("unused")
}

func TestVLESSSourceTLSActualHandshake(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer server.Close()
	hash := sha256.Sum256(server.Certificate().Raw)
	pin := hex.EncodeToString(hash[:])
	addr := server.Listener.Addr().String()
	for _, tc := range []struct {
		name, q string
		ok      bool
	}{
		{"strict", "", false}, {"insecure", "&insecure=1", true},
		{"leaf pin", "&pcs=" + pin, true}, {"multiple pins", "&pcs=" + strings.Repeat("0", 64) + "," + pin, true},
		{"mismatch despite insecure", "&insecure=1&pcs=" + strings.Repeat("0", 64), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := parseConfigBody([]byte("vless://11111111-1111-1111-1111-111111111111@" + addr + "?type=tcp&security=tls&sni=original.example&fp=chrome" + tc.q + "#fixture"))
			if err != nil {
				t.Fatal(err)
			}
			p := cfg.Proxies[0]
			if why := p["_cst_transport_condition"]; why != nil {
				t.Fatalf("valid policy blocked: %v", why)
			}
			// Exercise the run's effective TLS layer on a real socket, preserving SNI.
			var d C.Dialer = &tlsFixtureDialer{}
			_, d, err = prepareSourceTLS(p, d)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if tc.q == "" || tc.name == "insecure" {
				d, err = newSourceTLSDialer(p, d)
				if err != nil {
					t.Fatal(err)
				}
			}
			conn, err := d.DialContext(ctx, "tcp", addr)
			if conn != nil {
				conn.Close()
			}
			if (err == nil) != tc.ok {
				t.Fatalf("handshake ok=%v, error=%v", tc.ok, err)
			}
			if p["servername"] != "original.example" || p["uuid"] != "11111111-1111-1111-1111-111111111111" {
				t.Fatal("source identity changed")
			}
		})
	}
}

func TestVLESSPinnedCARequiresChainAndOriginalName(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, _ := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	ca, _ = x509.ParseCertificate(der)
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{"original.example"}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter}
	leafDER, _ := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, key)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{leafDER, der}, PrivateKey: key}}}
	server.StartTLS()
	defer server.Close()
	hash := sha256.Sum256(der)
	pin := hex.EncodeToString(hash[:])
	for _, tc := range []struct {
		sni string
		ok  bool
	}{{"original.example", true}, {"wrong.example", false}} {
		cfg, err := parseConfigBody([]byte("vless://11111111-1111-1111-1111-111111111111@" + server.Listener.Addr().String() + "?security=tls&type=tcp&sni=" + tc.sni + "&insecure=1&pcs=" + pin + "#CA"))
		if err != nil {
			t.Fatal(err)
		}
		_, d, err := prepareSourceTLS(cfg.Proxies[0], &tlsFixtureDialer{})
		if err != nil {
			t.Fatal(err)
		}
		c, err := d.DialContext(context.Background(), "tcp", server.Listener.Addr().String())
		if c != nil {
			c.Close()
		}
		if (err == nil) != tc.ok {
			t.Fatalf("name=%s error=%v", tc.sni, err)
		}
	}
}

func TestTLSRevisionKeepsIdentityAndSeparatesPolicy(t *testing.T) {
	prefix := "vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls&type=tcp&sni=original.example&"
	cfg, err := parseConfigBody([]byte(prefix + "insecure=0#strict\n" + prefix + "insecure=1#legacy\n" + prefix + "pcs=" + strings.Repeat("a", 64) + "#pinned"))
	if err != nil {
		t.Fatal(err)
	}
	ids, revs := map[string]bool{}, map[string]bool{}
	for _, p := range cfg.Proxies {
		id := monitor.ComputeNodeIdentityKeyFromConfig(p)
		ids[id] = true
		revs[monitor.ComputeConfigRevisionKey(id, p)] = true
	}
	if len(ids) != 1 || len(revs) != 3 {
		t.Fatal("TLS policies collided or stable identity changed")
	}
}

func TestSourceTLSPoliciesSurviveEndpointDeduplication(t *testing.T) {
	prefix := "vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls&type=tcp&sni=original.example&"
	cfg, err := parseConfigBody([]byte(prefix + "insecure=0#strict\n" + prefix + "insecure=1#legacy\n" + prefix + "pcs=" + strings.Repeat("a", 64) + "#pinned"))
	if err != nil {
		t.Fatal(err)
	}
	proxies := map[string]*CProxy{}
	for _, p := range cfg.Proxies {
		proxies[p["name"].(string)] = &CProxy{Config: p}
	}
	if got := len(deduplicateProxiesByServerPort(proxies)); got != 3 {
		t.Fatalf("independent TLS policies collapsed to %d", got)
	}
}

func TestVLESSAdapterTLSBeforeCredentialWrite(t *testing.T) {
	certServer := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer certServer.Close()
	cert := certServer.TLS.Certificates[0]
	h := sha256.Sum256(cert.Certificate[0])
	pin := hex.EncodeToString(h[:])
	for _, tc := range []struct {
		name, q string
		ok      bool
	}{{"pinned", "&pcs=" + pin, true}, {"pin mismatch with insecure", "&insecure=1&pcs=" + strings.Repeat("0", 64), false}, {"strict self signed", "", false}, {"explicit insecure", "&insecure=1", true}} {
		t.Run(tc.name, func(t *testing.T) {
			sni := make(chan string, 1)
			received := make(chan []byte, 1)
			listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}, GetConfigForClient: func(h *tls.ClientHelloInfo) (*tls.Config, error) { sni <- h.ServerName; return nil, nil }})
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			go func() {
				c, err := listener.Accept()
				if err != nil {
					return
				}
				defer c.Close()
				c.SetDeadline(time.Now().Add(3 * time.Second))
				header := make([]byte, 18)
				n, _ := io.ReadFull(c, header)
				received <- header[:n]
			}()
			cfg, err := parseConfigBody([]byte("vless://11111111-1111-1111-1111-111111111111@" + listener.Addr().String() + "?type=tcp&security=tls&sni=original.example&fp=chrome" + tc.q + "#fixture"))
			if err != nil {
				t.Fatal(err)
			}
			effective, d, err := prepareSourceTLS(cfg.Proxies[0], &tlsFixtureDialer{})
			if err != nil {
				t.Fatal(err)
			}
			p, err := adapter.ParseProxy(effective, adapter.WithDialerForAPI(d))
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			c, err := p.DialContext(ctx, &C.Metadata{NetWork: C.TCP, Host: "target.example", DstPort: 80})
			if err == nil {
				_, err = c.Write([]byte("GET / HTTP/1.1\r\nHost: target.example\r\n\r\n"))
				c.Close()
			}
			if (err == nil) != tc.ok {
				t.Fatalf("adapter TLS expected ok=%v, error=%v", tc.ok, err)
			}
			if name := <-sni; name != "original.example" {
				t.Fatalf("SNI changed: %s", name)
			}
			header := <-received
			if tc.ok {
				if len(header) != 18 || !bytes.Equal(header[1:17], bytes.Repeat([]byte{0x11}, 16)) {
					t.Fatal("VLESS credential header changed")
				}
			} else if len(header) != 0 {
				t.Fatal("credentials written before TLS verification")
			}
		})
	}
}

func TestGeneratedWSHeadersDoNotSplitIdenticalShareAliases(t *testing.T) {
	prefix := "vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls&type=ws&sni=original.example&path=%2Fws#"
	for i := 0; i < 10; i++ {
		cfg, err := parseConfigBody([]byte(prefix + "first\n" + prefix + "second"))
		if err != nil {
			t.Fatal(err)
		}
		proxies := map[string]*CProxy{}
		for _, p := range cfg.Proxies {
			proxies[p["name"].(string)] = &CProxy{Config: p}
		}
		if len(deduplicateProxiesByServerPort(proxies)) != 1 {
			t.Fatal("generated UA changed alias set")
		}
	}
}

func TestUnpreparedProxyRejectsSourcePolicyBeforeDial(t *testing.T) {
	p := &CProxy{Config: map[string]any{"_cst_tls_pins": []string{strings.Repeat("a", 64)}}}
	// A nil underlying proxy would panic if it were entered.
	c, err := p.DialContext(context.Background(), &C.Metadata{Host: "target.invalid", DstPort: 443})
	if c != nil || err == nil {
		t.Fatal("unprepared source TLS reached proxy")
	}
}

func TestVLESSInvalidSourceTLSRejectedWithoutSecrets(t *testing.T) {
	for _, query := range []string{"pcs=not-a-hash", "insecure=1&allowInsecure=0", "vcn=a,,b"} {
		body := []byte("vless://11111111-1111-1111-1111-111111111111@secret-server.invalid:443?type=tcp&security=tls&sni=original.example&" + query + "#fixture")
		err := ValidateSubscriptionBody(body)
		if err == nil {
			t.Fatalf("invalid policy accepted: %s", query)
		}
		if strings.Contains(err.Error(), "secret-server") || strings.Contains(err.Error(), "11111111") {
			t.Fatal("source configuration leaked in validation error")
		}
	}
}

func TestCProxyNilAndUnderlyingNilDefense(t *testing.T) {
	var nilProxy *CProxy
	c, err := nilProxy.DialContext(context.Background(), &C.Metadata{Host: "target.invalid", DstPort: 443})
	if c != nil || err == nil || !strings.Contains(err.Error(), "proxy is nil") {
		t.Fatalf("expected proxy is nil error, got c=%v err=%v", c, err)
	}

	uninitProxy := &CProxy{}
	c, err = uninitProxy.DialContext(context.Background(), &C.Metadata{Host: "target.invalid", DstPort: 443})
	if c != nil || err == nil || !strings.Contains(err.Error(), "underlying proxy is not initialized") {
		t.Fatalf("expected underlying proxy not initialized error, got c=%v err=%v", c, err)
	}
}

func TestSourceTLSDialerNilDefense(t *testing.T) {
	var nilDialer *sourceTLSDialer
	c, err := nilDialer.DialContext(context.Background(), "tcp", "127.0.0.1:443")
	if c != nil || err == nil || !strings.Contains(err.Error(), "dialer is not initialized") {
		t.Fatalf("expected dialer is not initialized error, got c=%v err=%v", c, err)
	}

	emptyDialer := &sourceTLSDialer{}
	c, err = emptyDialer.DialContext(context.Background(), "tcp", "127.0.0.1:443")
	if c != nil || err == nil || !strings.Contains(err.Error(), "dialer is not initialized") {
		t.Fatalf("expected dialer is not initialized error on empty dialer, got c=%v err=%v", c, err)
	}
}

func TestNormalizedPinsTrailingAndEmptyCommas(t *testing.T) {
	pin1 := strings.Repeat("0", 64)
	pin2 := strings.Repeat("a", 64)
	input := fmt.Sprintf("  %s  , ,  %s ,  ", pin1, pin2)
	pins, err := normalizedPins(input)
	if err != nil {
		t.Fatalf("unexpected error parsing pins with commas: %v", err)
	}
	if len(pins) != 2 || pins[0] != pin1 || pins[1] != pin2 {
		t.Fatalf("expected 2 pins [%s, %s], got %v", pin1, pin2, pins)
	}

	// Only empty commas should yield empty slice without error
	emptyPins, err := normalizedPins(" , ,   , ")
	if err != nil {
		t.Fatalf("unexpected error on empty tokens: %v", err)
	}
	if len(emptyPins) != 0 {
		t.Fatalf("expected 0 pins, got %v", emptyPins)
	}
}


package speedtester

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	tlsC "github.com/metacubex/mihomo/component/tls"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/tls"
)

// Xray's pcs pins individual certificates. A leaf pin accepts that leaf;
// a CA pin must validate the chain AND the original verification name.
// Never map pcs to Mihomo's fingerprint: its CA mode omits hostname checks.
func sourceTLSVerifier(pins, names []string, serverName string) func([][]byte, [][]*x509.Certificate) error {
	names = append([]string(nil), names...)
	if len(names) == 0 && serverName != "" {
		names = []string{serverName}
	}
	return func(raw [][]byte, _ [][]*x509.Certificate) error {
		if len(raw) == 0 {
			return fmt.Errorf("TLS peer supplied no certificates")
		}
		certs := make([]*x509.Certificate, len(raw))
		for i, b := range raw {
			c, err := x509.ParseCertificate(b)
			if err != nil {
				return fmt.Errorf("TLS peer certificate invalid")
			}
			certs[i] = c
		}
		var roots *x509.CertPool
		if len(pins) > 0 {
			match := func(c *x509.Certificate) bool {
				h := sha256.Sum256(c.Raw)
				s := hex.EncodeToString(h[:])
				for _, p := range pins {
					if p == s {
						return true
					}
				}
				return false
			}
			if match(certs[0]) {
				return nil
			}
			for _, c := range certs[1:] {
				if c.IsCA && match(c) {
					roots = x509.NewCertPool()
					roots.AddCert(c)
					break
				}
			}
			if roots == nil {
				return fmt.Errorf("TLS pcs certificate pin mismatch")
			}
		}
		if len(names) == 0 {
			return fmt.Errorf("TLS pinned CA needs an explicit verification name")
		}
		intermediates := x509.NewCertPool()
		for _, c := range certs[1:] {
			intermediates.AddCert(c)
		}
		for _, name := range names {
			if _, err := certs[0].Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, DNSName: name, CurrentTime: time.Now()}); err == nil {
				return nil
			}
		}
		return fmt.Errorf("TLS certificate chain or verification name rejected")
	}
}
func normalizedPins(raw string) ([]string, error) {
	var pins []string
	for _, s := range strings.Split(raw, ",") {
		s = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), ":", ""))
		b, err := hex.DecodeString(s)
		if err != nil || len(b) != 32 {
			return nil, fmt.Errorf("TLS pcs must contain SHA-256 certificate hashes")
		}
		pins = append(pins, s)
	}
	sort.Strings(pins)
	return pins, nil
}
func stringList(v any) []string {
	switch x := v.(type) {
	case []string:
		return append([]string(nil), x...)
	case []any:
		var out []string
		for _, s := range x {
			if v, ok := s.(string); ok {
				out = append(out, v)
			}
		}
		return out
	}
	return nil
}

type sourceTLSDialer struct {
	C.Dialer
	config      *tls.Config
	fingerprint string
}

func newSourceTLSDialer(config map[string]any, d C.Dialer) (C.Dialer, error) {
	name, _ := config["servername"].(string)
	if name == "" {
		name, _ = config["server"].(string)
	}
	insecure, _ := config["skip-cert-verify"].(bool)
	tc := &tls.Config{ServerName: name, InsecureSkipVerify: insecure, NextProtos: stringList(config["alpn"]), MinVersion: tls.VersionTLS12}
	pins, names := stringList(config["_cst_tls_pins"]), stringList(config["_cst_tls_names"])
	if len(pins) > 0 || len(names) > 0 {
		tc.InsecureSkipVerify = true
		tc.VerifyPeerCertificate = sourceTLSVerifier(pins, names, name)
	}
	fp, _ := config["client-fingerprint"].(string)
	return &sourceTLSDialer{Dialer: d, config: tc, fingerprint: fp}, nil
}
func (d *sourceTLSDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	c, err := d.Dialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	var secured net.Conn
	if fp, ok := tlsC.GetFingerprint(d.fingerprint); ok {
		u := tlsC.UClient(c, tlsC.UConfig(d.config.Clone()), fp)
		err = u.HandshakeContext(ctx)
		secured = u
	} else {
		t := tls.Client(c, d.config.Clone())
		err = t.HandshakeContext(ctx)
		secured = t
	}
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("node TLS policy v1: %w", err)
	}
	return secured, nil
}

// Apply TLS before VLESS's lazy auth header. This uses the existing run-local
// physical dialer for DNS/TCP; no global dialer or TLS defaults are modified.
func prepareSourceTLS(original map[string]any, d C.Dialer) (map[string]any, C.Dialer, error) {
	config := make(map[string]any, len(original))
	for k, v := range original {
		config[k] = v
	}
	if len(stringList(config["_cst_tls_pins"])) == 0 && len(stringList(config["_cst_tls_names"])) == 0 {
		return config, d, nil
	}
	typ, _ := config["type"].(string)
	network, _ := config["network"].(string)
	enabled, _ := config["tls"].(bool)
	if typ != "vless" || !enabled || (network != "" && network != "tcp") || config["reality-opts"] != nil {
		return nil, nil, fmt.Errorf("pcs/vcn currently require VLESS TLS over TCP")
	}
	wrapped, err := newSourceTLSDialer(config, d)
	if err != nil {
		return nil, nil, err
	}
	config["tls"] = false // Already negotiated by this private API dialer.
	return config, wrapped, nil
}

// A CProxy which has only been parsed by Mihomo cannot silently ignore the
// source certificate policy. The physical run constructor installs it first.
func (p *CProxy) DialContext(ctx context.Context, metadata *C.Metadata) (C.Conn, error) {
	if p.Config["_cst_transport_condition"] != nil {
		return nil, &PhysicalPathError{Cause: fmt.Errorf("invalid source TLS parameters")}
	}
	if (len(stringList(p.Config["_cst_tls_pins"])) > 0 || len(stringList(p.Config["_cst_tls_names"])) > 0) && !p.sourceTLSPrepared {
		return nil, &PhysicalPathError{Cause: fmt.Errorf("source TLS policy not installed for this path")}
	}
	return p.Proxy.DialContext(ctx, metadata)
}

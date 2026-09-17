package speedtester

import (
	"encoding/base64"
	"fmt"
	"testing"
)

func TestParseConfigBodyClashYAML(t *testing.T) {
	body := []byte(`
proxies:
  - name: hk-1
    type: ss
    server: 1.1.1.1
    port: 443
    cipher: aes-128-gcm
    password: test
`)
	cfg, err := parseConfigBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Proxies) != 1 || cfg.Proxies[0]["name"] != "hk-1" {
		t.Fatalf("unexpected proxies: %#v", cfg.Proxies)
	}
}

func TestParseConfigBodyShareLinksAndBase64(t *testing.T) {
	vless := "vless://11111111-1111-1111-1111-111111111111@example.com:443?type=ws&encryption=none&host=example.com&path=%2Fpath&security=tls&sni=example.com#JP-01"
	hy2 := "hysteria2://letmein@example.com:8443/?insecure=1&sni=real.example.com#hy2test"
	plain := vless + "\n" + hy2 + "\n"

	cfg, err := parseConfigBody([]byte(plain))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Proxies) != 2 {
		t.Fatalf("plain share links: got %d proxies", len(cfg.Proxies))
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(plain))
	cfg, err = parseConfigBody([]byte(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Proxies) != 2 {
		t.Fatalf("base64 share links: got %d proxies, first=%#v", len(cfg.Proxies), cfg.Proxies)
	}
	if cfg.Proxies[0]["type"] != "vless" {
		t.Fatalf("expected vless, got %#v", cfg.Proxies[0]["type"])
	}
	if cfg.Proxies[1]["type"] != "hysteria2" {
		t.Fatalf("expected hysteria2, got %#v", cfg.Proxies[1]["type"])
	}
}

func TestParseConfigBodyRejectsGarbageWithoutLeakingBody(t *testing.T) {
	_, err := parseConfigBody([]byte("not-a-subscription"))
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() == "" {
		t.Fatal("empty error")
	}
}

func TestParseConfigBodyClashJSONAndAlias(t *testing.T) {
	body := []byte(`{"proxies":[{"name":"json-1","type":"ss","server":"1.1.1.1","port":443,"cipher":"aes-128-gcm","password":"x"}]}`)
	cfg, err := parseConfigBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Proxies) != 1 || cfg.Proxies[0]["name"] != "json-1" {
		t.Fatalf("clash json: %#v", cfg.Proxies)
	}

	alias := []byte("Proxy:\n  - {name: old-1, type: ss, server: 1.1.1.1, port: 443, cipher: aes-128-gcm, password: x}\n")
	cfg, err = parseConfigBody(alias)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Proxies) != 1 || cfg.Proxies[0]["name"] != "old-1" {
		t.Fatalf("clash Proxy alias: %#v", cfg.Proxies)
	}
}

func TestParseConfigBodySIP008(t *testing.T) {
	body := []byte(`{"version":1,"servers":[{"server":"ss.example.com","server_port":8388,"password":"pwd","method":"chacha20-ietf-poly1305","remarks":"sip-hk"}]}`)
	cfg, err := parseConfigBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Proxies) != 1 {
		t.Fatalf("sip008 count=%d", len(cfg.Proxies))
	}
	if cfg.Proxies[0]["type"] != "ss" || cfg.Proxies[0]["name"] != "sip-hk" {
		t.Fatalf("sip008 proxy=%#v", cfg.Proxies[0])
	}
}

func TestParseConfigBodySingBox(t *testing.T) {
	body := []byte(`{
  "outbounds": [
    {"type":"direct","tag":"direct"},
    {
      "type":"vless",
      "tag":"sb-jp",
      "server":"jp.example.com",
      "server_port":443,
      "uuid":"11111111-1111-1111-1111-111111111111",
      "tls":{"enabled":true,"server_name":"jp.example.com"},
      "transport":{"type":"ws","path":"/vless","headers":{"Host":"jp.example.com"}}
    }
  ]
}`)
	cfg, err := parseConfigBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Proxies) != 1 {
		t.Fatalf("sing-box should skip direct, got %d", len(cfg.Proxies))
	}
	p := cfg.Proxies[0]
	if p["type"] != "vless" || p["name"] != "sb-jp" || p["network"] != "ws" {
		t.Fatalf("sing-box proxy=%#v", p)
	}
}

func TestParseConfigBodyQuantumultAndSurge(t *testing.T) {
	qx := []byte("shadowsocks=1.2.3.4:443, method=aes-256-gcm, password=secret, obfs=tls, obfs-host=win.com, tag=QX-HK\n")
	cfg, err := parseConfigBody(qx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Proxies) != 1 || cfg.Proxies[0]["type"] != "ss" || cfg.Proxies[0]["name"] != "QX-HK" {
		t.Fatalf("quantumult: %#v", cfg.Proxies)
	}

	surge := []byte("[Proxy]\nHK = ss, 1.1.1.1, 443, encrypt-method=aes-128-gcm, password=secret, obfs=http, obfs-host=example.com\n")
	cfg, err = parseConfigBody(surge)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Proxies) != 1 || cfg.Proxies[0]["type"] != "ss" || cfg.Proxies[0]["name"] != "HK" {
		t.Fatalf("surge: %#v", cfg.Proxies)
	}
}

func TestParseConfigBodySnellWireGuardAndHy2Ports(t *testing.T) {
	snell := "snell://psk123@snell.example.com:440?version=4&obfs=http&obfs-host=example.com#snell-1"
	wg := "wireguard://PRIVATEKEY@wg.example.com:51820?publickey=PUBLICKEY&address=10.0.0.2/32&mtu=1280#wg-1"
	hy2 := "hysteria2://letmein@hy2.example.com:443/?insecure=1&sni=hy2.example.com&mport=60000-65530#hy2-hop"
	cfg, err := parseConfigBody([]byte(snell + "\n" + wg + "\n" + hy2 + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Proxies) != 3 {
		t.Fatalf("got %d proxies: %#v", len(cfg.Proxies), cfg.Proxies)
	}
	found := map[string]map[string]any{}
	for _, proxy := range cfg.Proxies {
		found[fmt.Sprint(proxy["type"])] = proxy
	}
	if found["snell"] == nil || found["wireguard"] == nil || found["hysteria2"] == nil {
		t.Fatalf("missing types: %#v", found)
	}
	if found["hysteria2"]["ports"] != "60000-65530" {
		t.Fatalf("hy2 ports not mapped: %#v", found["hysteria2"])
	}
}

func TestParseConfigBodyConcatenatedShareLinks(t *testing.T) {
	vless := "vless://11111111-1111-1111-1111-111111111111@a.example.com:443?type=tcp&security=tls#A"
	ss := "ss://YWVzLTEyOC1nY206cGFzcw@b.example.com:8388#B"
	cfg, err := parseConfigBody([]byte(vless + " " + ss))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Proxies) != 2 {
		t.Fatalf("concatenated links: got %d", len(cfg.Proxies))
	}
}

func TestParseRealHy2(t *testing.T) {
	link := "hysteria2://af1795e8-97c7-4eb4-b427-4bd63223e3dc@aws-linkhy9.lxyun.xyz:60000/?insecure=false&sni=iosapps.itunes.apple.com&pinSHA256=2b6c9b75b2ef903fbe66ee91d1801941dea0ddb5429505ae3bce65e2fb17ad45&mport=60000-65530#SG-HY2"
	cfg, err := parseConfigBody([]byte(link))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Parsed Hy2 proxy: %#v", cfg.Proxies[0])
}



func TestParseConfigBodyUTF8BOMAndURLSafeBase64(t *testing.T) {
	plain := "ss://YWVzLTEyOC1nY206cGFzcw@b.example.com:8388#BOM"
	withBOM := append([]byte{0xEF, 0xBB, 0xBF}, []byte(plain)...)
	cfg, err := parseConfigBody(withBOM)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Proxies) != 1 {
		t.Fatalf("bom: got %d", len(cfg.Proxies))
	}

	enc := base64.RawURLEncoding.EncodeToString([]byte(plain + "\n"))
	cfg, err = parseConfigBody([]byte(enc))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Proxies) != 1 {
		t.Fatalf("url-safe base64: got %d", len(cfg.Proxies))
	}
}

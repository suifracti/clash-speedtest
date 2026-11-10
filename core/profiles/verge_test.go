package profiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestVergeProfilesKeepIdentityAndImportOnlyCachedNodes(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "profiles"), 0o700); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(dir, "profiles.yaml")
	index := `current: profile-a
items:
  - {uid: rules-a, type: merge, file: rules.yaml}
  - {uid: profile-a, type: remote, name: Example subscription, file: profile-a.yaml, url: "https://example.invalid/old", updated: 1793836800, extra: {download: 0, total: 1048576}}
  - {uid: incomplete, type: remote, name: Incomplete}
`
	if err := os.WriteFile(indexPath, []byte(index), 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := LoadVergeProfiles(indexPath)
	if err != nil || len(items) != 1 {
		t.Fatalf("load remote profiles: count=%d err=%v", len(items), err)
	}
	profile := items[0]
	if profile.UID != "profile-a" || profile.URL != "https://example.invalid/old" || !profile.UpdatedAt.Equal(time.Unix(1793836800, 0)) {
		t.Fatalf("unexpected remote profile: %#v", profile)
	}
	if profile.Usage == nil || profile.Usage.Upload != nil || profile.Usage.Download == nil || *profile.Usage.Download != 0 || profile.Usage.Expire != nil {
		t.Fatalf("missing usage must stay unknown: %#v", profile.Usage)
	}
	if err = os.WriteFile(indexPath, []byte(strings.ReplaceAll(index, "/old", "/new")), 0o600); err != nil {
		t.Fatal(err)
	}
	updated, err := LoadVergeProfiles(indexPath)
	if err != nil || len(updated) != 1 || updated[0].UID != profile.UID || updated[0].URL != "https://example.invalid/new" {
		t.Fatalf("URL change must retain profile identity: count=%d err=%v", len(updated), err)
	}
	cache := `secret: do-not-import
dns: {enable: true}
rules: [MATCH,DIRECT]
proxies:
  - name: Example node
    type: ss
    server: example.invalid
    port: 443
    cipher: aes-128-gcm
    password: "001234"
    sni: example.invalid
`
	if err = os.WriteFile(filepath.Join(dir, "profiles", profile.File), []byte(cache), 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := ReadVergeProfileCache(indexPath, updated[0])
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err = yaml.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("only the proxies root should be imported: %v", got)
	}
	nodes := got["proxies"].([]any)
	node := nodes[0].(map[string]any)
	if node["password"] != "001234" || node["port"] != 443 || node["sni"] != "example.invalid" {
		t.Fatalf("cached node values changed: %v", node)
	}
}

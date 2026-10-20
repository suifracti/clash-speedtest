package profiles

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestPartialUsageHeaderPreservesKnownFields(t *testing.T) {
	tests := []struct {
		name, header string
		want         map[string]any
	}{
		{"scientific and fractional", "upload=1.024E3;download=3072.9;total=8.192e3", map[string]any{"upload": float64(1024), "download": float64(3072), "total": float64(8192)}},
		{"missing upload", "download=3072;total=8192", map[string]any{"upload": nil, "download": float64(3072), "total": float64(8192)}},
		{"invalid field", "upload=bad;download=2;total=secret", map[string]any{"upload": nil, "download": float64(2)}},
		{"last valid duplicate", "upload=1;upload=2;upload=bad;download=3", map[string]any{"upload": float64(2), "download": float64(3)}},
		{"capacity only", "total=600;expire=2000000000", map[string]any{"upload": nil, "download": nil, "total": float64(600), "expire": float64(2000000000)}},
		{"malformed diagnostics", "upload=NaN;download=Inf;total=-3", map[string]any{"upload": nil, "download": nil}},
		{"exact safe boundary", "upload=9007199254740991.1;download=0", map[string]any{"upload": nil, "download": float64(0)}},
		{"huge exponent", "upload=1e-999999999;download=0", map[string]any{"upload": nil, "download": float64(0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := parseSubscriptionUsage(tt.header)
			if u == nil {
				t.Fatal("discarded nonempty provider header")
			}
			raw, err := json.Marshal(u)
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]any{}
			if err = json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			for field, want := range tt.want {
				if got[field] != want {
					t.Errorf("%s=%v want %v", field, got[field], want)
				}
			}
			if got["raw_header"] != tt.header {
				t.Errorf("raw header not retained: %v", got["raw_header"])
			}
		})
	}
}

func TestUsageHeaderRepeatedLinesAndMetadata(t *testing.T) {
	headers := http.Header{"Subscription-Userinfo": []string{"upload=10", "download=20;total=100"}}
	raw, _ := json.Marshal(parseSubscriptionUsageHeaders(headers))
	var got map[string]any
	_ = json.Unmarshal(raw, &got)
	if got["upload"] != float64(10) || got["download"] != float64(20) || got["raw_header"] != "upload=10\ndownload=20;total=100" {
		t.Fatalf("split fields lost: %s", raw)
	}
	headers.Set("Subscription-Userinfo", "upload=bad")
	headers.Set("X-Oss-Meta-Subscription-Userinfo", "upload=5;download=7;total=100")
	raw, _ = json.Marshal(parseSubscriptionUsageHeaders(headers))
	_ = json.Unmarshal(raw, &got)
	if got["upload"] != float64(5) || got["download"] != float64(7) {
		t.Fatalf("incomplete standard header hid complete metadata: %s", raw)
	}
}

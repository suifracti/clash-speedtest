package speedtester

import "testing"

func TestNewLatencySuiteHasFourTargetsAndPreservesFrozenSix(t *testing.T) {
	target, err := LatencyTargetURL("all")
	if err != nil {
		t.Fatal(err)
	}
	urls := LatencyProbeURLs(target)
	if len(urls) != 4 {
		t.Fatalf("new scope has %d targets", len(urls))
	}
	for i, id := range []string{"cloudflare", "google", "github", "apple"} {
		want, _ := LatencyTargetURL(id)
		if urls[i] != want {
			t.Fatal(urls)
		}
	}
	if len(LatencyProbeURLs("multi://latency-v1")) != 6 {
		t.Fatal("old suite changed")
	}
}

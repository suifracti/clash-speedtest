package history

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestLatencyNetworkPathSurvivesReopenWithoutChangingLegacy(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	legacy := &LatencyTest{AttemptID: "legacy", ProfileID: "p", NodeKey: "n", TestProject: "latency_stability", RequestedAt: at, StartedAt: at, FinishedAt: at}
	if err = s.SaveLatencyTest(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	measured := *legacy
	measured.AttemptID = "physical"
	measured.NetworkPath = json.RawMessage(`{"method":"physical_socket_v2","interface":"en1","socket_bind_verified":true}`)
	if err = s.SaveLatencyTest(ctx, &measured); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveLatencyTest(ctx, &measured); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.GetLatencyTest(ctx, "physical")
	if err != nil {
		t.Fatal(err)
	}
	if string(got.NetworkPath) != string(measured.NetworkPath) {
		t.Fatalf("lost actual path: %s", got.NetworkPath)
	}
	old, err := s.GetLatencyTest(ctx, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if len(old.NetworkPath) != 0 {
		t.Fatal("legacy evidence invented")
	}
	page, err := s.QueryLatencyTests(ctx, LatencyTestFilter{ProfileID: "p", NodeKey: "n", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Tests) != 2 {
		t.Fatal("idempotency failed")
	}
	for _, v := range page.Tests {
		if v.AttemptID == "physical" && len(v.NetworkPath) == 0 {
			t.Fatal("list path lost")
		}
	}
}

package speedtester

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBoundedPhysicalContextAddsDeadlineAndHonorsCancellation(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	ctx, cancel := boundedPhysicalContext(parent, 50*time.Millisecond)
	defer cancel()
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("physical preparation must have a deadline without a parent deadline")
	}
	cancelParent()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("physical preparation did not inherit cancellation: %v", ctx.Err())
	}
}

func TestDownloadNetworkPathPreservesFrontendAddressSourceKey(t *testing.T) {
	encoded, err := json.Marshal(DownloadNetworkPath{AddressSource: "physical_ipv4_dns"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"address_source":"physical_ipv4_dns"`) || strings.Contains(string(encoded), `"resolution_source"`) {
		t.Fatalf("network path does not use the established address_source field: %s", encoded)
	}
}

func TestPhysicalRequestBudgetChecksCancellationAndExhaustion(t *testing.T) {
	budget := newPhysicalRequestBudget(2)
	if err := budget.reserve(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := budget.reserve(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := budget.reserve(context.Background()); !errors.Is(err, errPhysicalDownloadBudgetExceeded) {
		t.Fatalf("request budget should stop a third operation: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := budget.reserve(ctx); !errors.Is(err, context.Canceled) || budget.count() != 2 {
		t.Fatalf("cancelled request consumed the exhausted budget: err=%v count=%d", err, budget.count())
	}
}

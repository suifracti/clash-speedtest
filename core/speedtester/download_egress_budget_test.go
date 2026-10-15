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

func TestPhysicalRequestBudgetNilDefenses(t *testing.T) {
	var budget *physicalRequestBudget
	if err := budget.reserve(context.Background()); err != nil {
		t.Fatalf("nil budget reserve returned error: %v", err)
	}
	if got := budget.count(); got != 0 {
		t.Fatalf("nil budget count expected 0, got %d", got)
	}
}

func TestBoundedPhysicalContextNilParent(t *testing.T) {
	ctx, cancel := boundedPhysicalContext(nil, 0)
	defer cancel()
	if ctx == nil {
		t.Fatal("expected non-nil context for nil parent")
	}
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("expected deadline on bounded physical context")
	}
}

func TestPhysicalPathErrorUnwrap(t *testing.T) {
	var nilErr *PhysicalPathError
	if nilErr.Unwrap() != nil {
		t.Fatalf("expected nil unwrap from nil error, got %v", nilErr.Unwrap())
	}
	cause := errors.New("underlying cause")
	err := &PhysicalPathError{Cause: cause}
	if !errors.Is(err, cause) {
		t.Fatalf("expected errors.Is to match cause: %v", err.Unwrap())
	}
	if err.Error() != "physical download path could not be verified" {
		t.Fatalf("unexpected error message: %s", err.Error())
	}
}


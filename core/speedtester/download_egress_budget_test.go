package speedtester

import (
	"context"
	"errors"
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

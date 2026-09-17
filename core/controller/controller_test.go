package controller

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type mockController struct{}

func (m *mockController) GetVersion(ctx context.Context) (VersionInfo, error) {
	return VersionInfo{Version: "1.18.0", CoreType: "mihomo"}, nil
}

func (m *mockController) GetCapabilities(ctx context.Context) (Capabilities, error) {
	return Capabilities{CanSelectNode: true, CanTestDelay: true}, nil
}

func (m *mockController) ListGroups(ctx context.Context) ([]Group, error) {
	return []Group{
		{Name: "PROXY", Type: "Selector", Now: "HK-01", All: []string{"HK-01", "SG-01"}},
	}, nil
}

func (m *mockController) ListNodes(ctx context.Context, group string) ([]Node, error) {
	return []Node{
		{Name: "HK-01", Type: "ss", UDP: true},
		{Name: "SG-01", Type: "vmess", UDP: true},
	}, nil
}

func (m *mockController) GetCurrentSelection(ctx context.Context, group string) (string, error) {
	return "HK-01", nil
}

func (m *mockController) SelectNode(ctx context.Context, group string, nodeName string) error {
	return nil
}

func (m *mockController) TestDelay(ctx context.Context, proxyName string, url string, timeout time.Duration) (time.Duration, error) {
	return 85 * time.Millisecond, nil
}

func TestControllerInterface(t *testing.T) {
	var c Controller = &mockController{}
	ctx := context.Background()

	v, err := c.GetVersion(ctx)
	if err != nil || v.CoreType != "mihomo" {
		t.Fatalf("unexpected version info: %+v, err: %v", v, err)
	}

	groups, err := c.ListGroups(ctx)
	if err != nil || len(groups) != 1 {
		t.Fatalf("unexpected groups: %+v, err: %v", groups, err)
	}

	cur, err := c.GetCurrentSelection(ctx, "PROXY")
	if err != nil || cur != "HK-01" {
		t.Fatalf("unexpected selection: %s, err: %v", cur, err)
	}

	// Verify JSON serialization
	b, err := json.Marshal(groups[0])
	if err != nil || len(b) == 0 {
		t.Fatalf("json marshal failed: %v", err)
	}
}

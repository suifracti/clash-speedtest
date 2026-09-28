package application

import (
	"context"
	"testing"
)

func TestExportNodesClashConfigEmpty(t *testing.T) {
	svc := &AppService{}
	_, err := svc.ExportNodesClashConfig(context.Background(), ExportClashConfigRequest{
		NodeKeys: []string{},
	})
	if err == nil {
		t.Fatal("expected error on empty node keys, got nil")
	}
}

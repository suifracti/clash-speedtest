package profiles

import (
	"reflect"
	"testing"
)

func TestSubscriptionNodeVersionsChangesCancelsSameNameEndpointRotation(t *testing.T) {
	versions := &SubscriptionNodeVersions{
		Previous: []SubscriptionNode{
			{Identity: "id-old-port", Name: "台湾03家宽"},
			{Identity: "id-renamed", Name: "日本01"},
			{Identity: "id-removed", Name: "旧节点"},
		},
		Current: []SubscriptionNode{
			{Identity: "id-new-port", Name: "台湾03家宽"},
			{Identity: "id-renamed", Name: "日本 · 01"},
			{Identity: "id-added", Name: "台湾01家宽"},
		},
	}
	changes := versions.Changes()
	if changes == nil {
		t.Fatal("expected changes")
	}
	if !reflect.DeepEqual(changes.Added, []string{"台湾01家宽"}) {
		t.Fatalf("unexpected Added: %+v", changes.Added)
	}
	if !reflect.DeepEqual(changes.Removed, []string{"旧节点"}) {
		t.Fatalf("unexpected Removed: %+v", changes.Removed)
	}
	if !reflect.DeepEqual(changes.Renamed, []NodeRename{{From: "日本01", To: "日本 · 01"}}) {
		t.Fatalf("unexpected Renamed: %+v", changes.Renamed)
	}
}

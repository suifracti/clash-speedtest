package desktop

import (
	"errors"
	"github.com/faceair/clash-speedtest/core/monitor"
	"strings"
	"testing"
)

func TestDesktopStartAdmission(t *testing.T) {
	if desktopStartError(nil) != nil {
		t.Fatal("success changed")
	}
	rejected := desktopStartError(monitor.NewValidationError("frozen scope unavailable"))
	if !strings.Contains(rejected.Error(), `"admission":"not_accepted"`) {
		t.Fatal(rejected)
	}
	unknown := errors.New("storage unavailable")
	if desktopStartError(unknown) != unknown {
		t.Fatal("unknown must not become safe rejection")
	}
}

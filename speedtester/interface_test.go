package speedtester

import (
	"testing"
)

func TestAutoDetectPhysicalInterface(t *testing.T) {
	ifaceName := AutoDetectPhysicalInterface()
	t.Logf("Detected physical interface: %q", ifaceName)
	// We don't fail if empty in headless CI environments without physical NIC, but log it.
	bound := BindPhysicalInterface()
	if bound != ifaceName {
		t.Fatalf("BindPhysicalInterface() = %q, want %q", bound, ifaceName)
	}
}

package auth

import (
	"strings"
	"testing"
)

func TestGeneratePKCE(t *testing.T) {
	pkce, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE failed: %v", err)
	}

	if pkce.ChallengeMethod != "S256" {
		t.Errorf("expected ChallengeMethod S256, got %s", pkce.ChallengeMethod)
	}

	// Verifier length must be between 43 and 128
	vLen := len(pkce.Verifier)
	if vLen < 43 || vLen > 128 {
		t.Errorf("verifier length %d is out of range [43, 128]", vLen)
	}

	// Challenge length must be > 0 and not contain padding '='
	if len(pkce.Challenge) == 0 {
		t.Error("challenge is empty")
	}
	if strings.Contains(pkce.Challenge, "=") {
		t.Error("challenge should not contain padding '='")
	}

	// Verify challenge matches computed S256 of verifier
	expectedChallenge := ComputeChallengeS256(pkce.Verifier)
	if pkce.Challenge != expectedChallenge {
		t.Errorf("challenge mismatch: expected %s, got %s", expectedChallenge, pkce.Challenge)
	}
}

func TestRFC7636AppendixBVector(t *testing.T) {
	// RFC 7636 Appendix B test vector:
	// verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	// challenge = "E9Melhoa2OwvFrGMTJguCH5 невидимый..." -> "E9Melhoa2OwvFrGMTJguCH5Zw_XVjBtIx39mb_hFixE"
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	expectedChallenge := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	challenge := ComputeChallengeS256(verifier)
	if challenge != expectedChallenge {
		t.Errorf("RFC 7636 test vector mismatch: expected %s, got %s", expectedChallenge, challenge)
	}
}

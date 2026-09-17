package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// PKCECodes holds code_verifier and code_challenge conforming to RFC 7636 (Proof Key for Code Exchange).
// This is the recommended authorization flow for public/installed desktop clients.
type PKCECodes struct {
	Verifier        string `json:"code_verifier"`
	Challenge       string `json:"code_challenge"`
	ChallengeMethod string `json:"code_challenge_method"` // "S256"
}

// GeneratePKCE creates a cryptographic code_verifier and derives the S256 code_challenge.
// RFC 7636 requires:
// - code_verifier: 43 to 128 characters from [A-Z, a-z, 0-9, "-", ".", "_", "~"]
// - code_challenge: BASE64URL-ENCODE(SHA256(code_verifier)) without padding.
func GeneratePKCE() (*PKCECodes, error) {
	// 32 random bytes encode to 43 base64url characters, which satisfies the min 43 length.
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate random verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)

	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])

	return &PKCECodes{
		Verifier:        verifier,
		Challenge:       challenge,
		ChallengeMethod: "S256",
	}, nil
}

// ComputeChallengeS256 computes the RFC 7636 S256 code_challenge from a given code_verifier.
func ComputeChallengeS256(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

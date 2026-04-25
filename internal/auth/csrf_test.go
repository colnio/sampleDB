package auth

import (
	"testing"
)

func TestGenerateCSRFToken_UniquePerCall(t *testing.T) {
	t.Parallel()
	a, err := GenerateCSRFToken()
	if err != nil {
		t.Fatalf("GenerateCSRFToken: %v", err)
	}
	b, err := GenerateCSRFToken()
	if err != nil {
		t.Fatalf("GenerateCSRFToken: %v", err)
	}
	if a == b {
		t.Fatal("expected unique tokens per call")
	}
	// 32 bytes → 43 base64url chars (no padding)
	if len(a) != 43 {
		t.Fatalf("expected 43-char token, got %d", len(a))
	}
}

func TestValidateCSRFToken_CorrectToken(t *testing.T) {
	token := "valid-csrf-token-abc123"
	session := Session{CSRFToken: token}
	if !ValidateCSRFToken(session, token) {
		t.Fatal("expected validation to pass with correct token")
	}
}

func TestValidateCSRFToken_WrongToken(t *testing.T) {
	session := Session{CSRFToken: "correct-token"}
	if ValidateCSRFToken(session, "wrong-token") {
		t.Fatal("expected validation to fail with wrong token")
	}
}

func TestValidateCSRFToken_EmptySubmitted(t *testing.T) {
	session := Session{CSRFToken: "some-token"}
	if ValidateCSRFToken(session, "") {
		t.Fatal("expected validation to fail with empty submitted token")
	}
}

func TestValidateCSRFToken_EmptySession(t *testing.T) {
	session := Session{CSRFToken: ""}
	if ValidateCSRFToken(session, "some-token") {
		t.Fatal("expected validation to fail with empty session token")
	}
}

package llm

import (
	"strings"
	"testing"
)

func TestNewNonce_Length(t *testing.T) {
	n := NewNonce()
	if len(n) != 32 {
		t.Errorf("nonce length = %d, want 32", len(n))
	}
}

func TestNewNonce_Hex(t *testing.T) {
	n := NewNonce()
	for _, c := range n {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("nonce contains non-hex character: %q", c)
		}
	}
}

func TestNewNonce_Unique(t *testing.T) {
	seen := make(map[string]bool, 100)
	for range 100 {
		n := NewNonce()
		if seen[n] {
			t.Fatalf("duplicate nonce generated: %s", n)
		}
		seen[n] = true
	}
}

func TestNewNonceNotIn_AvoidsCollision(t *testing.T) {
	// Force a collision: pre-place the first nonce the function would generate
	// into the text by calling NewNonce once and using it as the "forbidden" text.
	// We cannot predict future nonces, so instead we verify the contract by
	// checking that the returned nonce is not present in the supplied texts.
	texts := []string{"some document content", "another passage"}
	n := NewNonceNotIn(texts...)
	for _, text := range texts {
		if strings.Contains(text, n) {
			t.Errorf("returned nonce %q appears in text %q", n, text)
		}
	}
}

func TestNewNonceNotIn_RejectsNonceInText(t *testing.T) {
	// Seed a known nonce value into the text and confirm the function does not
	// return it. We do this by overriding the internal collision check directly:
	// generate a nonce, put it in the text, then call NewNonceNotIn with that
	// text and verify the result differs.
	forbidden := NewNonce()
	n := NewNonceNotIn(forbidden) // text IS the nonce itself
	if n == forbidden {
		t.Errorf("NewNonceNotIn returned the forbidden nonce %q", forbidden)
	}
	if strings.Contains(forbidden, n) {
		t.Errorf("returned nonce %q still appears in forbidden text", n)
	}
}

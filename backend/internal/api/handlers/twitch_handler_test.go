package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// TestVerifyEventSubSignature locks in the exact algorithm from Twitch's
// docs (dev.twitch.tv/docs/eventsub/handling-webhook-events): HMAC-SHA256
// over messageID+timestamp+rawBody, "sha256=" prefix, hex-encoded, compared
// time-safely. Computed independently here (not by calling the function
// under test) so a regression that changes the concatenation order, drops
// the prefix, or hashes the wrong bytes gets caught rather than just
// re-asserting whatever the code currently does.
func TestVerifyEventSubSignature(t *testing.T) {
	secret := "s3cret0123456789" // Twitch requires 10-100 ASCII chars
	messageID := "84c1e79a-2a4b-4c13-ba0b-4312293e9308"
	timestamp := "2026-08-12T14:21:26.000Z"
	body := []byte(`{"subscription":{"id":"abc"},"event":{"user_name":"testuser"}}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(messageID + timestamp + string(body)))
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !verifyEventSubSignature(secret, messageID, timestamp, body, want) {
		t.Fatalf("expected valid signature to verify")
	}
	if verifyEventSubSignature(secret, messageID, timestamp, body, "sha256=deadbeef") {
		t.Fatalf("expected tampered signature to fail")
	}
	if verifyEventSubSignature("wrong-secret-0123456789", messageID, timestamp, body, want) {
		t.Fatalf("expected signature computed with a different secret to fail")
	}
	if verifyEventSubSignature(secret, messageID, timestamp, []byte(`{"tampered":true}`), want) {
		t.Fatalf("expected signature to fail once the body changes")
	}
}

// TestVerifyEventSubSignatureOrderMatters guards specifically against
// swapping messageID/timestamp order, since Twitch's docs call that out
// explicitly ("the order is important").
func TestVerifyEventSubSignatureOrderMatters(t *testing.T) {
	secret := "s3cret0123456789"
	a, b := "id-part", "2026-01-01T00:00:00Z"
	body := []byte(`{}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(a + b + string(body)))
	signedAB := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !verifyEventSubSignature(secret, a, b, body, signedAB) {
		t.Fatalf("expected id+timestamp order to verify")
	}
	if verifyEventSubSignature(secret, b, a, body, signedAB) {
		t.Fatalf("expected timestamp+id (swapped order) to fail verification")
	}
}

func TestWidgetPageHTML(t *testing.T) {
	html := widgetPageHTML("abc123token")

	if !strings.Contains(html, `"abc123token"`) {
		t.Fatalf("expected the token to appear as a JS string literal in the page")
	}
	if !strings.Contains(html, "/ws/widgets/") {
		t.Fatalf("expected the page to connect to the widget WS endpoint")
	}
	if strings.Count(html, "<html>") != 1 || !strings.Contains(html, "</html>") {
		t.Fatalf("expected exactly one well-formed <html> root element")
	}
}

func TestJSStringLiteralEscapesHTML(t *testing.T) {
	// The token itself is always our own randomToken() hex output, never
	// attacker-controlled, but the escaping is still checked so a future
	// caller can't accidentally reintroduce an XSS via this helper.
	out := jsStringLiteral(`</script><script>alert(1)</script>`)
	if strings.Contains(out, "<script>") {
		t.Fatalf("expected jsStringLiteral to escape markup, got: %s", out)
	}
}

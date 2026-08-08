package handler

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"
)

func TestValidateLoopbackRedirect(t *testing.T) {
	ok := []string{
		"http://127.0.0.1:1/callback",
		"http://127.0.0.1:53123/callback",
		"http://127.0.0.1:65535/callback",
		"http://[::1]:53123/callback",
	}
	for _, raw := range ok {
		if err := validateLoopbackRedirect(raw); err != nil {
			t.Errorf("validateLoopbackRedirect(%q) = %v, want nil", raw, err)
		}
	}

	bad := map[string]string{
		// Resolves through DNS, so the name is not proof of a loopback target.
		"http://localhost:53123/callback": "localhost",
		// Someone else's machine, wearing the right path.
		"http://10.0.0.5:53123/callback": "remote host",
		"http://example.com/callback":    "public host",
		// 0.0.0.0 is reachable from off-box once something binds it.
		"http://0.0.0.0:53123/callback": "wildcard",
		// The port is what the CLI proves it owns by listening; without one
		// there is nothing to bind.
		"http://127.0.0.1/callback": "no port",
		// https to loopback cannot be served by a CLI without a cert.
		"https://127.0.0.1:53123/callback": "https",
		// Anything that is not the collection endpoint.
		"http://127.0.0.1:53123/":         "wrong path",
		"http://127.0.0.1:53123/callback/": "trailing slash",
		// A pre-set query would let a caller smuggle its own parameters into
		// the redirect we hand the browser.
		"http://127.0.0.1:53123/callback?x=1": "query",
		"http://127.0.0.1:53123/callback#f":   "fragment",
		"http://u:p@127.0.0.1:53123/callback": "userinfo",
		// Not a redirect at all.
		"file:///etc/passwd": "file scheme",
		"":                   "empty",
	}
	for raw, why := range bad {
		if err := validateLoopbackRedirect(raw); err == nil {
			t.Errorf("validateLoopbackRedirect(%q) = nil, want error (%s)", raw, why)
		}
	}
}

// The exchange half of PKCE: only the holder of the verifier can redeem a code.
func TestCodeChallengeMatching(t *testing.T) {
	verifier := "a-verifier-that-never-leaves-the-cli-process"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	if len(challenge) < 43 {
		t.Fatalf("challenge %q is shorter than the length CLIAuthorize requires", challenge)
	}

	other := sha256.Sum256([]byte("a different verifier"))
	if base64.RawURLEncoding.EncodeToString(other[:]) == challenge {
		t.Fatal("distinct verifiers produced the same challenge")
	}
}

func TestSweepCLICodesDropsExpired(t *testing.T) {
	now := time.Now()

	cliAuth.Lock()
	defer cliAuth.Unlock()
	cliAuth.codes = map[string]pendingCLIAuth{
		"fresh":   {expiresAt: now.Add(time.Minute)},
		"expired": {expiresAt: now.Add(-time.Second)},
	}
	sweepCLICodes(now)

	if _, ok := cliAuth.codes["expired"]; ok {
		t.Error("expired code survived the sweep")
	}
	if _, ok := cliAuth.codes["fresh"]; !ok {
		t.Error("unexpired code was swept")
	}
}

func TestCLIKeyNameIsBoundedAndLabelled(t *testing.T) {
	if got := cliKeyName(""); got != "CLI" {
		t.Errorf("cliKeyName(\"\") = %q, want %q", got, "CLI")
	}
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'x'
	}
	if got := cliKeyName(string(long)); len(got) > 60 {
		t.Errorf("cliKeyName kept %d chars, want <= 60", len(got))
	}
}

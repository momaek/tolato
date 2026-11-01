package attestation

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestVerificationContract(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/fixtures/attestation-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Secret string
		Now    int64
		Claims Claims
		Token  string
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, token, secret, aud, nonce string
		at                              int64
		valid                           bool
	}{
		{"valid", f.Token, f.Secret, "patato", f.Claims.Nonce, f.Now, true},
		{"last-second", f.Token, f.Secret, "patato", f.Claims.Nonce, f.Now + 59, true},
		{"expiry", f.Token, f.Secret, "patato", f.Claims.Nonce, f.Now + 60, false},
		{"future", f.Token, f.Secret, "patato", f.Claims.Nonce, f.Now - 6, false},
		{"rotated-secret", f.Token, "other", "patato", f.Claims.Nonce, f.Now, false},
		{"wrong-audience", f.Token, f.Secret, "other", f.Claims.Nonce, f.Now, false},
		{"wrong-nonce", f.Token, f.Secret, "patato", strings.Repeat("x", 32), f.Now, false},
		{"tampered", f.Token[:20] + "x" + f.Token[21:], f.Secret, "patato", f.Claims.Nonce, f.Now, false},
		{"oversized", strings.Repeat("x", 4097), f.Secret, "patato", f.Claims.Nonce, f.Now, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := Verify(tc.token, tc.secret, tc.aud, tc.nonce, time.Unix(tc.at, 0))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if tc.valid && claims.NodeID != f.Claims.NodeID {
				t.Fatal("wrong node")
			}
		})
	}
}

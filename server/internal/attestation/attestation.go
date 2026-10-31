// Package attestation verifies the ta1 protocol implemented by tolato-agent.
package attestation

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const domain = "tolato/attestation/v1"

var ErrInvalid = errors.New("invalid or expired attestation")

type Claims struct {
	NodeID    string `json:"node_id"`
	Audience  string `json:"audience"`
	Nonce     string `json:"nonce"`
	IssuedAt  int64  `json:"issued_at"`
	ExpiresAt int64  `json:"expires_at"`
}

// Decode is only for finding the signing node. No decoded claim is trusted
// until Verify succeeds with that node's current secret.
func Decode(token string) (Claims, error) {
	var claims Claims
	parts := strings.Split(token, ".")
	if len(token) > 4096 || len(parts) != 3 || parts[0] != "ta1" {
		return claims, ErrInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || json.Unmarshal(raw, &claims) != nil || claims.NodeID == "" {
		return claims, ErrInvalid
	}
	return claims, nil
}

func Verify(token, secret, audience, nonce string, now time.Time) (Claims, error) {
	claims, err := Decode(token)
	if err != nil {
		return claims, err
	}
	parts := strings.Split(token, ".")
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	derive := hmac.New(sha256.New, []byte(secret))
	derive.Write([]byte(domain))
	mac := hmac.New(sha256.New, derive.Sum(nil))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if err != nil || secret == "" || !hmac.Equal(sig, mac.Sum(nil)) ||
		len(audience) < 1 || len(audience) > 128 || len(nonce) < 16 || len(nonce) > 256 ||
		claims.Audience != audience || claims.Nonce != nonce || claims.IssuedAt > now.Unix()+5 ||
		claims.ExpiresAt <= now.Unix() || claims.ExpiresAt <= claims.IssuedAt || claims.ExpiresAt-claims.IssuedAt > 60 {
		return Claims{}, ErrInvalid
	}
	return claims, nil
}

package attestation

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/momaek/tolato/agent/internal/identity"
)

func TestSigningContract(t *testing.T) {
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
	token, err := Sign(&identity.Identity{NodeID: f.Claims.NodeID, Secret: f.Secret}, f.Claims.Audience, f.Claims.Nonce, time.Unix(f.Now, 0))
	if err != nil || token != f.Token {
		t.Fatalf("agent/server signing contract changed: %v", err)
	}
	if _, err := Sign(nil, "patato", f.Claims.Nonce, time.Now()); err == nil {
		t.Fatal("unregistered agent signed")
	}
	if _, err := Sign(&identity.Identity{NodeID: "n", Secret: "s"}, "patato", "short", time.Now()); err == nil {
		t.Fatal("short nonce accepted")
	}
}

func TestLocalHTTP(t *testing.T) {
	id := &identity.Identity{NodeID: "n", Secret: "must-not-leak"}
	h := Handler(func() (*identity.Identity, error) { return id, nil })
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{"audience":"patato","nonce":"0123456789abcdef"}`, 200},
		{`{"audience":"patato","nonce":"short"}`, 400},
		{`{"audience":"patato","nonce":"0123456789abcdef","secret":"x"}`, 400},
		{strings.Repeat("x", 5000), 400},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/attestations", strings.NewReader(tc.body)))
		if w.Code != tc.code || strings.Contains(w.Body.String(), id.Secret) {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("proof response can be cached")
		}
	}
	id = nil
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/attestations", strings.NewReader(`{"audience":"patato","nonce":"0123456789abcdef"}`)))
	if w.Code != 503 {
		t.Fatal("unregistered agent served a proof")
	}
}

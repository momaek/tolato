// Package attestation serves short-lived machine proofs over a root-only socket.
package attestation

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/momaek/tolato/agent/internal/identity"
	"github.com/momaek/tolato/agent/internal/localguard"
)

const Domain = "tolato/attestation/v1"

type Claims struct {
	NodeID    string `json:"node_id"`
	Audience  string `json:"audience"`
	Nonce     string `json:"nonce"`
	IssuedAt  int64  `json:"issued_at"`
	ExpiresAt int64  `json:"expires_at"`
}

func Sign(id *identity.Identity, audience, nonce string, now time.Time) (string, error) {
	if id == nil || id.NodeID == "" || id.Secret == "" {
		return "", errors.New("agent has not registered")
	}
	if len(audience) < 1 || len(audience) > 128 || len(nonce) < 16 || len(nonce) > 256 {
		return "", errors.New("audience must be 1–128 bytes and nonce 16–256 bytes")
	}
	claims := Claims{id.NodeID, audience, nonce, now.Unix(), now.Add(time.Minute).Unix()}
	raw, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(raw)
	derive := hmac.New(sha256.New, []byte(id.Secret))
	derive.Write([]byte(Domain))
	mac := hmac.New(sha256.New, derive.Sum(nil))
	mac.Write([]byte("ta1." + payload))
	return "ta1." + payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func Handler(load func() (*identity.Identity, error)) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/attestations", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		var req struct {
			Audience string `json:"audience"`
			Nonce    string `json:"nonce"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		dec.DisallowUnknownFields()
		if dec.Decode(&req) != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		id, err := load()
		if err != nil || id == nil {
			http.Error(w, "agent identity unavailable", http.StatusServiceUnavailable)
			return
		}
		token, err := Sign(id, req.Audience, req.Nonce, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
	})
	return mux
}

// Listen refuses symlinks, shared directories and existing paths. Binding in a
// root-only directory prevents access during the brief bind-before-chmod window.
func Listen(path string) (net.Listener, error) {
	if os.Geteuid() != 0 {
		return nil, errors.New("attestation socket requires root")
	}
	if !filepath.IsAbs(path) || strings.IndexByte(path, 0) >= 0 {
		return nil, errors.New("socket path must be absolute")
	}
	dir := filepath.Dir(path)
	canonical, err := localguard.PrivateDirectory(dir)
	if err != nil {
		return nil, err
	}
	path = filepath.Join(canonical, filepath.Base(path))
	// A crash can leave a socket behind. Never unlink a regular file or an
	// active listener, including another instance of this agent.
	if old, err := os.Lstat(path); err == nil {
		if old.Mode()&os.ModeSocket == 0 {
			return nil, errors.New("socket path already exists and is not a socket")
		}
		conn, err := net.DialTimeout("unix", path, time.Second)
		if err == nil {
			conn.Close()
			return nil, errors.New("socket is already active")
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

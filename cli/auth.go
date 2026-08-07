package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
	"time"
)

// Browser login (RFC 8252 loopback redirect + PKCE).
//
// `tolato auth login` listens on a random loopback port, sends the browser to
// the server's consent screen, and waits for it to come back with a one-time
// code. The code is then traded for an API key over a direct POST — the key
// never rides in a URL, where it would land in browser history, a Referer
// header, and the access log of every proxy in between.
//
// The verifier behind the code challenge stays in this process. That is what
// makes the loopback port safe to expose: another local program can hit
// /callback, but the state check rejects it, and even a stolen code buys
// nothing without the verifier.

// authWait bounds the whole browser round trip. Long enough to sign in and read
// the consent screen, short enough that a forgotten terminal stops listening.
const authWait = 3 * time.Minute

func runAuth(args []string) error {
	if len(args) == 0 {
		return errUsage
	}
	switch args[0] {
	case "login":
		return runAuthLogin(args[1:])
	case "status":
		return runAuthStatus(args[1:])
	case "logout":
		return runAuthLogout(args[1:])
	default:
		return fmt.Errorf("unknown auth command %q\n\n%s", args[0], usage)
	}
}

func runAuthLogin(args []string) error {
	fs := flag.NewFlagSet("auth login", flag.ContinueOnError)
	serverURL := fs.String("url", "", "Tolato server URL (default: TOLATO_URL, or the configured one)")
	noBrowser := fs.Bool("no-browser", false, "print the URL instead of opening a browser")
	if err := fs.Parse(args); err != nil {
		return err
	}

	base, err := resolveServerURL(*serverURL)
	if err != nil {
		return err
	}

	verifier, err := randomToken()
	if err != nil {
		return err
	}
	state, err := randomToken()
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	// Explicitly 127.0.0.1, never :0 on all interfaces — a callback listener
	// reachable from the network is a callback anyone on it can answer.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen on loopback: %w", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	authURL := base + "/cli-auth?" + url.Values{
		"redirect_uri":   {redirectURI},
		"state":          {state},
		"code_challenge": {challenge},
		"label":          {machineLabel()},
	}.Encode()

	fmt.Fprintf(os.Stderr, "Opening %s\n", base+"/cli-auth")
	if *noBrowser {
		fmt.Fprintf(os.Stderr, "\n  %s\n\n", authURL)
	} else if err := openBrowser(authURL); err != nil {
		fmt.Fprintf(os.Stderr, "Could not open a browser (%v). Open this URL yourself:\n\n  %s\n\n", err, authURL)
	}
	fmt.Fprintln(os.Stderr, "Waiting for authorization...")

	code, err := awaitCallback(ln, state)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	granted, err := exchangeCode(ctx, base, code, verifier)
	if err != nil {
		return err
	}

	cfg := Config{URL: base, APIKey: granted.APIKey}
	if err := writeConfigFile(cfg); err != nil {
		return err
	}

	fmt.Printf("Authorized as %q (%s).\nKey written to %s\n", granted.Name, granted.Permission, configPath())
	return nil
}

func runAuthStatus(args []string) error {
	fs := flag.NewFlagSet("auth status", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	fmt.Printf("Server: %s\n", cfg.URL)
	fmt.Printf("Key:    %s\n", maskKey(cfg.APIKey))
	fmt.Printf("Config: %s\n", configPath())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The only check that means anything is whether the server still accepts
	// the key: it may have been revoked, or its owner disabled, since login.
	nodes, err := newClient(cfg).ListNodes(ctx)
	if err != nil {
		fmt.Printf("Status: not working — %v\n", err)
		return err
	}
	fmt.Printf("Status: ok, %d node(s) visible\n", len(nodes))
	return nil
}

func runAuthLogout(args []string) error {
	fs := flag.NewFlagSet("auth logout", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Revoke before forgetting. Deleting only the local copy would leave a live
	// credential on the server that nobody is tracking any more.
	revokeErr := newClient(cfg).RevokeOwnKey(ctx)

	cfg.APIKey = ""
	if err := writeConfigFile(cfg); err != nil {
		return err
	}

	if revokeErr != nil {
		return fmt.Errorf("local key removed, but revoking it on the server failed: %w", revokeErr)
	}
	fmt.Printf("Key revoked and removed from %s\n", configPath())
	return nil
}

// awaitCallback serves exactly one request on the loopback listener and returns
// the code the browser brought back.
func awaitCallback(ln net.Listener, state string) (string, error) {
	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		// Any local page can guess this port and call it. Matching state is
		// what proves the caller is the browser we sent out.
		if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1 {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return // not our callback; keep waiting for the real one
		}

		if e := q.Get("error"); e != "" {
			respondHTML(w, "Authorization denied.", "You can close this window.")
			done <- result{err: fmt.Errorf("authorization denied (%s)", e)}
			return
		}

		code := q.Get("code")
		if code == "" {
			respondHTML(w, "Something went wrong.", "No code was returned. You can close this window.")
			done <- result{err: fmt.Errorf("callback carried no code")}
			return
		}

		respondHTML(w, "Authorized.", "You can close this window and return to the terminal.")
		done <- result{code: code}
	})

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	select {
	case res := <-done:
		return res.code, res.err
	case <-time.After(authWait):
		return "", fmt.Errorf("timed out after %s waiting for authorization", authWait)
	}
}

type grantedKey struct {
	APIKey     string `json:"api_key"`
	Name       string `json:"name"`
	Permission string `json:"permission"`
}

// exchangeCode trades the one-time code for the key. Deliberately a plain POST
// rather than a method on client: there is no API key to authenticate with yet,
// which is the whole point of the exchange.
func exchangeCode(ctx context.Context, base, code, verifier string) (*grantedKey, error) {
	c := newClient(Config{URL: base})
	var out grantedKey
	body := map[string]string{"code": code, "code_verifier": verifier}
	if err := c.do(ctx, http.MethodPost, "/api/auth/cli/exchange", body, &out); err != nil {
		return nil, err
	}
	if out.APIKey == "" {
		return nil, fmt.Errorf("server returned no key")
	}
	return &out, nil
}

func respondHTML(w http.ResponseWriter, title, detail string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>tolato</title>`+
		`<body style="font-family:system-ui,sans-serif;display:flex;height:90vh;align-items:center;justify-content:center">`+
		`<div style="text-align:center"><h1 style="font-size:1.25rem;margin:0 0 .5rem">%s</h1>`+
		`<p style="color:#666;margin:0">%s</p></div>`, title, detail)
}

// resolveServerURL picks the server to log in to: the flag, then the
// environment, then whatever a previous login wrote.
func resolveServerURL(flagValue string) (string, error) {
	candidates := []string{flagValue, os.Getenv("TOLATO_URL")}
	if fileCfg, err := readConfigFile(); err == nil {
		candidates = append(candidates, fileCfg.URL)
	}
	for _, c := range candidates {
		if c = strings.TrimSpace(c); c != "" {
			return strings.TrimSuffix(c, "/"), nil
		}
	}
	return "", fmt.Errorf("no server URL: pass --url https://tolato.example.com or set TOLATO_URL")
}

// machineLabel identifies where this CLI runs, so the key it obtains is
// recognisable in Settings → API Keys and can be revoked on its own. Just the
// machine — the consent screen and the key name supply their own wording.
func machineLabel() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown host"
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username + "@" + host
	}
	return host
}

func openBrowser(target string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	return exec.Command(cmd, append(args, target)...).Start()
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// maskKey shows enough to tell two keys apart and not enough to use one.
func maskKey(key string) string {
	if len(key) <= 12 {
		return "set"
	}
	return key[:12] + "..."
}

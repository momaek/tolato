// Package services manages patato-node as a supervised child of tolato-agent.
// Manifests survive reconnects and agent restarts. Downloads are pinned by hash;
// no remote shell command or client-selected executable path is accepted.
package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/momaek/tolato/agent/internal/localguard"
)

const Name = "patato-node"
const maxBinary = 256 << 20

type Request struct {
	Action    string `json:"action"`
	BinaryURL string `json:"binary_url,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
	// JSON is also valid YAML, which is patato-node's configuration format.
	Config json.RawMessage `json:"config,omitempty"`
}

type Status struct {
	Enabled  bool   `json:"enabled"`
	Running  bool   `json:"running"`
	PID      int    `json:"pid,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Restarts int    `json:"restarts"`
	Error    string `json:"error,omitempty"`
}

type manifest struct {
	Enabled bool   `json:"enabled"`
	SHA256  string `json:"sha256"`
}

type Manager struct {
	opMu   sync.Mutex
	mu     sync.Mutex
	dir    string
	ctx    context.Context
	cancel context.CancelFunc
	stop   context.CancelFunc
	done   chan struct{}
	cfg    manifest
	status Status
	http   *http.Client
	closed bool
	// command is a test seam for a short-lived helper, without downloading or
	// executing a real patato-node during supervision tests.
	command func(context.Context, string, string) *exec.Cmd
}

func New(dir string) (*Manager, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return nil, errors.New("managed services require Linux and root")
	}
	canonical, err := localguard.PrivateDirectory(dir)
	if err != nil {
		return nil, err
	}
	return open(canonical)
}

func open(dir string) (*Manager, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("services directory must be private (0700)")
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{dir: dir, ctx: ctx, cancel: cancel, http: &http.Client{Timeout: 2 * time.Minute}}
	m.command = func(ctx context.Context, bin, cfg string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, bin, "run", "-c", cfg)
		configureCommand(cmd)
		// Never relay child output or configuration tokens through the API.
		cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
		return cmd
	}
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err == nil {
		if json.Unmarshal(raw, &m.cfg) != nil || !validDigest(m.cfg.SHA256) {
			cancel()
			return nil, errors.New("invalid service manifest")
		}
		m.status.Enabled, m.status.SHA256 = m.cfg.Enabled, m.cfg.SHA256
		if m.cfg.Enabled {
			if err := m.verifyInstalled(); err != nil {
				cancel()
				return nil, err
			}
			if err := m.start(); err != nil {
				m.status.Error = "failed to start installed service"
			}
		}
	} else if !os.IsNotExist(err) {
		cancel()
		return nil, err
	}
	return m, nil
}

func validDigest(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == sha256.Size && s == strings.ToLower(s)
}

func (m *Manager) Status() Status { m.mu.Lock(); defer m.mu.Unlock(); return m.status }

func (m *Manager) verifyInstalled() error {
	f, err := os.Open(filepath.Join(m.dir, m.cfg.SHA256))
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != m.cfg.SHA256 {
		return errors.New("installed service digest mismatch")
	}
	return nil
}

func (m *Manager) Apply(ctx context.Context, req Request) (Status, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if m.closed {
		return m.Status(), errors.New("service manager is closed")
	}
	downloadCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(m.ctx, cancel)
	defer stop()
	defer cancel()
	switch req.Action {
	case "status":
		return m.Status(), nil
	case "install", "upgrade":
		if req.Action == "upgrade" && m.cfg.SHA256 == "" {
			return m.Status(), errors.New("service is not installed")
		}
		if err := m.install(downloadCtx, req); err != nil {
			return m.Status(), err
		}
	case "restart":
		if m.cfg.SHA256 == "" {
			return m.Status(), errors.New("service is not installed")
		}
		if err := m.verifyInstalled(); err != nil {
			return m.Status(), err
		}
		wasEnabled := m.cfg.Enabled
		m.cfg.Enabled = true
		if err := m.save(); err != nil {
			m.cfg.Enabled = wasEnabled
			return m.Status(), err
		}
		m.halt()
		if err := m.start(); err != nil {
			return m.Status(), errors.New("failed to start service")
		}
	case "stop":
		if m.cfg.SHA256 == "" {
			return m.Status(), errors.New("service is not installed")
		}
		wasEnabled := m.cfg.Enabled
		m.cfg.Enabled = false
		if err := m.save(); err != nil {
			m.cfg.Enabled = wasEnabled
			return m.Status(), err
		}
		m.halt()
		m.mu.Lock()
		m.status.Enabled = false
		m.mu.Unlock()
	default:
		return m.Status(), errors.New("unknown service action")
	}
	return m.Status(), nil
}

func (m *Manager) install(ctx context.Context, req Request) error {
	if !validDigest(req.SHA256) {
		return errors.New("sha256 must be 64 lowercase hex characters")
	}
	u, err := url.Parse(req.BinaryURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return errors.New("binary_url must be HTTPS without credentials or fragment")
	}
	oldConfig, readErr := os.ReadFile(filepath.Join(m.dir, "config.json"))
	config := req.Config
	if len(config) == 0 {
		if readErr != nil {
			return errors.New("config is required for first install")
		}
		config = oldConfig
	}
	var obj map[string]json.RawMessage
	if len(config) > 65536 || json.Unmarshal(config, &obj) != nil || obj == nil {
		return errors.New("config must be a JSON object up to 64 KiB")
	}
	// Validate the existing Patato contract without ever including values in errors.
	var panel, token string
	if json.Unmarshal(obj["panel"], &panel) != nil || json.Unmarshal(obj["token"], &token) != nil || panel == "" || token == "" {
		return errors.New("config requires panel and token")
	}
	parsed, err := url.Parse(panel)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return errors.New("config panel must be HTTPS")
	}
	reqHTTP, err := http.NewRequestWithContext(ctx, http.MethodGet, req.BinaryURL, nil)
	if err != nil {
		return errors.New("invalid download request")
	}
	// Forbid HTTPS -> HTTP redirect downgrades, including a custom mirror.
	client := *m.http
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 10 || r.URL.Scheme != "https" || r.URL.User != nil {
			return errors.New("unsafe download redirect")
		}
		return nil
	}
	resp, err := client.Do(reqHTTP)
	if err != nil {
		return errors.New("service binary download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("service binary download failed")
	}
	f, err := os.CreateTemp(m.dir, ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, maxBinary+1))
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil || n == 0 || n > maxBinary {
		return errors.New("invalid or incomplete service binary")
	}
	if hex.EncodeToString(h.Sum(nil)) != req.SHA256 {
		return errors.New("service binary sha256 mismatch")
	}
	if err := os.Chmod(f.Name(), 0700); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), filepath.Join(m.dir, req.SHA256)); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	old := m.cfg
	m.halt()
	rollback := func() {
		m.cfg = old
		if readErr == nil {
			_ = atomicWrite(filepath.Join(m.dir, "config.json"), oldConfig)
		} else {
			_ = os.Remove(filepath.Join(m.dir, "config.json"))
		}
		if old.SHA256 != "" {
			_ = m.save()
			if old.Enabled {
				_ = m.start()
			}
		} else {
			_ = os.Remove(filepath.Join(m.dir, "manifest.json"))
			m.mu.Lock()
			m.status = Status{}
			m.mu.Unlock()
		}
	}
	if err := atomicWrite(filepath.Join(m.dir, "config.json"), config); err != nil {
		rollback()
		return err
	}
	m.cfg = manifest{Enabled: true, SHA256: req.SHA256}
	if err := m.save(); err != nil {
		rollback()
		return err
	}
	if err := m.start(); err != nil {
		rollback()
		return errors.New("new service could not start; restored previous version")
	}
	return nil
}

func atomicWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (m *Manager) save() error {
	data, _ := json.Marshal(m.cfg)
	return atomicWrite(filepath.Join(m.dir, "manifest.json"), data)
}

func (m *Manager) start() error {
	ctx, cancel := context.WithCancel(m.ctx)
	m.stop, m.done = cancel, make(chan struct{})
	first := make(chan error, 1)
	bin, cfg := filepath.Join(m.dir, m.cfg.SHA256), filepath.Join(m.dir, "config.json")
	m.mu.Lock()
	m.status = Status{Enabled: true, SHA256: m.cfg.SHA256}
	m.mu.Unlock()
	go func() {
		defer close(m.done)
		initial := true
		backoff := time.Second
		for ctx.Err() == nil {
			cmd := m.command(ctx, bin, cfg)
			err := cmd.Start()
			m.mu.Lock()
			if err == nil {
				m.status.Running, m.status.PID, m.status.Error = true, cmd.Process.Pid, ""
			} else {
				m.status.Error = "failed to start service"
			}
			m.mu.Unlock()
			if initial {
				first <- err
				initial = false
				if err != nil {
					return
				}
			}
			started := time.Now()
			if err == nil {
				err = cmd.Wait()
			}
			m.mu.Lock()
			m.status.Running, m.status.PID = false, 0
			if ctx.Err() == nil {
				m.status.Restarts++
				if err != nil {
					m.status.Error = "service exited; restarting"
				} else {
					m.status.Error = "service stopped; restarting"
				}
			}
			m.mu.Unlock()
			if time.Since(started) > time.Minute {
				backoff = time.Second
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
		if initial {
			first <- ctx.Err()
		}
	}()
	return <-first
}

func (m *Manager) halt() {
	if m.stop != nil {
		m.stop()
		<-m.done
		m.stop = nil
	}
}

func (m *Manager) Close() {
	m.cancel()
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.closed = true
	m.halt()
}

// PrepareExec stops children while excluding concurrent service mutations. If
// exec fails, call the returned function to restore supervision and unlock.
func (m *Manager) PrepareExec() func() {
	m.opMu.Lock()
	m.halt()
	return func() {
		if !m.closed && m.cfg.Enabled {
			_ = m.start()
		}
		m.opMu.Unlock()
	}
}

// String does not include the download URL, config, tokens, or child output.
func (s Status) String() string { return fmt.Sprintf("enabled=%t running=%t", s.Enabled, s.Running) }

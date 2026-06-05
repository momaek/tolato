package client

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// UpdateResultPayload reports the outcome of a self-update back to the server.
type UpdateResultPayload struct {
	OK         bool   `json:"ok"`
	OldVersion string `json:"old_version"`
	NewVersion string `json:"new_version,omitempty"`
	Error      string `json:"error,omitempty"`
}

// handleUpdate downloads the latest agent binary via the server's /releases
// proxy chain, verifies it, swaps it into place, reports the result, and
// restarts so the new binary takes over.
func (c *Client) handleUpdate(msg WSMessage) {
	res := c.doUpdate()
	if res.OK {
		log.Printf("[update] success %s -> %s, restarting", res.OldVersion, res.NewVersion)
	} else {
		log.Printf("[update] failed: %s", res.Error)
	}
	_ = c.sendMessage("update_result", msg.ID, res)

	// Only restart when the binary actually changed. doUpdate leaves the old
	// binary in place when the latest release matches the running version.
	if res.OK && res.NewVersion != res.OldVersion {
		// Let the result frame flush over the socket before we exec away.
		time.Sleep(500 * time.Millisecond)
		if err := restartSelf(); err != nil {
			log.Printf("[update] restart failed: %v", err)
		}
	}
}

func (c *Client) doUpdate() UpdateResultPayload {
	res := UpdateResultPayload{OldVersion: Version}

	dlURL, err := c.releaseURL()
	if err != nil {
		res.Error = "build download url: " + err.Error()
		return res
	}
	log.Printf("[update] downloading %s", dlURL)

	exePath, err := os.Executable()
	if err != nil {
		res.Error = "resolve executable: " + err.Error()
		return res
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}

	// Download into the same directory as the binary so the final rename is an
	// atomic same-filesystem move.
	tmp, err := os.CreateTemp(filepath.Dir(exePath), ".tolato-agent-update-*")
	if err != nil {
		res.Error = "create temp file: " + err.Error()
		return res
	}
	tmpPath := tmp.Name()
	// Removed if we bail before renaming; harmless no-op after a successful rename.
	defer os.Remove(tmpPath)

	if err := downloadTo(c.ctx, dlURL, tmp); err != nil {
		tmp.Close()
		res.Error = "download: " + err.Error()
		return res
	}
	tmp.Close()

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		res.Error = "chmod: " + err.Error()
		return res
	}

	// Verify the downloaded binary actually runs and report its version. This
	// guards against a truncated/corrupt download bricking the agent on restart.
	newVer, err := verifyBinary(tmpPath)
	if err != nil {
		res.Error = "verify downloaded binary: " + err.Error()
		return res
	}
	res.NewVersion = newVer

	// Already on the latest version — nothing to swap, no restart needed.
	if newVer == Version {
		log.Printf("[update] already at latest version %s, skipping", Version)
		res.OK = true
		return res
	}

	// Atomic replace. On unix, renaming over the running binary is safe: the
	// running process keeps the old inode open until it exits.
	if err := os.Rename(tmpPath, exePath); err != nil {
		res.Error = "replace binary: " + err.Error()
		return res
	}

	res.OK = true
	return res
}

// releaseURL derives the binary download URL from the server WS URL, reusing the
// exact /releases proxy chain that scripts/install.sh uses (server → GitHub).
func (c *Client) releaseURL() (string, error) {
	u, err := url.Parse(c.serverURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	case "http", "https":
		// already fine
	default:
		return "", fmt.Errorf("unsupported server scheme %q", u.Scheme)
	}

	base := strings.TrimSuffix(u.Path, "/ws/agent")
	base = strings.TrimRight(base, "/")
	asset := fmt.Sprintf("tolato-agent-%s-%s", runtime.GOOS, runtime.GOARCH)

	u.Path = base + "/releases/latest/download/" + asset
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func downloadTo(ctx context.Context, rawURL string, dst io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "tolato-agent/"+Version)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	n, err := io.Copy(dst, resp.Body)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("downloaded 0 bytes")
	}
	return nil
}

func verifyBinary(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return "", fmt.Errorf("empty --version output")
	}
	return v, nil
}

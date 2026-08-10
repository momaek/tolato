package main

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is what the CLI needs to reach one server.
type Config struct {
	URL    string `yaml:"url"`
	APIKey string `yaml:"api_key"`
}

// defaultProfile names the profile created when there is nothing to name it
// after: the first login on a fresh machine, and the home of a pre-profile
// config that gets migrated.
const defaultProfile = "default"

// configFile is the on-disk shape. A set of named profiles, one of them
// current, so one machine can hold credentials for several Tolato deployments
// — typically a company one and a personal one — without them overwriting each
// other.
//
// LegacyURL and LegacyAPIKey are the pre-profile format, where the file held a
// single server at the top level. They are read so an existing config keeps
// working, folded into a profile by migrate(), and never written back.
type configFile struct {
	Current  string            `yaml:"current,omitempty"`
	Profiles map[string]Config `yaml:"profiles,omitempty"`

	LegacyURL    string `yaml:"url,omitempty"`
	LegacyAPIKey string `yaml:"api_key,omitempty"`
}

// loadConfig resolves which profile to act on and returns its settings.
//
// TOLATO_URL and TOLATO_API_KEY still override the individual fields, so a
// single shell can target a server that has no profile at all — but only when
// the profile was not named explicitly. A typed --profile beats an exported
// variable: the alternative is that a stale TOLATO_URL in someone's shell
// silently redirects a command they thought they had aimed by name, which is
// the one mistake this whole feature exists to prevent.
func loadConfig(explicitProfile string) (Config, error) {
	file, err := readConfigFile()
	if err != nil {
		return Config{}, err
	}

	name, err := file.resolve(explicitProfile)
	if err != nil {
		return Config{}, err
	}
	cfg := file.Profiles[name]

	if strings.TrimSpace(explicitProfile) == "" {
		if v := strings.TrimSpace(os.Getenv("TOLATO_URL")); v != "" {
			cfg.URL = v
		}
		if v := strings.TrimSpace(os.Getenv("TOLATO_API_KEY")); v != "" {
			cfg.APIKey = v
		}
	}

	if cfg.URL == "" {
		return cfg, fmt.Errorf("no server URL%s: run `tolato auth login --url <server>` or set TOLATO_URL (config: %s)",
			forProfile(name), configPath())
	}
	if cfg.APIKey == "" {
		return cfg, fmt.Errorf("no API key%s: run `tolato auth login` or set TOLATO_API_KEY (config: %s)",
			forProfile(name), configPath())
	}
	cfg.URL = strings.TrimSuffix(cfg.URL, "/")
	return cfg, nil
}

func forProfile(name string) string {
	if name == "" {
		return ""
	}
	return fmt.Sprintf(" for profile %q", name)
}

// resolve picks the profile to act on: an explicit --profile, then
// $TOLATO_PROFILE, then the file's `current`, then the only profile there is.
//
// A profile asked for by name that does not exist is an error rather than a
// fallback. Quietly running against a different server than the one named is
// the failure this returns an error to avoid.
func (f *configFile) resolve(explicit string) (string, error) {
	asked := []struct{ name, source string }{
		{strings.TrimSpace(explicit), "--profile"},
		{strings.TrimSpace(os.Getenv("TOLATO_PROFILE")), "$TOLATO_PROFILE"},
	}
	for _, a := range asked {
		if a.name == "" {
			continue
		}
		if _, ok := f.Profiles[a.name]; !ok {
			return "", fmt.Errorf("%s %q: no such profile%s", a.source, a.name, f.available())
		}
		return a.name, nil
	}

	if _, ok := f.Profiles[f.Current]; ok && f.Current != "" {
		return f.Current, nil
	}
	if len(f.Profiles) == 1 {
		return f.names()[0], nil
	}
	if len(f.Profiles) > 1 {
		// Several profiles and nothing says which. Guessing here would mean
		// picking someone's company fleet by coin flip.
		return "", fmt.Errorf("no profile selected%s: run `tolato profile use <name>` or pass --profile <name>", f.available())
	}
	// Nothing configured at all. Not an error yet — the environment may supply
	// everything, and `auth login` exists precisely for this state.
	return "", nil
}

// names returns the configured profile names in a stable order.
func (f *configFile) names() []string {
	out := make([]string, 0, len(f.Profiles))
	for name := range f.Profiles {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (f *configFile) available() string {
	if len(f.Profiles) == 0 {
		return " (none configured)"
	}
	return " (have: " + strings.Join(f.names(), ", ") + ")"
}

func (f *configFile) set(name string, cfg Config) {
	if f.Profiles == nil {
		f.Profiles = map[string]Config{}
	}
	f.Profiles[name] = cfg
}

// migrate folds a pre-profile config into a profile named "default".
//
// In memory only. A read must not rewrite a file nobody asked to change, so the
// new shape reaches disk at the next write — a login, a logout, a profile
// switch — and a CLI that only ever reads leaves the old file untouched.
func (f *configFile) migrate() {
	if f.LegacyURL == "" && f.LegacyAPIKey == "" {
		return
	}
	if _, taken := f.Profiles[defaultProfile]; !taken {
		f.set(defaultProfile, Config{URL: f.LegacyURL, APIKey: f.LegacyAPIKey})
		if f.Current == "" {
			f.Current = defaultProfile
		}
	}
	f.LegacyURL, f.LegacyAPIKey = "", ""
}

var profileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// validateProfileName keeps names to what reads well in a shell and survives a
// round trip through YAML as a mapping key.
func validateProfileName(name string) error {
	if !profileNamePattern.MatchString(name) {
		return fmt.Errorf("invalid profile name %q: use letters, digits, dot, dash or underscore (max 64)", name)
	}
	return nil
}

// profileNameFromURL suggests a name for a server from its hostname, picking
// the label a person would use for the deployment itself:
//
//	https://tolato.corp.com   -> corp
//	https://corp.com          -> corp
//	http://localhost:8080     -> localhost
//	http://10.0.0.4:8080      -> 10-0-0-4
func profileNameFromURL(raw string) string {
	host := strings.TrimSpace(raw)
	if u, err := url.Parse(host); err == nil && u.Host != "" {
		host = u.Hostname()
	}
	host = strings.ToLower(strings.Trim(host, "/"))
	if host == "" {
		return defaultProfile
	}
	if net.ParseIP(host) != nil {
		return sanitizeProfileName(host)
	}

	labels := strings.Split(host, ".")
	switch {
	case len(labels) >= 3:
		return sanitizeProfileName(labels[1]) // drop the service subdomain
	case len(labels) == 2:
		return sanitizeProfileName(labels[0])
	default:
		return sanitizeProfileName(host)
	}
}

func sanitizeProfileName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		default:
			b.WriteByte('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" || validateProfileName(name) != nil {
		return defaultProfile
	}
	return name
}

// chooseProfile decides which profile a login writes to.
//
// The URL match matters more than it looks: without it, logging in again to a
// server already saved under a hand-picked name would derive a name from the
// host and quietly create a second profile pointing at the same place, leaving
// the user with two half-live credentials and no idea which one is in use.
func chooseProfile(f *configFile, explicit, serverURL string) (string, error) {
	if explicit = strings.TrimSpace(explicit); explicit != "" {
		if err := validateProfileName(explicit); err != nil {
			return "", err
		}
		return explicit, nil
	}

	want := strings.TrimSuffix(serverURL, "/")
	for _, name := range f.names() {
		if strings.EqualFold(strings.TrimSuffix(f.Profiles[name].URL, "/"), want) {
			return name, nil
		}
	}
	if len(f.Profiles) == 0 {
		return defaultProfile, nil
	}
	return uniqueProfileName(f, profileNameFromURL(serverURL)), nil
}

// uniqueProfileName appends a counter rather than overwriting a profile that
// happens to derive the same name from a different server.
func uniqueProfileName(f *configFile, base string) string {
	if _, taken := f.Profiles[base]; !taken {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if _, taken := f.Profiles[candidate]; !taken {
			return candidate
		}
	}
}

// configPath is where the CLI looks for its settings: $TOLATO_CONFIG if set,
// otherwise ~/.config/tolato/config.yaml (honouring $XDG_CONFIG_HOME).
//
// Resolving this by hand rather than through os.UserConfigDir() is deliberate.
// That function returns each platform's official application config directory,
// which on macOS is ~/Library/Application Support — the right answer for a
// bundled GUI app, and the wrong one for a command-line tool, where ~/.config
// is what people expect on every platform. It stays as a last resort only.
func configPath() string {
	if p := strings.TrimSpace(os.Getenv("TOLATO_CONFIG")); p != "" {
		return p
	}
	if dir := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); dir != "" {
		return filepath.Join(dir, "tolato", "config.yaml")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "tolato", "config.yaml")
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "tolato", "config.yaml")
	}
	// No home directory at all. Return something openable and printable rather
	// than a literal "~/...", which nothing in Go expands.
	return filepath.Join(".config", "tolato", "config.yaml")
}

func readConfigFile() (*configFile, error) {
	f := &configFile{}
	data, err := os.ReadFile(configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil // absent file is fine; the environment may supply everything
		}
		return f, fmt.Errorf("read %s: %w", configPath(), err)
	}
	if err := yaml.Unmarshal(data, f); err != nil {
		return f, fmt.Errorf("parse %s: %w", configPath(), err)
	}
	f.migrate()
	return f, nil
}

// writeConfigFile persists the config, creating the directory if needed.
//
// The file holds API keys, so it is written 0600 under a 0700 directory, and
// replaced through a temporary file so an interrupted write cannot leave a
// half-written credential behind.
func writeConfigFile(f *configFile) error {
	path := configPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	data, err := yaml.Marshal(f)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	defer os.Remove(tmp.Name())

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

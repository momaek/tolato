package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// isolate points the config at a temporary file and clears the environment
// variables that would otherwise leak the developer's own setup into a test.
func isolate(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("TOLATO_CONFIG", path)
	t.Setenv("TOLATO_PROFILE", "")
	t.Setenv("TOLATO_URL", "")
	t.Setenv("TOLATO_API_KEY", "")
	if contents != "" {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}
	return path
}

const twoProfiles = `current: personal
profiles:
  personal:
    url: https://tolato.me.example.com
    api_key: tlat_personalkey000
  work:
    url: https://tolato.corp.example.com
    api_key: tlat_workkey0000000
`

// A config written before profiles existed must keep working untouched.
func TestLegacyConfigMigratesToDefaultProfile(t *testing.T) {
	path := isolate(t, "url: https://tolato.example.com\napi_key: tlat_legacy0000000\n")

	file, err := readConfigFile()
	if err != nil {
		t.Fatalf("readConfigFile: %v", err)
	}
	want := Config{URL: "https://tolato.example.com", APIKey: "tlat_legacy0000000"}
	if got := file.Profiles[defaultProfile]; got != want {
		t.Errorf("migrated profile = %+v, want %+v", got, want)
	}
	if file.Current != defaultProfile {
		t.Errorf("current = %q, want %q", file.Current, defaultProfile)
	}

	cfg, err := loadConfig("")
	if err != nil {
		t.Fatalf("loadConfig on a legacy file: %v", err)
	}
	if cfg != want {
		t.Errorf("loadConfig = %+v, want %+v", cfg, want)
	}

	// Reading must not rewrite the user's file behind their back.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.HasPrefix(string(after), "url:") {
		t.Errorf("a read rewrote the config file:\n%s", after)
	}
}

func TestResolveProfilePrecedence(t *testing.T) {
	isolate(t, twoProfiles)
	file, err := readConfigFile()
	if err != nil {
		t.Fatalf("readConfigFile: %v", err)
	}

	if got, _ := file.resolve(""); got != "personal" {
		t.Errorf("current should apply, got %q", got)
	}

	t.Setenv("TOLATO_PROFILE", "work")
	if got, _ := file.resolve(""); got != "work" {
		t.Errorf("$TOLATO_PROFILE should beat current, got %q", got)
	}
	if got, _ := file.resolve("personal"); got != "personal" {
		t.Errorf("--profile should beat $TOLATO_PROFILE, got %q", got)
	}

	// A name that does not exist is an error, never a silent fallback to
	// whatever happens to be current.
	t.Setenv("TOLATO_PROFILE", "")
	if _, err := file.resolve("staging"); err == nil {
		t.Error("unknown --profile should error")
	} else if !strings.Contains(err.Error(), "personal") {
		t.Errorf("error should list what exists, got %v", err)
	}

	// Nothing current and several to choose from: refuse rather than guess.
	file.Current = ""
	if _, err := file.resolve(""); err == nil {
		t.Error("ambiguous selection should error")
	}
}

// An exported TOLATO_URL must not quietly redirect a command that named its
// profile explicitly — that is the mistake profiles exist to prevent.
func TestExplicitProfileOutranksEnvironment(t *testing.T) {
	isolate(t, twoProfiles)
	t.Setenv("TOLATO_URL", "https://elsewhere.example.com")
	t.Setenv("TOLATO_API_KEY", "tlat_fromenv000000")

	cfg, err := loadConfig("work")
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.URL != "https://tolato.corp.example.com" || cfg.APIKey != "tlat_workkey0000000" {
		t.Errorf("--profile work resolved to %+v, want the work profile untouched by the environment", cfg)
	}

	// Without an explicit profile the variables still work as a one-off target.
	cfg, err = loadConfig("")
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.URL != "https://elsewhere.example.com" || cfg.APIKey != "tlat_fromenv000000" {
		t.Errorf("environment override = %+v, want both fields from the environment", cfg)
	}
}

// With no config file at all, the environment alone is still enough.
func TestEnvironmentOnlyNeedsNoProfile(t *testing.T) {
	isolate(t, "")
	t.Setenv("TOLATO_URL", "https://tolato.example.com/")
	t.Setenv("TOLATO_API_KEY", "tlat_env0000000000")

	cfg, err := loadConfig("")
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.URL != "https://tolato.example.com" {
		t.Errorf("URL = %q, want the trailing slash trimmed", cfg.URL)
	}
}

func TestChooseProfileForLogin(t *testing.T) {
	isolate(t, twoProfiles)
	file, err := readConfigFile()
	if err != nil {
		t.Fatalf("readConfigFile: %v", err)
	}

	// Logging in again to a server already saved refreshes that profile rather
	// than growing a duplicate under a derived name.
	if got, _ := chooseProfile(file, "", "https://tolato.corp.example.com"); got != "work" {
		t.Errorf("re-login picked %q, want the existing \"work\"", got)
	}
	if got, _ := chooseProfile(file, "staging", "https://tolato.corp.example.com"); got != "staging" {
		t.Errorf("--profile picked %q, want \"staging\"", got)
	}
	// A new server derives a name from its host.
	if got, _ := chooseProfile(file, "", "https://tolato.acme.example.com"); got != "acme" {
		t.Errorf("derived name = %q, want \"acme\"", got)
	}
	// A derived name already taken by a different server gets a counter, so an
	// existing profile is never overwritten.
	if got, _ := chooseProfile(file, "", "https://tolato.work.example.com"); got != "work-2" {
		t.Errorf("collision resolved to %q, want \"work-2\"", got)
	}
	if _, err := chooseProfile(file, "not a name", "https://x.example.com"); err == nil {
		t.Error("invalid --profile name should be rejected")
	}

	// The very first login on a clean machine has nothing to name itself after.
	if got, _ := chooseProfile(&configFile{}, "", "https://tolato.corp.example.com"); got != defaultProfile {
		t.Errorf("first login named %q, want %q", got, defaultProfile)
	}
}

func TestProfileNameFromURL(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"https://tolato.corp.com", "corp"},
		{"https://corp.com", "corp"},
		{"https://tolato.example.co.uk", "example"},
		{"http://localhost:8080", "localhost"},
		{"http://10.0.0.4:8080", "10-0-0-4"},
		{"", defaultProfile},
	} {
		if got := profileNameFromURL(tc.in); got != tc.want {
			t.Errorf("profileNameFromURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --profile is global, but everything after `--` belongs to the remote shell:
// taking a flag out of there would rewrite what runs on the node.
func TestExtractProfileFlag(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in      []string
		profile string
		rest    []string
	}{
		{"before the subcommand", []string{"--profile", "work", "nodes", "list"}, "work", []string{"nodes", "list"}},
		{"after the subcommand", []string{"nodes", "list", "--profile=work"}, "work", []string{"nodes", "list"}},
		{"absent", []string{"nodes", "list"}, "", []string{"nodes", "list"}},
		{
			"not past the separator",
			[]string{"exec", "web-01", "--", "tolato", "--profile", "work"},
			"",
			[]string{"exec", "web-01", "--", "tolato", "--profile", "work"},
		},
		{
			"ours before, theirs after",
			[]string{"--profile", "work", "exec", "web-01", "--", "ls", "--profile=x"},
			"work",
			[]string{"exec", "web-01", "--", "ls", "--profile=x"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile, rest, err := extractProfileFlag(tc.in)
			if err != nil {
				t.Fatalf("extractProfileFlag: %v", err)
			}
			if profile != tc.profile {
				t.Errorf("profile = %q, want %q", profile, tc.profile)
			}
			if !reflect.DeepEqual(rest, tc.rest) {
				t.Errorf("rest = %q, want %q", rest, tc.rest)
			}
		})
	}

	if _, _, err := extractProfileFlag([]string{"nodes", "--profile"}); err == nil {
		t.Error("a --profile with no name should error")
	}
	if _, _, err := extractProfileFlag([]string{"--profile", "../etc", "nodes"}); err == nil {
		t.Error("an invalid profile name should be rejected before it reaches a file path")
	}
}

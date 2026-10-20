package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// The node runs the command with `sh -c`, so the real test of remoteCommand is
// whether sh splits it back into the argv we started from.
func TestRemoteCommandSurvivesTheRemoteShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX sh")
	}
	cases := [][]string{
		{"grep", "foo bar", "/etc/hosts"},
		{"bash", "-c", "echo one\necho two"},
		{"echo", `it's "quoted"`},
		{"echo", "$HOME", "`id`", "$(id)"},
		{"echo", "*", "~", "a;b", "a|b", "a&&b", "#x"},
		{"echo", "", "trailing space ", "\ttab"},
		{"echo", "中文", "FOO=bar", "--json"},
		{"echo", "'", "''", `'\''`},
	}
	for _, words := range cases {
		// printf repeats its format per argument, so each word comes back on
		// its own line between markers and an empty one is still visible.
		argv := append([]string{"printf", `<%s>\n`}, words[1:]...)
		out, err := exec.Command("sh", "-c", remoteCommand(argv, false)).Output()
		if err != nil {
			t.Fatalf("%q: sh: %v", words, err)
		}
		var want strings.Builder
		for _, w := range words[1:] {
			want.WriteString("<" + w + ">\n")
		}
		if string(out) != want.String() {
			t.Errorf("%q\nsent: %s\ngot:  %q\nwant: %q", words, remoteCommand(argv, false), out, want.String())
		}
	}
}

// One argument is the caller's own script: pipes, globs and newlines in it are
// meant for the remote shell and must not be quoted away.
func TestRemoteCommandSendsASingleScriptVerbatim(t *testing.T) {
	script := "grep 'foo bar' /etc/hosts | head -1\nls /var/log/*.log"
	if got := remoteCommand([]string{script}, false); got != script {
		t.Errorf("got %q, want %q", got, script)
	}
}

// Plain words stay bare so the audit log reads like the command that was typed.
func TestRemoteCommandLeavesPlainWordsBare(t *testing.T) {
	got := remoteCommand([]string{"systemctl", "status", "nginx.service", "--no-pager", "-n", "50"}, false)
	if want := "systemctl status nginx.service --no-pager -n 50"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// cmd.exe has no single quotes, so POSIX quoting would only break things there.
func TestRemoteCommandPlainJoinOnWindows(t *testing.T) {
	got := remoteCommand([]string{"dir", "C:\\Program Files"}, true)
	if want := "dir C:\\Program Files"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if !isWindows(Node{OS: "Microsoft Windows Server 2022 Datacenter"}) || isWindows(Node{OS: "ubuntu 22.04"}) {
		t.Error("isWindows misreads the platform name")
	}
}

// --meta and --redact-output are easy to swallow into the remote command or
// drop on the floor, so check they reach the request body from either side of
// the node argument.
func TestExecSendsAuditOptions(t *testing.T) {
	var got map[string]string
	var command string
	var redacted bool
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Node{{ID: "n1", Name: "box", Status: "online"}})
	})
	mux.HandleFunc("POST /api/v1/nodes/n1/execute", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Command      string
			Metadata     map[string]string
			RedactOutput bool `json:"redact_output"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		got, command, redacted = req.Metadata, req.Command, req.RedactOutput
		_ = json.NewEncoder(w).Encode(ExecResult{})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := newClient(Config{URL: srv.URL, APIKey: "k"})

	if err := runExec(c, []string{"--meta", "ticket=T-42", "box", "--meta", "note=a=b", "--", "uptime"}); err != nil {
		t.Fatal(err)
	}
	if command != "uptime" || len(got) != 2 || got["ticket"] != "T-42" || got["note"] != "a=b" {
		t.Errorf("sent command %q, metadata %v", command, got)
	}
	if redacted {
		t.Error("asked for redaction without --redact-output")
	}

	if err := runExec(c, []string{"box", "--redact-output", "--", "env"}); err != nil {
		t.Fatal(err)
	}
	if !redacted || got != nil {
		t.Errorf("--redact-output: sent redact_output=%v, metadata %v", redacted, got)
	}

	if err := runExec(c, []string{"--meta", "novalue", "box", "--", "uptime"}); err == nil {
		t.Error("accepted --meta without =")
	}
}

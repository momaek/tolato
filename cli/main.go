// Command tolato is a small client for a Tolato server's /api/v1 endpoints.
//
// It is deliberately a thin wrapper: everything it can do is something the
// REST API already exposes, and the API key decides what that is. In
// particular there is no way to edit a node's attributes from here — that
// capability exists only in the web UI, and the v1 API has no endpoint for it.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// version is injected at build time via -ldflags "-X main.version=<tag>".
var version = "dev"

const usage = `tolato — command-line client for a Tolato server

Usage:
  tolato auth login [--url <server>] [--profile <name>] [--no-browser]
  tolato auth status
  tolato auth logout
  tolato profile list
  tolato profile use <name>
  tolato profile remove <name> [--force]
  tolato nodes list [--status online|offline] [--json]
  tolato nodes get <node> [--json]
  tolato exec <node> -- <command>...
  tolato version

Node arguments accept an id, an alias, or a hostname.

Configuration ("tolato auth login" writes this for you):
  ~/.config/tolato/config.yaml     one entry per server under "profiles",
                                   with "current" naming the active one
                                   ($TOLATO_CONFIG or $XDG_CONFIG_HOME override
                                   the location; same path on every platform)
  TOLATO_PROFILE                   pick a profile for this shell
  TOLATO_URL and TOLATO_API_KEY    target a server with no profile at all
                                   (ignored when --profile is given)

Flags:
  --profile   act on a named profile without changing the current one
  --json      print the raw JSON response instead of a table
  --timeout   seconds to allow a command to run (default 60)
  --confirm   proceed with a command the server flags as sensitive
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, errUsage) {
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "tolato: %v\n", err)
		os.Exit(1)
	}
}

var errUsage = errors.New("usage")

func run(args []string) error {
	if len(args) == 0 {
		return errUsage
	}

	switch args[0] {
	case "version", "-v", "--version":
		fmt.Println(version)
		return nil
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	}

	// --profile applies to every command, so it is taken out here rather than
	// declared on each subcommand's FlagSet. That also lets it appear after the
	// subcommand, where people naturally type it.
	profile, args, err := extractProfileFlag(args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return errUsage
	}

	switch args[0] {
	// Dispatched before loadConfig: `auth login` is what produces a config, so
	// requiring one first would make it impossible to run when it is needed,
	// and `profile` edits the file rather than talking to a server.
	case "auth":
		return runAuth(profile, args[1:])
	case "profile":
		return runProfile(profile, args[1:])
	}

	cfg, err := loadConfig(profile)
	if err != nil {
		return err
	}
	client := newClient(cfg)

	switch args[0] {
	case "nodes":
		return runNodes(client, args[1:])
	case "exec":
		return runExec(client, args[1:])
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
	}
}

// extractProfileFlag pulls --profile out of the argument list and returns the
// rest untouched.
//
// Scanning stops at the first bare `--`: everything past it belongs to the
// remote command in `tolato exec node -- ...`, and stealing a --profile out of
// there would rewrite what runs on the node.
func extractProfileFlag(args []string) (string, []string, error) {
	profile := ""
	rest := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			return profile, append(rest, args[i:]...), nil
		case arg == "--profile" || arg == "-profile":
			if i+1 >= len(args) || args[i+1] == "--" {
				return "", nil, fmt.Errorf("--profile needs a name")
			}
			i++
			profile = args[i]
		case strings.HasPrefix(arg, "--profile="), strings.HasPrefix(arg, "-profile="):
			profile = arg[strings.Index(arg, "=")+1:]
		default:
			rest = append(rest, arg)
		}
	}

	if profile != "" {
		if err := validateProfileName(profile); err != nil {
			return "", nil, err
		}
	}
	return profile, rest, nil
}

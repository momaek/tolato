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
)

// version is injected at build time via -ldflags "-X main.version=<tag>".
var version = "dev"

const usage = `tolato — command-line client for a Tolato server

Usage:
  tolato nodes list [--status online|offline] [--json]
  tolato nodes get <node> [--json]
  tolato exec <node> -- <command>...
  tolato version

Node arguments accept an id, an alias, or a hostname.

Configuration (checked in this order):
  TOLATO_URL and TOLATO_API_KEY environment variables
  ~/.config/tolato/config.yaml     with keys: url, api_key

Flags:
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

	cfg, err := loadConfig()
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

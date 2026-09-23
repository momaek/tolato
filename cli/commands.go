package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

func runNodes(c *client, args []string) error {
	if len(args) == 0 {
		return errUsage
	}
	switch args[0] {
	case "list", "ls":
		return runNodesList(c, args[1:])
	case "get", "show":
		return runNodesGet(c, args[1:])
	default:
		return fmt.Errorf("unknown subcommand %q for `tolato nodes`", args[0])
	}
}

func runNodesList(c *client, args []string) error {
	fs := flag.NewFlagSet("nodes list", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print raw JSON")
	status := fs.String("status", "", "filter by status: online or offline")
	if err := fs.Parse(args); err != nil {
		return err
	}

	nodes, err := c.ListNodes(context.Background())
	if err != nil {
		return err
	}

	if *status != "" {
		filtered := nodes[:0]
		for _, n := range nodes {
			if strings.EqualFold(n.Status, *status) {
				filtered = append(filtered, n)
			}
		}
		nodes = filtered
	}

	if *asJSON {
		return printJSON(nodes)
	}
	if len(nodes) == 0 {
		fmt.Println("No nodes.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tIP\tOS\tID")
	for _, n := range nodes {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", n.DisplayName(), n.Status, n.IP, n.OS, n.ID)
	}
	return w.Flush()
}

func runNodesGet(c *client, args []string) error {
	fs := flag.NewFlagSet("nodes get", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print raw JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errUsage
	}

	ctx := context.Background()
	ref, err := c.resolveNode(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	n, err := c.GetNode(ctx, ref.ID)
	if err != nil {
		return err
	}

	if *asJSON {
		return printJSON(n)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Name:\t%s\n", n.DisplayName())
	fmt.Fprintf(w, "ID:\t%s\n", n.ID)
	fmt.Fprintf(w, "Status:\t%s\n", n.Status)
	fmt.Fprintf(w, "IP:\t%s\n", n.IP)
	fmt.Fprintf(w, "OS:\t%s %s\n", n.OS, n.Kernel)
	fmt.Fprintf(w, "Agent:\t%s\n", n.AgentVersion)
	fmt.Fprintf(w, "Hardware:\t%d cores, %d MB RAM, %d GB disk\n", n.CPUCores, n.MemoryTotalMB, n.DiskTotalGB)
	if n.CPU != nil && n.Memory != nil && n.Disk != nil {
		fmt.Fprintf(w, "Usage:\tcpu %.1f%%, mem %.1f%%, disk %.1f%%\n", *n.CPU, *n.Memory, *n.Disk)
	}
	return w.Flush()
}

// runExec runs one command on one node. Everything after `--` is the remote
// command: our own flags are never read out of it, so `tolato exec web-01 --
// ls --json` runs `ls --json` remotely rather than printing JSON here.
//
// A single argument is a shell script and goes to the node's shell verbatim,
// newlines, quotes and pipes included. Several arguments are an argv: each is
// quoted so the node runs exactly the words the local shell handed us, rather
// than re-splitting them on spaces. See remoteCommand.
func runExec(c *client, args []string) error {
	// Split at the first `--` before parsing. Without this the separator ends
	// up inside the command string and the remote shell chokes on it.
	flagArgs, remote := args, []string(nil)
	sawSeparator := false
	for i, a := range args {
		if a == "--" {
			flagArgs, remote, sawSeparator = args[:i], args[i+1:], true
			break
		}
	}

	fs := flag.NewFlagSet("exec", flag.ContinueOnError)
	timeout := fs.Int("timeout", 60, "seconds to allow the command to run")
	confirm := fs.Bool("confirm", false, "proceed with a command the server flags as sensitive")
	asJSON := fs.Bool("json", false, "print raw JSON")
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) == 0 {
		return errUsage
	}
	nodeRef := rest[0]

	// Go's flag package stops at the first non-flag argument, which is the node
	// reference, so flags written after it are still unparsed. Parse again to
	// pick them up; silently folding `--timeout 300` into the remote command is
	// how this went wrong before, and a swallowed `--confirm` is worse than a
	// noisy one.
	if err := fs.Parse(rest[1:]); err != nil {
		return err
	}
	trailing := fs.Args()

	if !sawSeparator {
		remote = trailing
	} else if len(trailing) > 0 {
		// Args both before and after `--`. Which half is the command is anyone's
		// guess, so refuse rather than run half of it.
		return errUsage
	}

	if strings.TrimSpace(strings.Join(remote, "")) == "" {
		return errUsage
	}

	ctx := context.Background()
	node, err := c.resolveNode(ctx, nodeRef)
	if err != nil {
		return err
	}
	command := remoteCommand(remote, isWindows(node))

	res, err := c.Exec(ctx, node.ID, command, *timeout, *confirm)
	if err != nil {
		var apiErr *apiError
		if errors.As(err, &apiErr) && apiErr.IsSensitive() {
			return fmt.Errorf("%s\nRe-run with --confirm if you are sure", apiErr.Message)
		}
		return err
	}

	if *asJSON {
		return printJSON(res)
	}

	// stdout to stdout and stderr to stderr, so the output composes with pipes
	// the same way a local command would.
	if res.Stdout != "" {
		fmt.Print(res.Stdout)
		if !strings.HasSuffix(res.Stdout, "\n") {
			fmt.Println()
		}
	}
	if res.Stderr != "" {
		fmt.Fprint(os.Stderr, res.Stderr)
		if !strings.HasSuffix(res.Stderr, "\n") {
			fmt.Fprintln(os.Stderr)
		}
	}
	// Mirror the remote exit code so shell `&&` chains behave as expected.
	if res.ExitCode != 0 {
		os.Exit(res.ExitCode)
	}
	return nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// remoteCommand turns the words after `--` into the string the node's shell
// runs.
//
// One word is taken as a whole script and sent untouched, which is how
// multi-line scripts, pipes and redirections get there. More than one word is
// an argv the local shell has already split and unquoted: joining it back with
// bare spaces let the node split it a second time, so `grep 'foo bar' f`
// arrived as `grep foo bar f` and `bash -c "a<newline>b"` ran `b` on its own.
// Quoting each word makes the remote argv match the local one.
//
// cmd.exe has no single-quote syntax, so Windows nodes keep the plain join.
func remoteCommand(words []string, windows bool) string {
	if len(words) == 1 || windows {
		return strings.Join(words, " ")
	}
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = shellQuote(w)
	}
	return strings.Join(quoted, " ")
}

// shellQuote quotes s for a POSIX shell. Words made only of characters no
// shell treats specially are left bare so the command stays readable in the
// audit log; everything else is single-quoted, the one form in which nothing
// but the closing quote is special.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexFunc(s, needsQuoting) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func needsQuoting(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return false
	case strings.ContainsRune("@%+=:,./-_", r):
		return false
	}
	return true
}

// isWindows reports whether the node's shell is cmd.exe. The agent reports its
// platform name, e.g. "Microsoft Windows Server 2022 Datacenter".
func isWindows(n Node) bool {
	return strings.Contains(strings.ToLower(n.OS), "windows")
}

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

// Profiles let one machine hold credentials for several Tolato deployments —
// a company one and a personal one, say — and choose between them per command
// (--profile) or per shell ($TOLATO_PROFILE), without any command silently
// landing on the wrong fleet.

func runProfile(profile string, args []string) error {
	if len(args) == 0 {
		return runProfileList(profile, nil)
	}
	switch args[0] {
	case "list", "ls":
		return runProfileList(profile, args[1:])
	case "use", "switch":
		return runProfileUse(args[1:])
	case "remove", "rm", "delete":
		return runProfileRemove(args[1:])
	default:
		return fmt.Errorf("unknown subcommand %q for `tolato profile`", args[0])
	}
}

func runProfileList(profile string, args []string) error {
	fs := flag.NewFlagSet("profile list", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	file, err := readConfigFile()
	if err != nil {
		return err
	}
	if len(file.Profiles) == 0 {
		fmt.Printf("No profiles in %s.\nRun `tolato auth login --url https://tolato.example.com` to create one.\n", configPath())
		return nil
	}

	// The marker has to show what a command would actually use, not just what
	// the file says is current — an exported $TOLATO_PROFILE outranks it.
	active, resolveErr := file.resolve(profile)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "\tNAME\tSERVER\tKEY")
	for _, name := range file.names() {
		marker := " "
		if name == active {
			marker = "*"
		}
		key := "(not logged in)"
		if p := file.Profiles[name]; p.APIKey != "" {
			key = maskKey(p.APIKey)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", marker, name, file.Profiles[name].URL, key)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	fmt.Printf("\nConfig: %s\n", configPath())
	if resolveErr != nil {
		fmt.Printf("Active: none — %v\n", resolveErr)
	}
	if profile == "" {
		warnEnvOverride()
	}
	return nil
}

func runProfileUse(args []string) error {
	fs := flag.NewFlagSet("profile use", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errUsage
	}
	name := fs.Arg(0)

	file, err := readConfigFile()
	if err != nil {
		return err
	}
	if _, ok := file.Profiles[name]; !ok {
		return fmt.Errorf("no such profile %q%s", name, file.available())
	}

	file.Current = name
	if err := writeConfigFile(file); err != nil {
		return err
	}

	fmt.Printf("Now using %q (%s).\n", name, file.Profiles[name].URL)
	warnEnvOverride()
	return nil
}

func runProfileRemove(args []string) error {
	fs := flag.NewFlagSet("profile remove", flag.ContinueOnError)
	force := fs.Bool("force", false, "remove even if the profile still holds an API key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return errUsage
	}
	name := rest[0]

	// Go's flag package stops at the first non-flag argument, so `--force`
	// written after the name is still unparsed. Parse again to pick it up: a
	// silently swallowed --force would turn a deliberate override back into the
	// refusal it was meant to lift.
	if err := fs.Parse(rest[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errUsage
	}

	file, err := readConfigFile()
	if err != nil {
		return err
	}
	cfg, ok := file.Profiles[name]
	if !ok {
		return fmt.Errorf("no such profile %q%s", name, file.available())
	}

	// Deleting the local copy of a live key leaves it valid on the server with
	// nobody tracking it. `auth logout` revokes first, which is what the user
	// almost always means.
	if cfg.APIKey != "" && !*force {
		return fmt.Errorf("profile %q still holds a key; run `tolato auth logout --profile %s` to revoke it first, or pass --force to drop it locally anyway", name, name)
	}

	delete(file.Profiles, name)
	if file.Current == name {
		file.Current = ""
	}
	if err := writeConfigFile(file); err != nil {
		return err
	}

	fmt.Printf("Removed profile %q from %s\n", name, configPath())
	if cfg.APIKey != "" {
		fmt.Fprintf(os.Stderr, "Its key was not revoked — it still works until you delete it in Settings → API Keys.\n")
	}
	return nil
}

// warnEnvOverride says so when the environment is about to outrank the file.
// Without it, `tolato profile use personal` looks like it worked while every
// command keeps hitting whatever TOLATO_URL points at.
func warnEnvOverride() {
	var set []string
	for _, name := range []string{"TOLATO_PROFILE", "TOLATO_URL", "TOLATO_API_KEY"} {
		if strings.TrimSpace(os.Getenv(name)) != "" {
			set = append(set, name)
		}
	}
	if len(set) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "Note: %s is set in this shell and overrides the file; pass --profile to override it back.\n",
		strings.Join(set, " and "))
}

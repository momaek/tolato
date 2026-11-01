package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"
)

// New integration commands accept the node before flags, like exec and cp.
func runCommands(c *client, args []string) error {
	if len(args) == 0 {
		return errUsage
	}
	fs := flag.NewFlagSet("commands", flag.ContinueOnError)
	since := fs.String("since", "", "exclusive RFC 3339 start time")
	cursor := fs.String("cursor", "", "next_cursor or checkpoint from a previous response")
	size := fs.Int("page-size", 100, "records per page (1–100)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 || *size < 1 || *size > 100 {
		return errUsage
	}
	if *since != "" {
		if _, err := time.Parse(time.RFC3339Nano, *since); err != nil {
			return err
		}
	}
	if *since != "" && *cursor != "" {
		return fmt.Errorf("use since or cursor")
	}
	ctx := context.Background()
	n, err := c.resolveNode(ctx, args[0])
	if err != nil {
		return err
	}
	q := url.Values{"page_size": {fmt.Sprint(*size)}}
	if *since != "" {
		q.Set("since", *since)
	}
	if *cursor != "" {
		q.Set("cursor", *cursor)
	}
	var out json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/nodes/"+url.PathEscape(n.ID)+"/commands?"+q.Encode(), nil, &out); err != nil {
		return err
	}
	return printJSON(out)
}

func runServices(c *client, args []string) error {
	if len(args) < 2 {
		return errUsage
	}
	action, node := args[0], args[1]
	if action != "status" && action != "install" && action != "upgrade" && action != "restart" && action != "stop" {
		return errUsage
	}
	fs := flag.NewFlagSet("services", flag.ContinueOnError)
	binary := fs.String("binary-url", "", "HTTPS URL of a patato-node release binary")
	digest := fs.String("sha256", "", "expected binary sha256")
	config := fs.String("config", "", "path to a local JSON configuration (contains a machine token)")
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errUsage
	}
	if (action == "install" || action == "upgrade") && (*binary == "" || *digest == "") {
		return fmt.Errorf("binary-url and sha256 are required")
	}
	if action != "install" && action != "upgrade" && (*binary != "" || *digest != "" || *config != "") {
		return errUsage
	}
	body := map[string]any{"action": action}
	if *binary != "" {
		body["binary_url"] = *binary
		body["sha256"] = *digest
	}
	if *config != "" {
		raw, err := os.ReadFile(*config)
		if err != nil {
			return err
		}
		var obj map[string]json.RawMessage
		if len(raw) > 65536 || json.Unmarshal(raw, &obj) != nil || obj == nil {
			return fmt.Errorf("config must be a JSON object up to 64 KiB")
		}
		body["config"] = json.RawMessage(raw)
	}
	ctx := context.Background()
	n, err := c.resolveNode(ctx, node)
	if err != nil {
		return err
	}
	method := http.MethodPost
	var request any = body
	if action == "status" {
		method = http.MethodGet
		request = nil
	}
	var out json.RawMessage
	if err := c.do(ctx, method, "/api/v1/nodes/"+url.PathEscape(n.ID)+"/services/patato-node", request, &out); err != nil {
		return err
	}
	return printJSON(out)
}

package model

import "time"

// CommandEvent intentionally excludes output, output hashes and metadata.
// Read-only integrations need an activity feed, not command results or secrets.
type CommandEvent struct {
	ID        uint      `json:"id"`
	Command   string    `json:"command"`
	Source    string    `json:"source"`
	Actor     string    `json:"actor"`
	CreatedAt time.Time `json:"created_at"`
}

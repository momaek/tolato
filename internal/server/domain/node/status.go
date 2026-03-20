package node

import "time"

const (
	StaleAfter   = 10 * time.Second
	OfflineAfter = 30 * time.Second
)

func NormalizeStatus(item Node, now time.Time) string {
	if item.LastSeenAt.IsZero() {
		if item.Status == "online" {
			return "stale"
		}
		return "offline"
	}

	age := now.Sub(item.LastSeenAt)
	switch {
	case age > OfflineAfter:
		return "offline"
	case age > StaleAfter:
		return "stale"
	default:
		return "online"
	}
}

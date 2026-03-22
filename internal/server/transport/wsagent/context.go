package wsagent

import "context"

type contextKey string

const clientIDContextKey contextKey = "wsagent-client-id"

func WithClientID(ctx context.Context, clientID string) context.Context {
	return context.WithValue(ctx, clientIDContextKey, clientID)
}

func ClientIDFromContext(ctx context.Context) (string, bool) {
	clientID, ok := ctx.Value(clientIDContextKey).(string)
	return clientID, ok
}

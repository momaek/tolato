package ginws

import (
	"context"
	"net/http"
)

type contextKey string

const requestContextKey contextKey = "ginws.request"

func WithHTTPRequest(ctx context.Context, req *http.Request) context.Context {
	return context.WithValue(ctx, requestContextKey, req)
}

func HTTPRequestFromContext(ctx context.Context) (*http.Request, bool) {
	req, ok := ctx.Value(requestContextKey).(*http.Request)
	return req, ok && req != nil
}

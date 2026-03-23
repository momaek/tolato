package wsui

import (
	"context"
	"net/http"
	"strings"

	appauth "github.com/momaek/tolato/internal/server/app/auth"
	infraws "github.com/momaek/tolato/internal/server/infra/ws"
	"github.com/momaek/tolato/internal/server/transport/ginws"
)

type TokenAuthenticator struct {
	Auth appauth.Service
}

func (a TokenAuthenticator) AuthenticateUI(ctx context.Context, client infraws.Client) error {
	_ = client
	if a.Auth == nil {
		return appauth.ErrUnauthorized
	}
	req, ok := ginws.HTTPRequestFromContext(ctx)
	if !ok {
		return appauth.ErrUnauthorized
	}
	_, err := a.Auth.AuthenticateToken(ctx, extractAccessToken(req))
	return err
}

func extractAccessToken(req *http.Request) string {
	if req == nil {
		return ""
	}
	if token := bearerValue(req.Header.Get("Authorization")); token != "" {
		return token
	}
	return strings.TrimSpace(req.URL.Query().Get("access_token"))
}

func bearerValue(raw string) string {
	raw = strings.TrimSpace(raw)
	parts := strings.SplitN(raw, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

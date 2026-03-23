package auth

import (
	"context"
	"testing"
	"time"

	"github.com/momaek/tolato/internal/server/infra/store/memory"
)

func TestServiceBootstrapLoginAndAuthenticate(t *testing.T) {
	store := memory.NewStore()
	svc := NewService(Repositories{Settings: store.Settings, AuthSessions: store.AuthSessions}, Config{
		AdminUsername: "admin",
		AdminPassword: "admin-secret",
		AgentToken:    "agent-secret",
	}).(*service)
	svc.now = func() time.Time { return time.Date(2026, 3, 22, 9, 0, 0, 0, time.UTC) }

	if err := svc.BootstrapAdmin(context.Background()); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}

	login, err := svc.Login(context.Background(), "admin", "admin-secret")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if login.UserID != "admin" || login.SessionID == "" || login.Token == "" {
		t.Fatalf("login = %#v, want user/session/token", login)
	}

	principal, err := svc.AuthenticateToken(context.Background(), login.Token)
	if err != nil {
		t.Fatalf("AuthenticateToken() error = %v", err)
	}
	if principal.UserID != "admin" || principal.SessionID != login.SessionID {
		t.Fatalf("principal = %#v, want admin/%s", principal, login.SessionID)
	}

	if err := svc.AuthenticateAgentToken(context.Background(), "agent-secret"); err != nil {
		t.Fatalf("AuthenticateAgentToken() error = %v", err)
	}
}

func TestServiceChangePasswordRevokesSessions(t *testing.T) {
	store := memory.NewStore()
	svc := NewService(Repositories{Settings: store.Settings, AuthSessions: store.AuthSessions}, Config{
		AdminUsername: "admin",
		AdminPassword: "before",
	}).(*service)
	if err := svc.BootstrapAdmin(context.Background()); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}

	first, err := svc.Login(context.Background(), "admin", "before")
	if err != nil {
		t.Fatalf("Login(first) error = %v", err)
	}
	second, err := svc.Login(context.Background(), "admin", "before")
	if err != nil {
		t.Fatalf("Login(second) error = %v", err)
	}

	if err := svc.RevokeOtherSessions(context.Background(), "admin", first.SessionID); err != nil {
		t.Fatalf("RevokeOtherSessions() error = %v", err)
	}
	if _, err := svc.AuthenticateToken(context.Background(), second.Token); err == nil {
		t.Fatal("AuthenticateToken(second) error = nil, want revoked session")
	}

	if err := svc.ChangePassword(context.Background(), "admin", "before", "after"); err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}
	if _, err := svc.AuthenticateToken(context.Background(), first.Token); err == nil {
		t.Fatal("AuthenticateToken(first) error = nil, want all sessions revoked")
	}
	if _, err := svc.Login(context.Background(), "admin", "before"); err == nil {
		t.Fatal("Login(old password) error = nil, want unauthorized")
	}
	if _, err := svc.Login(context.Background(), "admin", "after"); err != nil {
		t.Fatalf("Login(new password) error = %v", err)
	}
}

func TestServiceAuthenticateTokenSurvivesNewServiceInstance(t *testing.T) {
	store := memory.NewStore()
	first := NewService(Repositories{Settings: store.Settings, AuthSessions: store.AuthSessions}, Config{
		AdminUsername: "admin",
		AdminPassword: "admin-secret",
	}).(*service)
	if err := first.BootstrapAdmin(context.Background()); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}

	login, err := first.Login(context.Background(), "admin", "admin-secret")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	second := NewService(Repositories{Settings: store.Settings, AuthSessions: store.AuthSessions}, Config{
		AdminUsername: "admin",
		AdminPassword: "admin-secret",
	}).(*service)
	principal, err := second.AuthenticateToken(context.Background(), login.Token)
	if err != nil {
		t.Fatalf("AuthenticateToken() after restart error = %v", err)
	}
	if principal.UserID != "admin" || principal.SessionID != login.SessionID {
		t.Fatalf("principal after restart = %#v, want admin/%s", principal, login.SessionID)
	}
}

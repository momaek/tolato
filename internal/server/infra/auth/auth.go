package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/momaek/tolato/internal/shared/config"
	"github.com/momaek/tolato/internal/shared/types"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID           string
	Name         string
	Username     string
	Password     string
	PasswordHash string
	Role         string
}

type SessionStore interface {
	Store(ctx context.Context, token string, user types.CurrentUser, expiresAt time.Time) error
	Load(ctx context.Context, token string) (types.CurrentUser, error)
}

type Service struct {
	users      map[string]User
	sessions   SessionStore
	sessionTTL time.Duration
}

func NewService(cfg config.ServerConfig, sessions SessionStore) Service {
	users := make(map[string]User)
	for _, item := range cfg.Auth.Users {
		username := strings.TrimSpace(item.Username)
		if username == "" {
			continue
		}
		role := normalizeRole(item.Role)
		users[username] = User{
			ID:           defaultString(item.ID, "u_"+username),
			Name:         defaultString(item.Name, username),
			Username:     username,
			Password:     item.Password,
			PasswordHash: item.PasswordHash,
			Role:         role,
		}
	}
	if len(users) == 0 && cfg.Auth.AdminUsername != "" && cfg.Auth.AdminPassword != "" {
		users[cfg.Auth.AdminUsername] = User{
			ID:       "u_admin",
			Name:     "admin",
			Username: cfg.Auth.AdminUsername,
			Password: cfg.Auth.AdminPassword,
			Role:     "admin",
		}
	}

	ttl, err := time.ParseDuration(cfg.Auth.SessionTTL)
	if err != nil || ttl <= 0 {
		ttl = 24 * time.Hour
	}

	if sessions == nil {
		sessions = NewMemorySessionStore()
	}

	return Service{
		users:      users,
		sessions:   sessions,
		sessionTTL: ttl,
	}
}

func (s *Service) Login(ctx context.Context, username, password string) (types.LoginResponse, error) {
	user, ok := s.users[strings.TrimSpace(username)]
	if !ok || !matchesPassword(user, password) {
		return types.LoginResponse{}, errors.New("invalid credentials")
	}

	current := types.CurrentUser{
		ID:       user.ID,
		Name:     user.Name,
		Username: user.Username,
		Role:     user.Role,
	}
	token, err := newSessionToken()
	if err != nil {
		return types.LoginResponse{}, err
	}

	if err := s.sessions.Store(ctx, token, current, time.Now().UTC().Add(s.sessionTTL)); err != nil {
		return types.LoginResponse{}, err
	}

	return types.LoginResponse{
		User:  current,
		Token: token,
	}, nil
}

func (s *Service) AuthenticateToken(ctx context.Context, token string) (types.CurrentUser, error) {
	return s.sessions.Load(ctx, strings.TrimSpace(token))
}

func (s *Service) AuthenticateRequest(r *http.Request) (types.CurrentUser, error) {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	if token == "" {
		return types.CurrentUser{}, errors.New("missing authorization token")
	}
	return s.AuthenticateToken(r.Context(), token)
}

func newSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func bearerToken(header string) string {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(header)), "bearer ") {
		return ""
	}
	return strings.TrimSpace(header[7:])
}

func matchesPassword(user User, password string) bool {
	switch {
	case strings.TrimSpace(user.PasswordHash) != "":
		return bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) == nil
	case user.Password != "":
		return user.Password == password
	default:
		return false
	}
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin":
		return "admin"
	default:
		return "operator"
	}
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

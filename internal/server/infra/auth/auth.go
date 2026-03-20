package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/momaek/tolato/internal/shared/types"
)

type Service struct {
	adminUsername string
	adminPassword string
	mu            sync.RWMutex
	sessions      map[string]types.CurrentUser
}

func NewService(username, password string) Service {
	return Service{
		adminUsername: username,
		adminPassword: password,
		sessions:      make(map[string]types.CurrentUser),
	}
}

func (s *Service) Login(username, password string) (types.LoginResponse, error) {
	if username != s.adminUsername || password != s.adminPassword {
		return types.LoginResponse{}, errors.New("invalid credentials")
	}

	user := s.CurrentUser()
	token, err := newSessionToken()
	if err != nil {
		return types.LoginResponse{}, err
	}

	s.mu.Lock()
	s.sessions[token] = user
	s.mu.Unlock()

	return types.LoginResponse{
		User:  user,
		Token: token,
	}, nil
}

func (s *Service) CurrentUser() types.CurrentUser {
	return types.CurrentUser{
		ID:   "u_admin",
		Role: "admin",
	}
}

func (s *Service) AuthenticateToken(token string) (types.CurrentUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.sessions[strings.TrimSpace(token)]
	if !ok {
		return types.CurrentUser{}, errors.New("unauthorized")
	}
	return user, nil
}

func (s *Service) AuthenticateRequest(r *http.Request) (types.CurrentUser, error) {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	if token == "" {
		return types.CurrentUser{}, errors.New("missing authorization token")
	}
	return s.AuthenticateToken(token)
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

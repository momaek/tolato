package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
)

type Service interface {
	BootstrapAdmin(ctx context.Context) error
	Login(ctx context.Context, username string, password string) (LoginResult, error)
	AuthenticateToken(ctx context.Context, token string) (Principal, error)
	AuthenticateAgentToken(ctx context.Context, token string) error
	ChangePassword(ctx context.Context, userID string, currentPassword string, newPassword string) error
	RevokeOtherSessions(ctx context.Context, userID string, currentSessionID string) error
}

type Repositories struct {
	Settings     domain.SettingsRepository
	AuthSessions domain.AuthSessionRepository
}

type Config struct {
	AdminUsername string
	AdminPassword string
	AgentToken    string
}

type Principal struct {
	UserID    string
	SessionID string
}

type LoginResult struct {
	UserID    string `json:"userId"`
	SessionID string `json:"sessionId"`
	Token     string `json:"token"`
}

type service struct {
	repos Repositories
	cfg   Config
	now   func() time.Time

	mu             sync.RWMutex
	sessionByToken map[string]domain.AuthSession
}

type authCredentials struct {
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash"`
	UpdatedAt    string `json:"updatedAt"`
}

func NewService(repos Repositories, cfg Config) Service {
	return &service{
		repos:          repos,
		cfg:            cfg,
		now:            time.Now,
		sessionByToken: make(map[string]domain.AuthSession),
	}
}

func (s *service) BootstrapAdmin(ctx context.Context) error {
	username := normalizeUsername(s.cfg.AdminUsername)
	password := strings.TrimSpace(s.cfg.AdminPassword)
	if username == "" || password == "" {
		return domain.ErrInvalidArgument
	}

	record, err := s.repos.Settings.Get(ctx, username, domain.SettingKeyAuthCredentials)
	switch {
	case err == nil && len(record.Value) > 0:
		return nil
	case err != nil && err != domain.ErrNotFound:
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	credentials := authCredentials{
		Username:     username,
		PasswordHash: string(hash),
		UpdatedAt:    s.now().UTC().Format(time.RFC3339),
	}
	if err := s.putCredentials(ctx, credentials); err != nil {
		return err
	}
	return s.ensureAccountSecurity(ctx, username)
}

func (s *service) Login(ctx context.Context, username string, password string) (LoginResult, error) {
	credentials, err := s.loadCredentials(ctx, normalizeUsername(username))
	if err != nil {
		return LoginResult{}, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(credentials.PasswordHash), []byte(password)); err != nil {
		return LoginResult{}, ErrUnauthorized
	}

	session := domain.AuthSession{
		UserID:     credentials.Username,
		SessionID:  randomToken("sess"),
		Token:      randomToken("tok"),
		CreatedAt:  s.now().UTC(),
		LastSeenAt: s.now().UTC(),
	}

	if s.repos.AuthSessions == nil {
		return LoginResult{}, domain.ErrUnsupportedConfig
	}
	if err := s.repos.AuthSessions.Put(ctx, session); err != nil {
		return LoginResult{}, err
	}

	s.mu.Lock()
	s.sessionByToken[session.Token] = session
	s.mu.Unlock()

	if err := s.touchLastLogin(ctx, credentials.Username); err != nil {
		return LoginResult{}, err
	}

	return LoginResult{
		UserID:    session.UserID,
		SessionID: session.SessionID,
		Token:     session.Token,
	}, nil
}

func (s *service) AuthenticateToken(ctx context.Context, token string) (Principal, error) {
	_ = ctx
	token = strings.TrimSpace(token)
	if token == "" {
		return Principal{}, ErrUnauthorized
	}

	s.mu.Lock()
	session, ok := s.sessionByToken[token]
	s.mu.Unlock()
	if !ok {
		if s.repos.AuthSessions == nil {
			return Principal{}, ErrUnauthorized
		}
		stored, err := s.repos.AuthSessions.GetByToken(ctx, token)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return Principal{}, ErrUnauthorized
			}
			return Principal{}, err
		}
		session = stored
		s.mu.Lock()
		s.sessionByToken[token] = session
		s.mu.Unlock()
	}
	session.LastSeenAt = s.now().UTC()
	if s.repos.AuthSessions != nil {
		if err := s.repos.AuthSessions.Touch(ctx, token, session.LastSeenAt); err != nil && !errors.Is(err, domain.ErrNotFound) {
			return Principal{}, err
		}
	}
	s.mu.Lock()
	s.sessionByToken[token] = session
	s.mu.Unlock()
	return Principal{
		UserID:    session.UserID,
		SessionID: session.SessionID,
	}, nil
}

func (s *service) AuthenticateAgentToken(ctx context.Context, token string) error {
	_ = ctx
	expected := strings.TrimSpace(s.cfg.AgentToken)
	if expected == "" {
		expected = strings.TrimSpace(s.cfg.AdminPassword)
	}
	if expected == "" || subtleTrimEqual(token, expected) == false {
		return ErrUnauthorized
	}
	return nil
}

func (s *service) ChangePassword(ctx context.Context, userID string, currentPassword string, newPassword string) error {
	credentials, err := s.loadCredentials(ctx, normalizeUsername(userID))
	if err != nil {
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(credentials.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrUnauthorized
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	credentials.PasswordHash = string(hash)
	credentials.UpdatedAt = s.now().UTC().Format(time.RFC3339)
	if err := s.putCredentials(ctx, credentials); err != nil {
		return err
	}

	if s.repos.AuthSessions != nil {
		if err := s.repos.AuthSessions.DeleteByUser(ctx, credentials.Username); err != nil {
			return err
		}
	}
	s.clearCachedSessionsByUser(credentials.Username)
	return nil
}

func (s *service) RevokeOtherSessions(ctx context.Context, userID string, currentSessionID string) error {
	_ = ctx
	userID = normalizeUsername(userID)
	currentSessionID = strings.TrimSpace(currentSessionID)
	if userID == "" || currentSessionID == "" {
		return domain.ErrInvalidArgument
	}

	if s.repos.AuthSessions != nil {
		if err := s.repos.AuthSessions.DeleteByUserExceptSession(ctx, userID, currentSessionID); err != nil {
			return err
		}
	}
	s.clearCachedSessionsByUserExcept(userID, currentSessionID)
	return nil
}

func (s *service) clearCachedSessionsByUser(userID string) {
	s.mu.Lock()
	for token, session := range s.sessionByToken {
		if session.UserID == userID {
			delete(s.sessionByToken, token)
		}
	}
	s.mu.Unlock()
}

func (s *service) clearCachedSessionsByUserExcept(userID string, sessionID string) {
	s.mu.Lock()
	for token, session := range s.sessionByToken {
		if session.UserID != userID || session.SessionID == sessionID {
			continue
		}
		delete(s.sessionByToken, token)
	}
	s.mu.Unlock()
}

func (s *service) loadCredentials(ctx context.Context, username string) (authCredentials, error) {
	if username == "" || s.repos.Settings == nil {
		return authCredentials{}, domain.ErrInvalidArgument
	}
	record, err := s.repos.Settings.Get(ctx, username, domain.SettingKeyAuthCredentials)
	if err != nil {
		if err == domain.ErrNotFound {
			return authCredentials{}, ErrUnauthorized
		}
		return authCredentials{}, err
	}
	var credentials authCredentials
	if err := json.Unmarshal(record.Value, &credentials); err != nil {
		return authCredentials{}, err
	}
	if normalizeUsername(credentials.Username) == "" || strings.TrimSpace(credentials.PasswordHash) == "" {
		return authCredentials{}, ErrUnauthorized
	}
	credentials.Username = normalizeUsername(credentials.Username)
	return credentials, nil
}

func (s *service) putCredentials(ctx context.Context, credentials authCredentials) error {
	if s.repos.Settings == nil {
		return domain.ErrUnsupportedConfig
	}
	raw, err := json.Marshal(credentials)
	if err != nil {
		return err
	}
	return s.repos.Settings.Put(ctx, domain.SettingRecord{
		UserID:    credentials.Username,
		Key:       domain.SettingKeyAuthCredentials,
		Value:     raw,
		UpdatedAt: s.now().UTC(),
	})
}

func (s *service) ensureAccountSecurity(ctx context.Context, username string) error {
	if s.repos.Settings == nil {
		return domain.ErrUnsupportedConfig
	}
	record, err := s.repos.Settings.Get(ctx, username, domain.SettingKeyAccountSecurity)
	switch {
	case err == nil && len(record.Value) > 0:
		return nil
	case err != nil && err != domain.ErrNotFound:
		return err
	}

	view := map[string]any{
		"username":           username,
		"lastLoginAt":        s.now().UTC().Format(time.RFC3339),
		"mfaEnabled":         true,
		"auditRetentionDays": 90,
	}
	raw, err := json.Marshal(view)
	if err != nil {
		return err
	}
	return s.repos.Settings.Put(ctx, domain.SettingRecord{
		UserID:    username,
		Key:       domain.SettingKeyAccountSecurity,
		Value:     raw,
		UpdatedAt: s.now().UTC(),
	})
}

func (s *service) touchLastLogin(ctx context.Context, username string) error {
	if s.repos.Settings == nil {
		return domain.ErrUnsupportedConfig
	}
	record, err := s.repos.Settings.Get(ctx, username, domain.SettingKeyAccountSecurity)
	if err != nil && err != domain.ErrNotFound {
		return err
	}

	view := map[string]any{
		"username":           username,
		"lastLoginAt":        s.now().UTC().Format(time.RFC3339),
		"mfaEnabled":         true,
		"auditRetentionDays": 90,
	}
	if err == nil && len(record.Value) > 0 {
		if unmarshalErr := json.Unmarshal(record.Value, &view); unmarshalErr != nil {
			return unmarshalErr
		}
		view["username"] = username
		view["lastLoginAt"] = s.now().UTC().Format(time.RFC3339)
	}

	raw, err := json.Marshal(view)
	if err != nil {
		return err
	}
	return s.repos.Settings.Put(ctx, domain.SettingRecord{
		UserID:    username,
		Key:       domain.SettingKeyAccountSecurity,
		Value:     raw,
		UpdatedAt: s.now().UTC(),
	})
}

func normalizeUsername(userID string) string {
	return strings.TrimSpace(userID)
}

func randomToken(prefix string) string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return prefix
	}
	return prefix + "_" + hex.EncodeToString(buf)
}

func subtleTrimEqual(left string, right string) bool {
	return strings.TrimSpace(left) == strings.TrimSpace(right)
}

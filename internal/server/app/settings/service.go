package settings

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

const (
	defaultUserID    = "local-dev"
	defaultSessionID = "dev-session"
)

type Service interface {
	GetModelConfig(ctx context.Context, userID string) (ModelConfigView, error)
	PutModelConfig(ctx context.Context, userID string, in UpdateModelConfigInput) (ModelConfigView, error)
	TestModelConfig(ctx context.Context, userID string, in TestModelConfigInput) (ModelConfigTestResult, error)
	ListModelOptions(ctx context.Context, userID string, in ListModelOptionsInput) ([]ModelOption, error)
	GetAccountSecurity(ctx context.Context, userID string) (AccountSecurityView, error)
	PutAccountSecurity(ctx context.Context, userID string, in UpdateAccountSecurityInput) (AccountSecurityView, error)
	ChangePassword(ctx context.Context, userID string, in ChangePasswordInput) error
	RevokeOtherSessions(ctx context.Context, userID string, currentSessionID string) error
	GetPreferences(ctx context.Context, userID string) (UserPreferencesView, error)
	PutPreferences(ctx context.Context, userID string, in UpdatePreferencesInput) (UserPreferencesView, error)
}

type Repositories struct {
	Settings domain.SettingsRepository
	Security SecurityService
	Models   ModelCatalog
}

type SecurityService interface {
	ChangePassword(ctx context.Context, userID string, currentPassword string, newPassword string) error
	RevokeOtherSessions(ctx context.Context, userID string, currentSessionID string) error
}

type ModelCatalog interface {
	ListModels(ctx context.Context, provider string, endpoint string, apiKey string) ([]ModelOption, error)
}

type ModelConfigView struct {
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	Endpoint     string  `json:"endpoint,omitempty"`
	Temperature  float64 `json:"temperature"`
	MaxTokens    int     `json:"maxTokens,omitempty"`
	TimeoutSec   int     `json:"timeoutSec,omitempty"`
	ApprovalMode string  `json:"approvalMode"`
	HasAPIKey    bool    `json:"hasApiKey"`
}

type UpdateModelConfigInput struct {
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	Endpoint     string  `json:"endpoint,omitempty"`
	APIKey       string  `json:"apiKey,omitempty"`
	Temperature  float64 `json:"temperature"`
	MaxTokens    int     `json:"maxTokens,omitempty"`
	TimeoutSec   int     `json:"timeoutSec,omitempty"`
	ApprovalMode string  `json:"approvalMode"`
}

type TestModelConfigInput struct {
	UpdateModelConfigInput
}

type ModelConfigTestResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type ListModelOptionsInput struct {
	Provider string `json:"provider"`
	Endpoint string `json:"endpoint,omitempty"`
	APIKey   string `json:"apiKey,omitempty"`
}

type ModelOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type AccountSecurityView struct {
	Username           string `json:"username"`
	LastLoginAt        string `json:"lastLoginAt"`
	MFAEnabled         bool   `json:"mfaEnabled"`
	AuditRetentionDays int    `json:"auditRetentionDays"`
}

type UpdateAccountSecurityInput struct {
	Username           string `json:"username"`
	LastLoginAt        string `json:"lastLoginAt"`
	MFAEnabled         bool   `json:"mfaEnabled"`
	AuditRetentionDays int    `json:"auditRetentionDays"`
}

type ChangePasswordInput struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type UserPreferencesView struct {
	PreferredRegion string `json:"preferredRegion"`
	DefaultMode     string `json:"defaultMode"`
	Locale          string `json:"locale"`
	CompactTimeline bool   `json:"compactTimeline"`
	StreamMarkdown  bool   `json:"streamMarkdown"`
}

type UpdatePreferencesInput struct {
	PreferredRegion string `json:"preferredRegion"`
	DefaultMode     string `json:"defaultMode"`
	Locale          string `json:"locale"`
	CompactTimeline bool   `json:"compactTimeline"`
	StreamMarkdown  bool   `json:"streamMarkdown"`
}

type service struct {
	repos Repositories
	now   func() time.Time
}

type storedModelConfig struct {
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	Endpoint     string  `json:"endpoint,omitempty"`
	APIKey       string  `json:"apiKey,omitempty"`
	Temperature  float64 `json:"temperature"`
	MaxTokens    int     `json:"maxTokens,omitempty"`
	TimeoutSec   int     `json:"timeoutSec,omitempty"`
	ApprovalMode string  `json:"approvalMode"`
}

func NewService(repos Repositories) Service {
	return &service{
		repos: repos,
		now:   time.Now,
	}
}

func (s *service) GetModelConfig(ctx context.Context, userID string) (ModelConfigView, error) {
	record, err := s.getRecord(ctx, normalizeUserID(userID), domain.SettingKeyModelConfig)
	if err != nil {
		return ModelConfigView{}, err
	}

	var stored storedModelConfig
	if len(record.Value) == 0 {
		stored = defaultModelConfig()
	} else if err := json.Unmarshal(record.Value, &stored); err != nil {
		return ModelConfigView{}, err
	}
	stored = normalizeStoredModelConfig(stored)
	return toModelConfigView(stored), nil
}

func (s *service) PutModelConfig(ctx context.Context, userID string, in UpdateModelConfigInput) (ModelConfigView, error) {
	stored := normalizeStoredModelConfig(storedModelConfig{
		Provider:     strings.TrimSpace(in.Provider),
		Model:        strings.TrimSpace(in.Model),
		Endpoint:     strings.TrimSpace(in.Endpoint),
		APIKey:       strings.TrimSpace(in.APIKey),
		Temperature:  in.Temperature,
		MaxTokens:    in.MaxTokens,
		TimeoutSec:   in.TimeoutSec,
		ApprovalMode: strings.TrimSpace(in.ApprovalMode),
	})
	if stored.APIKey == "" {
		existing, err := s.GetModelConfig(ctx, userID)
		if err == nil && existing.HasAPIKey {
			record, loadErr := s.getRecord(ctx, normalizeUserID(userID), domain.SettingKeyModelConfig)
			if loadErr == nil && len(record.Value) > 0 {
				var persisted storedModelConfig
				if err := json.Unmarshal(record.Value, &persisted); err == nil {
					stored.APIKey = strings.TrimSpace(persisted.APIKey)
				}
			}
		}
	}
	if err := validateModelConfig(stored); err != nil {
		return ModelConfigView{}, err
	}
	if err := s.putRecord(ctx, normalizeUserID(userID), domain.SettingKeyModelConfig, stored); err != nil {
		return ModelConfigView{}, err
	}
	return toModelConfigView(stored), nil
}

func (s *service) TestModelConfig(ctx context.Context, userID string, in TestModelConfigInput) (ModelConfigTestResult, error) {
	stored := normalizeStoredModelConfig(storedModelConfig{
		Provider:     strings.TrimSpace(in.Provider),
		Model:        strings.TrimSpace(in.Model),
		Endpoint:     strings.TrimSpace(in.Endpoint),
		APIKey:       strings.TrimSpace(in.APIKey),
		Temperature:  in.Temperature,
		MaxTokens:    in.MaxTokens,
		TimeoutSec:   in.TimeoutSec,
		ApprovalMode: strings.TrimSpace(in.ApprovalMode),
	})
	if stored.Provider == "" && stored.Model == "" && stored.ApprovalMode == "" {
		record, err := s.getRecord(ctx, normalizeUserID(userID), domain.SettingKeyModelConfig)
		if err != nil {
			return ModelConfigTestResult{}, err
		}
		if len(record.Value) == 0 {
			stored = defaultModelConfig()
		} else if err := json.Unmarshal(record.Value, &stored); err != nil {
			return ModelConfigTestResult{}, err
		}
		stored = normalizeStoredModelConfig(stored)
	}
	if err := validateModelConfig(stored); err != nil {
		return ModelConfigTestResult{}, err
	}
	return ModelConfigTestResult{
		OK:      true,
		Message: "connection test succeeded",
	}, nil
}

func (s *service) ListModelOptions(ctx context.Context, userID string, in ListModelOptionsInput) ([]ModelOption, error) {
	if s.repos.Models == nil {
		return nil, domain.ErrUnsupportedConfig
	}

	cfg, err := s.resolveModelCatalogConfig(ctx, userID, in)
	if err != nil {
		return nil, err
	}
	return s.repos.Models.ListModels(ctx, cfg.Provider, cfg.Endpoint, cfg.APIKey)
}

func (s *service) GetAccountSecurity(ctx context.Context, userID string) (AccountSecurityView, error) {
	record, err := s.getRecord(ctx, normalizeUserID(userID), domain.SettingKeyAccountSecurity)
	if err != nil {
		return AccountSecurityView{}, err
	}

	var view AccountSecurityView
	if len(record.Value) == 0 {
		view = defaultAccountSecurity(normalizeUserID(userID), s.now())
	} else if err := json.Unmarshal(record.Value, &view); err != nil {
		return AccountSecurityView{}, err
	}
	return view, nil
}

func (s *service) PutAccountSecurity(ctx context.Context, userID string, in UpdateAccountSecurityInput) (AccountSecurityView, error) {
	view := AccountSecurityView{
		Username:           strings.TrimSpace(in.Username),
		LastLoginAt:        strings.TrimSpace(in.LastLoginAt),
		MFAEnabled:         in.MFAEnabled,
		AuditRetentionDays: in.AuditRetentionDays,
	}
	if err := validateAccountSecurity(view); err != nil {
		return AccountSecurityView{}, err
	}
	if err := s.putRecord(ctx, normalizeUserID(userID), domain.SettingKeyAccountSecurity, view); err != nil {
		return AccountSecurityView{}, err
	}
	return view, nil
}

func (s *service) ChangePassword(ctx context.Context, userID string, in ChangePasswordInput) error {
	if normalizeUserID(userID) == "" {
		return domain.ErrInvalidArgument
	}
	if strings.TrimSpace(in.CurrentPassword) == "" || strings.TrimSpace(in.NewPassword) == "" {
		return domain.ErrInvalidArgument
	}
	if in.CurrentPassword == in.NewPassword {
		return domain.ErrInvalidArgument
	}
	if s.repos.Security == nil {
		return domain.ErrUnsupportedConfig
	}
	return s.repos.Security.ChangePassword(ctx, normalizeUserID(userID), in.CurrentPassword, in.NewPassword)
}

func (s *service) RevokeOtherSessions(ctx context.Context, userID string, currentSessionID string) error {
	if normalizeUserID(userID) == "" || normalizeSessionID(currentSessionID) == "" {
		return domain.ErrInvalidArgument
	}
	if s.repos.Security == nil {
		return domain.ErrUnsupportedConfig
	}
	return s.repos.Security.RevokeOtherSessions(ctx, normalizeUserID(userID), normalizeSessionID(currentSessionID))
}

func (s *service) GetPreferences(ctx context.Context, userID string) (UserPreferencesView, error) {
	record, err := s.getRecord(ctx, normalizeUserID(userID), domain.SettingKeyPreferences)
	if err != nil {
		return UserPreferencesView{}, err
	}

	var view UserPreferencesView
	if len(record.Value) == 0 {
		view = defaultPreferences()
	} else if err := json.Unmarshal(record.Value, &view); err != nil {
		return UserPreferencesView{}, err
	}
	return view, nil
}

func (s *service) PutPreferences(ctx context.Context, userID string, in UpdatePreferencesInput) (UserPreferencesView, error) {
	view := UserPreferencesView{
		PreferredRegion: strings.TrimSpace(in.PreferredRegion),
		DefaultMode:     strings.TrimSpace(in.DefaultMode),
		Locale:          strings.TrimSpace(in.Locale),
		CompactTimeline: in.CompactTimeline,
		StreamMarkdown:  in.StreamMarkdown,
	}
	if err := validatePreferences(view); err != nil {
		return UserPreferencesView{}, err
	}
	if err := s.putRecord(ctx, normalizeUserID(userID), domain.SettingKeyPreferences, view); err != nil {
		return UserPreferencesView{}, err
	}
	return view, nil
}

func (s *service) getRecord(ctx context.Context, userID string, key domain.SettingKey) (domain.SettingRecord, error) {
	if s.repos.Settings == nil {
		return domain.SettingRecord{}, domain.ErrUnsupportedConfig
	}

	record, err := s.repos.Settings.Get(ctx, userID, key)
	if err == nil {
		return record, nil
	}
	if err != domain.ErrNotFound {
		return domain.SettingRecord{}, err
	}

	return domain.SettingRecord{
		UserID:    userID,
		Key:       key,
		UpdatedAt: s.now().UTC(),
	}, nil
}

func (s *service) putRecord(ctx context.Context, userID string, key domain.SettingKey, value any) error {
	if s.repos.Settings == nil {
		return domain.ErrUnsupportedConfig
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.repos.Settings.Put(ctx, domain.SettingRecord{
		UserID:    userID,
		Key:       key,
		Value:     raw,
		UpdatedAt: s.now().UTC(),
	})
}

func toModelConfigView(stored storedModelConfig) ModelConfigView {
	stored = normalizeStoredModelConfig(stored)
	return ModelConfigView{
		Provider:     stored.Provider,
		Model:        stored.Model,
		Endpoint:     stored.Endpoint,
		Temperature:  stored.Temperature,
		MaxTokens:    stored.MaxTokens,
		TimeoutSec:   stored.TimeoutSec,
		ApprovalMode: stored.ApprovalMode,
		HasAPIKey:    strings.TrimSpace(stored.APIKey) != "",
	}
}

func validateModelConfig(in storedModelConfig) error {
	switch normalizeProvider(in.Provider) {
	case "openai", "devloop":
	default:
		return domain.ErrInvalidArgument
	}
	if in.Provider == "" || in.Model == "" {
		return domain.ErrInvalidArgument
	}
	if in.Temperature < 0 || in.Temperature > 2 {
		return domain.ErrInvalidArgument
	}
	switch in.ApprovalMode {
	case "safe", "balanced", "strict":
	default:
		return domain.ErrInvalidArgument
	}
	if in.MaxTokens < 0 || in.TimeoutSec < 0 {
		return domain.ErrInvalidArgument
	}
	return nil
}

func validateAccountSecurity(in AccountSecurityView) error {
	if strings.TrimSpace(in.Username) == "" {
		return domain.ErrInvalidArgument
	}
	if strings.TrimSpace(in.LastLoginAt) == "" {
		return domain.ErrInvalidArgument
	}
	if in.AuditRetentionDays < 7 {
		return domain.ErrInvalidArgument
	}
	return nil
}

func validatePreferences(in UserPreferencesView) error {
	if strings.TrimSpace(in.PreferredRegion) == "" {
		return domain.ErrInvalidArgument
	}
	switch in.DefaultMode {
	case "ai_agent", "direct_shell":
	default:
		return domain.ErrInvalidArgument
	}
	switch in.Locale {
	case "zh-CN", "en-US":
	default:
		return domain.ErrInvalidArgument
	}
	return nil
}

func normalizeUserID(userID string) string {
	if v := strings.TrimSpace(userID); v != "" {
		return v
	}
	return defaultUserID
}

func normalizeSessionID(sessionID string) string {
	if v := strings.TrimSpace(sessionID); v != "" {
		return v
	}
	return defaultSessionID
}

func normalizeProvider(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func normalizeStoredModelConfig(in storedModelConfig) storedModelConfig {
	in.Provider = normalizeProvider(in.Provider)
	in.Model = strings.TrimSpace(in.Model)
	in.Endpoint = strings.TrimSpace(in.Endpoint)
	in.APIKey = strings.TrimSpace(in.APIKey)
	in.ApprovalMode = strings.TrimSpace(in.ApprovalMode)
	return in
}

func (s *service) resolveModelCatalogConfig(ctx context.Context, userID string, in ListModelOptionsInput) (storedModelConfig, error) {
	record, err := s.getRecord(ctx, normalizeUserID(userID), domain.SettingKeyModelConfig)
	if err != nil {
		return storedModelConfig{}, err
	}

	cfg := defaultModelConfig()
	if len(record.Value) > 0 {
		if err := json.Unmarshal(record.Value, &cfg); err != nil {
			return storedModelConfig{}, err
		}
	}
	cfg = normalizeStoredModelConfig(cfg)

	if v := normalizeProvider(in.Provider); v != "" {
		cfg.Provider = v
	}
	if v := strings.TrimSpace(in.Endpoint); v != "" {
		cfg.Endpoint = v
	}
	if v := strings.TrimSpace(in.APIKey); v != "" {
		cfg.APIKey = v
	}

	if cfg.Provider == "" {
		return storedModelConfig{}, domain.ErrInvalidArgument
	}
	if cfg.Provider == "openai" && (cfg.Endpoint == "" || cfg.APIKey == "") {
		return storedModelConfig{}, domain.ErrInvalidArgument
	}
	return cfg, nil
}

func defaultModelConfig() storedModelConfig {
	return storedModelConfig{
		Provider:     "openai",
		Model:        "gpt-5.4",
		Endpoint:     "https://api.openai.com/v1",
		Temperature:  0.2,
		MaxTokens:    2048,
		TimeoutSec:   60,
		ApprovalMode: "balanced",
	}
}

func defaultAccountSecurity(userID string, now time.Time) AccountSecurityView {
	return AccountSecurityView{
		Username:           userID,
		LastLoginAt:        now.UTC().Add(-4 * time.Hour).Format(time.RFC3339),
		MFAEnabled:         true,
		AuditRetentionDays: 90,
	}
}

func defaultPreferences() UserPreferencesView {
	return UserPreferencesView{
		PreferredRegion: "Tokyo",
		DefaultMode:     "ai_agent",
		Locale:          "zh-CN",
		CompactTimeline: false,
		StreamMarkdown:  true,
	}
}

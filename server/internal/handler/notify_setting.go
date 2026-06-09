package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/notify"
	"github.com/momaek/tolato/server/internal/store"
)

// notifyConfigKey is the single settings key holding the whole NotifySettings
// JSON blob (token_sources/channels are variable-length, so per-field keys don't
// fit the existing group pattern as cleanly).
const notifyConfigKey = "notify.config"

// isSensitiveParam reports whether a template param key holds a credential that
// should be masked in GET responses (corpid/agentid/chat_id/url stay visible).
func isSensitiveParam(key string) bool {
	k := strings.ToLower(key)
	return strings.Contains(k, "secret") ||
		strings.Contains(k, "token") ||
		strings.Contains(k, "password") ||
		strings.Contains(k, "key")
}

// loadNotifySettings reads the stored config, falling back to defaults.
func loadNotifySettings() (model.NotifySettings, error) {
	s := model.NotifySettings{
		OfflineThresholdSeconds: 90,
		RecoverNotify:           true,
		StartupGraceSeconds:     90,
	}
	rec, err := store.GetSetting(notifyConfigKey)
	if err != nil || rec == nil {
		// Missing key is not an error — return defaults.
		return s, nil
	}
	if err := unmarshalSetting(rec.Value, &s); err != nil {
		return s, err
	}
	return s, nil
}

// maskParams masks sensitive param values in place across token sources and
// channels, so secrets aren't echoed back to the browser.
func maskParams(s *model.NotifySettings) {
	mask := func(params map[string]string) {
		for k, v := range params {
			if v != "" && isSensitiveParam(k) {
				params[k] = maskAPIKey(v)
			}
		}
	}
	for i := range s.TokenSources {
		mask(s.TokenSources[i].Params)
	}
	for i := range s.Channels {
		mask(s.Channels[i].Params)
	}
}

// restoreSecrets replaces any masked sensitive param (one the client echoed back
// unchanged, still containing "****") with the stored value, so saving the form
// without re-typing credentials keeps them intact.
func restoreSecrets(next *model.NotifySettings, prev model.NotifySettings) {
	prevSourceParams := map[string]map[string]string{}
	for _, ts := range prev.TokenSources {
		prevSourceParams[ts.Name] = ts.Params
	}
	prevChannelParams := map[string]map[string]string{}
	for _, ch := range prev.Channels {
		prevChannelParams[ch.Name] = ch.Params
	}
	restore := func(params, old map[string]string) {
		if old == nil {
			return
		}
		for k, v := range params {
			if isSensitiveParam(k) && strings.Contains(v, "****") {
				if ov, ok := old[k]; ok {
					params[k] = ov
				}
			}
		}
	}
	for i := range next.TokenSources {
		restore(next.TokenSources[i].Params, prevSourceParams[next.TokenSources[i].Name])
	}
	for i := range next.Channels {
		restore(next.Channels[i].Params, prevChannelParams[next.Channels[i].Name])
	}
}

func GetNotifySettings(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		s, err := loadNotifySettings()
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to read settings"})
			return
		}
		maskParams(&s)
		c.JSON(http.StatusOK, s)
	}
}

func PutNotifySettings(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.NotifySettings
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "bad_request", Message: "invalid request body"})
			return
		}

		prev, _ := loadNotifySettings()
		restoreSecrets(&req, prev)

		if err := store.SetSetting(notifyConfigKey, marshalSetting(req)); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to save settings"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "updated"})
	}
}

// notifyPreset describes a built-in preset for the UI: which params the user
// must fill and an optional token-source template for two-step platforms.
type notifyPreset struct {
	Preset       string                   `json:"preset"`
	Label        string                   `json:"label"`
	Params       []string                 `json:"params"`                  // channel params the user fills
	TokenSource  *model.NotifyTokenSource `json:"token_source,omitempty"`  // prefilled exchange step (params blank)
	SourceParams []string                 `json:"source_params,omitempty"` // params the token source needs
}

// GetNotifyPresets returns the built-in channel presets so the UI can render a
// "pick platform → fill key fields" form.
func GetNotifyPresets(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		presets := []notifyPreset{
			{Preset: "telegram", Label: "Telegram", Params: []string{"bot_token", "chat_id"}},
			{Preset: "discord", Label: "Discord", Params: []string{"url"}},
			{Preset: "slack", Label: "Slack", Params: []string{"url"}},
			{Preset: "wecom", Label: "企业微信", Params: []string{"agentid"}, SourceParams: []string{"corpid", "secret"}},
			{Preset: "feishu", Label: "飞书 / Lark", Params: []string{"receive_id"}, SourceParams: []string{"app_id", "app_secret"}},
			{Preset: "custom", Label: "自定义 Webhook"},
		}
		for i := range presets {
			if ts, ok := notify.PresetTokenSource(presets[i].Preset); ok {
				presets[i].TokenSource = &ts
			}
		}
		c.JSON(http.StatusOK, gin.H{"presets": presets})
	}
}

// TestNotifyChannel sends a sample offline event through a saved channel.
// Body: {"name": "<channel name>"}. Uses the saved (unmasked) config, so the
// client should save before testing.
func TestNotifyChannel(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "bad_request", Message: "channel name required"})
			return
		}
		if deps.Notifier == nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "notifier not available"})
			return
		}
		if err := deps.Notifier.Test(req.Name); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "test_failed", Message: err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "sent"})
	}
}

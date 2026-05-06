package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

// helper: read settings group into a map
func readSettingsGroup(prefix string) (map[string]string, error) {
	settings, err := store.GetSettingsGroup(prefix)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(settings))
	for _, s := range settings {
		// Strip prefix: "llm.api_key" -> "api_key"
		key := strings.TrimPrefix(s.Key, prefix+".")
		result[key] = s.Value
	}
	return result, nil
}

// helper: unmarshal JSON setting value
func unmarshalSetting(val string, target any) error {
	return json.Unmarshal([]byte(val), target)
}

// helper: marshal value to JSON string
func marshalSetting(val any) string {
	b, _ := json.Marshal(val)
	return string(b)
}

// --- LLM Settings ---

func GetLLMSettings(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := readSettingsGroup("llm")
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to read settings"})
			return
		}

		settings := model.LLMSettings{}
		if v, ok := raw["api_base_url"]; ok {
			unmarshalSetting(v, &settings.APIBaseURL)
		}
		if v, ok := raw["api_key"]; ok {
			unmarshalSetting(v, &settings.APIKey)
			// Mask API key for GET
			if len(settings.APIKey) > 8 {
				settings.APIKey = settings.APIKey[:4] + "****" + settings.APIKey[len(settings.APIKey)-4:]
			}
		}
		if v, ok := raw["default_model"]; ok {
			unmarshalSetting(v, &settings.DefaultModel)
		}
		if v, ok := raw["max_rounds"]; ok {
			unmarshalSetting(v, &settings.MaxRounds)
		}
		if v, ok := raw["temperature"]; ok {
			unmarshalSetting(v, &settings.Temperature)
		}

		c.JSON(http.StatusOK, settings)
	}
}

func PutLLMSettings(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.LLMSettings
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "bad_request", Message: "invalid request body"})
			return
		}

		settings := map[string]string{
			"llm.api_base_url": marshalSetting(req.APIBaseURL),
			"llm.api_key":      marshalSetting(req.APIKey),
			"llm.default_model": marshalSetting(req.DefaultModel),
			"llm.max_rounds":   marshalSetting(req.MaxRounds),
			"llm.temperature":  marshalSetting(req.Temperature),
		}

		if err := store.SetSettingsGroup(settings); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to save settings"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "updated"})
	}
}

// --- Security Settings ---

func GetSecuritySettings(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := readSettingsGroup("security")
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to read settings"})
			return
		}

		settings := model.SecuritySettings{}
		if v, ok := raw["confirm_enabled"]; ok {
			unmarshalSetting(v, &settings.ConfirmEnabled)
		}
		if v, ok := raw["sensitive_keywords"]; ok {
			unmarshalSetting(v, &settings.SensitiveKeywords)
		}
		if v, ok := raw["command_blacklist"]; ok {
			unmarshalSetting(v, &settings.CommandBlacklist)
		}

		c.JSON(http.StatusOK, settings)
	}
}

func PutSecuritySettings(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.SecuritySettings
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "bad_request", Message: "invalid request body"})
			return
		}

		settings := map[string]string{
			"security.confirm_enabled":    marshalSetting(req.ConfirmEnabled),
			"security.sensitive_keywords": marshalSetting(req.SensitiveKeywords),
			"security.command_blacklist":  marshalSetting(req.CommandBlacklist),
		}

		if err := store.SetSettingsGroup(settings); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to save settings"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "updated"})
	}
}

// --- Agent Settings ---

func GetAgentSettings(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := readSettingsGroup("agent")
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to read settings"})
			return
		}

		settings := model.AgentSettings{}
		if v, ok := raw["heartbeat_interval"]; ok {
			unmarshalSetting(v, &settings.HeartbeatInterval)
		}
		if v, ok := raw["command_timeout"]; ok {
			unmarshalSetting(v, &settings.CommandTimeout)
		}
		if v, ok := raw["output_max_lines"]; ok {
			unmarshalSetting(v, &settings.OutputMaxLines)
		}

		c.JSON(http.StatusOK, settings)
	}
}

func PutAgentSettings(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.AgentSettings
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "bad_request", Message: "invalid request body"})
			return
		}

		settings := map[string]string{
			"agent.heartbeat_interval": marshalSetting(req.HeartbeatInterval),
			"agent.command_timeout":    marshalSetting(req.CommandTimeout),
			"agent.output_max_lines":   marshalSetting(req.OutputMaxLines),
		}

		if err := store.SetSettingsGroup(settings); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to save settings"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "updated"})
	}
}

// --- Chat Settings ---

func GetChatSettings(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := readSettingsGroup("chat")
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to read settings"})
			return
		}

		settings := model.ChatSettings{}
		if v, ok := raw["context_rounds"]; ok {
			unmarshalSetting(v, &settings.ContextRounds)
		}
		if v, ok := raw["output_truncate_lines"]; ok {
			unmarshalSetting(v, &settings.OutputTruncateLines)
		}
		if v, ok := raw["custom_system_prompt"]; ok {
			var prompt string
			unmarshalSetting(v, &prompt)
			if prompt != "" {
				settings.CustomSystemPrompt = &prompt
			}
		}

		c.JSON(http.StatusOK, settings)
	}
}

func PutChatSettings(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.ChatSettings
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "bad_request", Message: "invalid request body"})
			return
		}

		prompt := ""
		if req.CustomSystemPrompt != nil {
			prompt = *req.CustomSystemPrompt
		}

		settings := map[string]string{
			"chat.context_rounds":        marshalSetting(req.ContextRounds),
			"chat.output_truncate_lines": marshalSetting(req.OutputTruncateLines),
			"chat.custom_system_prompt":  marshalSetting(prompt),
		}

		if err := store.SetSettingsGroup(settings); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to save settings"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "updated"})
	}
}

// --- Web Fetch Settings ---

// maskAPIKey returns a redacted form of an API key for safe display in GET
// responses. Anything that already contains "****" is treated as already masked
// and returned as-is.
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if strings.Contains(key, "****") {
		return key
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

func GetWebFetchSettings(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := readSettingsGroup("webfetch")
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to read settings"})
			return
		}

		settings := model.WebFetchSettings{
			Mode:       "jina",
			TimeoutSec: 30,
			MaxKB:      1024,
		}
		if v, ok := raw["mode"]; ok {
			unmarshalSetting(v, &settings.Mode)
		}
		if v, ok := raw["jina_api_key"]; ok {
			unmarshalSetting(v, &settings.JinaAPIKey)
			settings.JinaAPIKey = maskAPIKey(settings.JinaAPIKey)
		}
		if v, ok := raw["timeout_sec"]; ok {
			unmarshalSetting(v, &settings.TimeoutSec)
		}
		if v, ok := raw["max_kb"]; ok {
			unmarshalSetting(v, &settings.MaxKB)
		}

		c.JSON(http.StatusOK, settings)
	}
}

func PutWebFetchSettings(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.WebFetchSettings
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "bad_request", Message: "invalid request body"})
			return
		}

		mode := strings.ToLower(strings.TrimSpace(req.Mode))
		if mode != "jina" && mode != "local" {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "bad_request", Message: "mode must be 'jina' or 'local'"})
			return
		}

		updates := map[string]string{
			"webfetch.mode":        marshalSetting(mode),
			"webfetch.timeout_sec": marshalSetting(req.TimeoutSec),
			"webfetch.max_kb":      marshalSetting(req.MaxKB),
		}
		// Skip overwriting the stored API key if the client echoed back the
		// masked value — i.e. the user didn't edit the field. An empty string
		// means "clear", which we honor.
		if !strings.Contains(req.JinaAPIKey, "****") {
			updates["webfetch.jina_api_key"] = marshalSetting(req.JinaAPIKey)
		}

		if err := store.SetSettingsGroup(updates); err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to save settings"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "updated"})
	}
}

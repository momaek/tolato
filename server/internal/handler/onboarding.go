package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

// Onboarding handles GET /api/onboarding.
//
// The web home page walks a new user through two steps — get a node online,
// then hand Tolato to their own coding agent — and greys out chat until an LLM
// is configured. Node visibility already comes from /nodes; this answers the
// two questions the browser can't: has this user ever finished a CLI login,
// and can chat actually run. Both are booleans, so a member learns nothing
// about the LLM config beyond whether chat will work.
func Onboarding(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		keys, err := store.ListAPIKeysByOwner(middleware.CurrentUserID(c))
		if err != nil {
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to read api keys"})
			return
		}
		cliConnected := false
		for _, k := range keys {
			if k.Status == "active" && isCLIKeyName(k.Name) {
				cliConnected = true
				break
			}
		}

		llm := deps.Settings.LLM()
		c.JSON(http.StatusOK, gin.H{
			"cli_connected":  cliConnected,
			"llm_configured": llm.APIBaseURL != "" && llm.APIKey != "",
		})
	}
}

// isCLIKeyName recognises a key minted by `tolato auth login` from the name
// cliKeyName gave it. Keys carry no record of how they were made, and a name is
// good enough for a checklist tick: the worst a hand-made key called "CLI" can
// do is mark a step done early.
func isCLIKeyName(name string) bool {
	return name == cliKeyName("") || strings.HasPrefix(name, "CLI on ")
}

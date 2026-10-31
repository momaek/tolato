package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/momaek/tolato/server/internal/attestation"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

// ExternalVerifyAttestation accepts readonly keys, scoped to the signing node.
// The relying party owns nonce issuance and one-time consumption.
func ExternalVerifyAttestation(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
		var req struct {
			Token    string `json:"token"`
			Audience string `json:"audience"`
			Nonce    string `json:"nonce"`
		}
		if c.ShouldBindJSON(&req) != nil {
			badRequest(c, "invalid attestation request")
			return
		}
		claims, err := attestation.Decode(req.Token)
		if err != nil {
			badRequest(c, "invalid or expired attestation")
			return
		}
		if !requireNodeLevel(c, claims.NodeID, model.LevelViewer) {
			return
		}
		n, err := store.GetNodeByID(claims.NodeID)
		if err != nil {
			notFound(c, "node not found")
			return
		}
		verified, err := attestation.Verify(req.Token, n.AgentSecret, req.Audience, req.Nonce, time.Now())
		if err != nil {
			badRequest(c, "invalid or expired attestation")
			return
		}
		c.JSON(http.StatusOK, gin.H{"valid": true, "claims": verified})
	}
}

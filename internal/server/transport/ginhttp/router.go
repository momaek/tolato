package ginhttp

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func NewRouter(handler Handler) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	handler.RegisterRoutes(router)
	return router
}

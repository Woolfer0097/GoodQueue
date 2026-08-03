package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type DatabasePinger interface {
	PingContext(context.Context) error
}

type HealthHandler struct {
	database    DatabasePinger
	pingTimeout time.Duration
}

type HealthResponse struct {
	Status string `json:"status" binding:"required" example:"ok"`
}

func NewHealthHandler(database DatabasePinger, pingTimeout time.Duration) *HealthHandler {
	return &HealthHandler{database: database, pingTimeout: pingTimeout}
}

// Health godoc
//
//	@Summary		Liveness probe
//	@Description	Reports that the HTTP process is serving; does not check dependencies.
//	@Tags			infrastructure
//	@Produce		json
//	@Success		200	{object}	HealthResponse
//	@Router			/healthz [get]
func (handler *HealthHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResponse{Status: "ok"})
}

// Ready godoc
//
//	@Summary		Readiness probe
//	@Description	Reports whether PostgreSQL responds within the configured timeout. Dependency errors are never exposed.
//	@Tags			infrastructure
//	@Produce		json
//	@Success		200	{object}	HealthResponse
//	@Failure		503	{object}	HealthResponse
//	@Router			/readyz [get]
func (handler *HealthHandler) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), handler.pingTimeout)
	defer cancel()
	if err := handler.database.PingContext(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, HealthResponse{Status: "unavailable"})
		return
	}
	c.JSON(http.StatusOK, HealthResponse{Status: "ok"})
}

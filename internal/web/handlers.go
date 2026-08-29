package web

import (
	"net/http"

	"github.com/Karlsk/oryxos-go/internal/observability"
	"github.com/Karlsk/oryxos-go/internal/web/api"
	"github.com/gin-gonic/gin"
)

type healthResponse struct {
	Status string `json:"status"`
}

type infoResponse struct {
	Version string `json:"version"`
	Mode    string `json:"mode"`
	Ready   bool   `json:"ready"`
}

func healthHandler(observer observability.Observer) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !observer.Snapshot().Ready {
			api.Error(c, http.StatusServiceUnavailable, "not_ready", "service not ready", nil)
			return
		}
		api.Success(c, http.StatusOK, "ok", "ok", healthResponse{Status: "ready"})
	}
}

func infoHandler(observer observability.Observer, version string) gin.HandlerFunc {
	return func(c *gin.Context) {
		api.Success(c, http.StatusOK, "ok", "ok", infoResponse{
			Version: version,
			Mode:    "foundation",
			Ready:   observer.Snapshot().Ready,
		})
	}
}

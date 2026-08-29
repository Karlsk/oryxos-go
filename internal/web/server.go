// Package web owns the bounded foundation HTTP server.
package web

import (
	"log/slog"
	"net"
	"net/http"

	"github.com/Karlsk/oryxos-go/internal/config"
	"github.com/Karlsk/oryxos-go/internal/observability"
	"github.com/Karlsk/oryxos-go/internal/web/api"
	"github.com/Karlsk/oryxos-go/internal/web/middleware"
	"github.com/gin-gonic/gin"
)

const requestBodyLimit = 1 << 20

// Server is the bounded foundation router and its owned HTTP component.
type Server struct {
	router          *gin.Engine
	httpServer      *http.Server
	observer        observability.Observer
	listenerFactory ListenerFactory
	errors          chan error
	done            chan struct{}
	state           startState
}

// NewServer constructs the foundation router without binding a listener.
func NewServer(cfg config.ServerConfig, observer observability.Observer, baseLogger *slog.Logger, version string, factories ...ListenerFactory) *Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.Use(
		middleware.RequestID(),
		middleware.RequestBodyLimit(requestBodyLimit),
		middleware.AccessObservation(observer, baseLogger),
		middleware.Recovery(baseLogger),
	)
	router.NoRoute(func(c *gin.Context) {
		api.Error(c, http.StatusNotFound, "not_found", "not found", nil)
	})
	router.NoMethod(func(c *gin.Context) {
		api.Error(c, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	})

	v1 := router.Group("/api/v1")
	v1.GET("/health", healthHandler(observer))
	v1.GET("/info", infoHandler(observer, version))

	listenerFactory := ListenerFactory(net.Listen)
	if len(factories) > 0 && factories[0] != nil {
		listenerFactory = factories[0]
	}

	return &Server{
		router:          router,
		observer:        observer,
		listenerFactory: listenerFactory,
		errors:          make(chan error, 1),
		done:            make(chan struct{}),
		httpServer: &http.Server{
			Addr:              cfg.ListenAddress,
			Handler:           router,
			ReadHeaderTimeout: cfg.ReadHeaderTimeout,
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
		},
	}
}

// Handler returns the constructed router for tests and in-process callers.
func (s *Server) Handler() http.Handler {
	return s.router
}

// Routes returns the router's registered route inventory.
func (s *Server) Routes() gin.RoutesInfo {
	return s.router.Routes()
}

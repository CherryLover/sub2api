//go:build !embed

// Package web provides embedded web assets for the application.
package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// PublicSettingsProvider is an interface to fetch public settings
// This stub is needed for compilation when frontend is not embedded
type PublicSettingsProvider interface {
	GetPublicSettingsForInjection(ctx context.Context) (any, error)
}

// FrontendServer is a stub for non-embed builds
type FrontendServer struct{}

// NewFrontendServer returns an error when frontend is not embedded
func NewFrontendServer(settingsProvider PublicSettingsProvider) (*FrontendServer, error) {
	return nil, errors.New("frontend not embedded")
}

// LoginEntryResolver reports where the login page currently lives (stub).
type LoginEntryResolver func() LoginEntry

// NewFrontendServerWithLoginEntry returns an error when frontend is not embedded.
// Hidden login entries need the backend-rendered index.html, so they are only
// available in embed builds.
func NewFrontendServerWithLoginEntry(settingsProvider PublicSettingsProvider, loginEntry LoginEntry) (*FrontendServer, error) {
	return nil, errors.New("frontend not embedded")
}

// NewFrontendServerWithLoginEntryResolver returns an error when frontend is not embedded.
func NewFrontendServerWithLoginEntryResolver(settingsProvider PublicSettingsProvider, resolve LoginEntryResolver) (*FrontendServer, error) {
	return nil, errors.New("frontend not embedded")
}

// InvalidateCache is a no-op for non-embed builds
func (s *FrontendServer) InvalidateCache() {}

// Middleware returns a handler that returns 404 for non-embed builds
func (s *FrontendServer) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.String(http.StatusNotFound, "Frontend not embedded. Build with -tags embed to include frontend.")
		c.Abort()
	}
}

func ServeEmbeddedFrontend() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.String(http.StatusNotFound, "Frontend not embedded. Build with -tags embed to include frontend.")
		c.Abort()
	}
}

func HasEmbeddedFrontend() bool {
	return false
}

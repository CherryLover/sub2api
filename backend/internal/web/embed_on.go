//go:build embed

package web

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

const (
	// NonceHTMLPlaceholder is the placeholder for nonce in HTML script tags
	NonceHTMLPlaceholder = "__CSP_NONCE_VALUE__"
)

//go:embed all:dist
var frontendFS embed.FS

// PublicSettingsProvider is an interface to fetch public settings
type PublicSettingsProvider interface {
	GetPublicSettingsForInjection(ctx context.Context) (any, error)
}

// FrontendServer serves the embedded frontend with settings injection
type FrontendServer struct {
	distFS      fs.FS
	fileServer  http.Handler
	baseHTML    []byte
	cache       *HTMLCache
	settings    PublicSettingsProvider
	overrideDir string // local file override directory
	// loginEntryFn is consulted per request instead of a fixed value: the login
	// entry is now admin-editable at runtime, so a snapshot taken at boot would
	// keep serving the previous layout until the process restarted.
	loginEntryFn LoginEntryResolver
}

// LoginEntryResolver reports where the login page currently lives. It is called on
// the index.html path, so implementations must be cheap (cached) and never block.
type LoginEntryResolver func() LoginEntry

// NewFrontendServer creates a new frontend server with settings injection.
// The login entry stays in its default (public) layout: /login is a normal route.
func NewFrontendServer(settingsProvider PublicSettingsProvider) (*FrontendServer, error) {
	return NewFrontendServerWithLoginEntry(settingsProvider, LoginEntry{})
}

// NewFrontendServerWithLoginEntry creates a frontend server whose login entry never
// changes for the lifetime of the process (used by tests and by callers that have a
// fixed layout).
func NewFrontendServerWithLoginEntry(settingsProvider PublicSettingsProvider, loginEntry LoginEntry) (*FrontendServer, error) {
	loginEntry.Path = NormalizeEntryPath(loginEntry.Path)
	return NewFrontendServerWithLoginEntryResolver(settingsProvider, func() LoginEntry { return loginEntry })
}

// NewFrontendServerWithLoginEntryResolver creates a frontend server that asks resolve
// where the login page lives on every index.html request. When the entry is hidden,
// the configured path never reaches the bundle or any API response — only a request
// that hits it exactly gets an HTML response carrying the "render the login page" flag.
func NewFrontendServerWithLoginEntryResolver(settingsProvider PublicSettingsProvider, resolve LoginEntryResolver) (*FrontendServer, error) {
	distFS, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		return nil, err
	}

	// Read base HTML once
	file, err := distFS.Open("index.html")
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	baseHTML, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	cache := NewHTMLCache()
	cache.SetBaseHTML(baseHTML)

	return &FrontendServer{
		distFS:       distFS,
		fileServer:   http.FileServer(http.FS(distFS)),
		baseHTML:     baseHTML,
		cache:        cache,
		settings:     settingsProvider,
		overrideDir:  filepath.Join("data", "public"),
		loginEntryFn: resolve,
	}, nil
}

// loginEntry returns the current login entry layout, normalized.
func (s *FrontendServer) loginEntry() LoginEntry {
	if s == nil || s.loginEntryFn == nil {
		return LoginEntry{}
	}
	entry := s.loginEntryFn()
	entry.Path = NormalizeEntryPath(entry.Path)
	return entry
}

// InvalidateCache invalidates the HTML cache (call when settings change)
func (s *FrontendServer) InvalidateCache() {
	if s != nil && s.cache != nil {
		s.cache.Invalidate()
	}
}

// Middleware returns the Gin middleware handler
func (s *FrontendServer) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Skip API routes
		if shouldBypassEmbeddedFrontend(path) {
			c.Next()
			return
		}

		cleanPath := strings.TrimPrefix(path, "/")
		if cleanPath == "" {
			cleanPath = "index.html"
		}

		// For index.html or SPA routes, serve with injected settings
		if cleanPath == "index.html" || !s.fileExists(cleanPath) {
			s.serveIndexHTML(c)
			return
		}

		// Try local override first
		if s.tryServeOverride(c, cleanPath) {
			return
		}

		// Serve static files normally (hashed assets get long-lived cache headers)
		applyStaticAssetCacheHeaders(c.Writer.Header(), cleanPath)
		s.fileServer.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	}
}

func (s *FrontendServer) fileExists(path string) bool {
	file, err := s.distFS.Open(path)
	if err != nil {
		return false
	}
	_ = file.Close()
	return true
}

// tryServeOverride checks if a local override file exists and serves it.
// Files in overrideDir take precedence over embedded files.
func (s *FrontendServer) tryServeOverride(c *gin.Context, cleanPath string) bool {
	if s.overrideDir == "" {
		return false
	}
	filePath := filepath.Join(s.overrideDir, filepath.Clean("/"+cleanPath))
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		return false
	}
	c.File(filePath)
	c.Abort()
	return true
}

func (s *FrontendServer) serveIndexHTML(c *gin.Context) {
	// Get nonce from context (generated by SecurityHeaders middleware)
	nonce := middleware.GetNonceFromContext(c)

	// Does this request hit the hidden login path? Only this one request gets the
	// login flag; the shared cache never stores a copy that carries it.
	loginEntry := s.loginEntry()
	isLoginEntry := loginEntry.Matches(c.Request.URL.Path)

	// Check cache first
	cached := s.cache.Get()
	if cached != nil {
		etag := cached.ETagFor(isLoginEntry)
		// Check If-None-Match for 304 response.
		// The ETag is variant-specific so a browser that cached an ordinary page can
		// never be handed a 304 for the login entry (or the other way round).
		if match := c.GetHeader("If-None-Match"); match == etag {
			c.Status(http.StatusNotModified)
			c.Abort()
			return
		}

		// Replace placeholders with per-request values before serving
		content := replaceNoncePlaceholder(cached.Content, nonce)
		content = s.replaceLoginEntryFlag(content, isLoginEntry)

		c.Header("ETag", etag)
		c.Header("Cache-Control", "no-cache") // Must revalidate
		c.Data(http.StatusOK, "text/html; charset=utf-8", content)
		c.Abort()
		return
	}

	// Cache miss - fetch settings and render
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	settings, err := s.settings.GetPublicSettingsForInjection(ctx)
	if err != nil {
		// Fallback: serve without injection
		c.Data(http.StatusOK, "text/html; charset=utf-8", s.baseHTML)
		c.Abort()
		return
	}

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		// Fallback: serve without injection
		c.Data(http.StatusOK, "text/html; charset=utf-8", s.baseHTML)
		c.Abort()
		return
	}

	rendered := s.injectSettingsFor(loginEntry, settingsJSON)
	s.cache.Set(rendered, settingsJSON)

	// Replace placeholders with per-request values before serving
	content := replaceNoncePlaceholder(rendered, nonce)
	content = s.replaceLoginEntryFlag(content, isLoginEntry)

	cached = s.cache.Get()
	if cached != nil {
		c.Header("ETag", cached.ETagFor(isLoginEntry))
	}
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/html; charset=utf-8", content)
	c.Abort()
}

// injectSettings renders the cached HTML for the current login entry layout.
func (s *FrontendServer) injectSettings(settingsJSON []byte) []byte {
	return s.injectSettingsFor(s.loginEntry(), settingsJSON)
}

func (s *FrontendServer) injectSettingsFor(loginEntry LoginEntry, settingsJSON []byte) []byte {
	// Create the script tag to inject with nonce placeholder
	// The placeholder will be replaced with actual nonce at request time.
	//
	// In hidden-login mode a second assignment carries the login entry flag as a
	// placeholder. The custom path itself is deliberately NOT part of settingsJSON:
	// that JSON is embedded into every page and is also served verbatim by
	// /api/v1/settings/public, so anything in it is public by definition.
	body := `window.__APP_CONFIG__=` + string(settingsJSON) + `;`
	if loginEntry.Enabled() {
		body += `window.__LOGIN_ENTRY__=` + LoginEntryFlagPlaceholder + `;`
	}
	script := []byte(`<script nonce="` + NonceHTMLPlaceholder + `">` + body + `</script>`)

	// Inject before </head>
	headClose := []byte("</head>")
	result := bytes.Replace(s.baseHTML, headClose, append(script, headClose...), 1)

	return result
}

// replaceNoncePlaceholder replaces the nonce placeholder with actual nonce value
func replaceNoncePlaceholder(html []byte, nonce string) []byte {
	return bytes.ReplaceAll(html, []byte(NonceHTMLPlaceholder), []byte(nonce))
}

// replaceLoginEntryFlag swaps the cached login-entry placeholder for this request's
// value. "1" and "0" are both a single byte, so the hidden login page and every
// ordinary page produce byte-identical response lengths.
//
// The substitution is unconditional: the layout can flip while a rendered page is
// still in the cache, and a placeholder that survived into a response would leak the
// marker's existence (and break the page). ReplaceAll is a no-op when the cached HTML
// carries no placeholder, which is the public-mode case.
func (s *FrontendServer) replaceLoginEntryFlag(html []byte, isLoginEntry bool) []byte {
	flag := []byte("0")
	if isLoginEntry {
		flag = []byte("1")
	}
	return bytes.ReplaceAll(html, []byte(LoginEntryFlagPlaceholder), flag)
}

// ServeEmbeddedFrontend returns a middleware for serving embedded frontend
// This is the legacy function for backward compatibility when no settings provider is available
func ServeEmbeddedFrontend() gin.HandlerFunc {
	distFS, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		panic("failed to get dist subdirectory: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(distFS))
	overrideDir := filepath.Join("data", "public")

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		if shouldBypassEmbeddedFrontend(path) {
			c.Next()
			return
		}

		cleanPath := strings.TrimPrefix(path, "/")
		if cleanPath == "" {
			cleanPath = "index.html"
		}

		if file, err := distFS.Open(cleanPath); err == nil {
			_ = file.Close()
			// Try local override first
			if tryServeOverrideFile(c, overrideDir, cleanPath) {
				return
			}
			applyStaticAssetCacheHeaders(c.Writer.Header(), cleanPath)
			fileServer.ServeHTTP(c.Writer, c.Request)
			c.Abort()
			return
		}

		serveIndexHTML(c, distFS)
	}
}

// tryServeOverrideFile is a standalone version of tryServeOverride for legacy usage.
func tryServeOverrideFile(c *gin.Context, overrideDir, cleanPath string) bool {
	if overrideDir == "" {
		return false
	}
	filePath := filepath.Join(overrideDir, filepath.Clean("/"+cleanPath))
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		return false
	}
	c.File(filePath)
	c.Abort()
	return true
}

func shouldBypassEmbeddedFrontend(path string) bool {
	trimmed := strings.TrimSpace(path)
	return strings.HasPrefix(trimmed, "/api/") ||
		strings.HasPrefix(trimmed, "/v1/") ||
		strings.HasPrefix(trimmed, "/v1beta/") ||
		strings.HasPrefix(trimmed, "/backend-api/") ||
		strings.HasPrefix(trimmed, "/antigravity/") ||
		strings.HasPrefix(trimmed, "/setup/") ||
		trimmed == "/health" ||
		trimmed == "/models" ||
		trimmed == "/responses" ||
		strings.HasPrefix(trimmed, "/responses/") ||
		trimmed == "/alpha/search" ||
		strings.HasPrefix(trimmed, "/images/") ||
		strings.HasPrefix(trimmed, "/videos/")
}

func serveIndexHTML(c *gin.Context, fsys fs.FS) {
	file, err := fsys.Open("index.html")
	if err != nil {
		c.String(http.StatusNotFound, "Frontend not found")
		c.Abort()
		return
	}
	defer func() { _ = file.Close() }()

	content, err := io.ReadAll(file)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to read index.html")
		c.Abort()
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", content)
	c.Abort()
}

func HasEmbeddedFrontend() bool {
	_, err := frontendFS.ReadFile("dist/index.html")
	return err == nil
}

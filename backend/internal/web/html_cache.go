//go:build embed

package web

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// HTMLCache manages the cached index.html with injected settings
type HTMLCache struct {
	mu              sync.RWMutex
	cachedHTML      []byte
	etag            string
	loginEntryETag  string
	baseHTMLHash    string // Hash of the original index.html (immutable after build)
	settingsVersion uint64 // Incremented when settings change
}

// CachedHTML represents the cache state.
//
// Content always holds the placeholder form of the HTML — the nonce and the login
// entry flag are substituted per request, so the cached bytes can never carry the
// "this is the login page" marker and pollute an ordinary page load.
type CachedHTML struct {
	Content []byte
	ETag    string
	// LoginEntryETag is the ETag for the variant served on the hidden login path.
	// Both ETags are fixed-width hashes, so their values differ but their lengths
	// do not — the two responses stay indistinguishable by size.
	LoginEntryETag string
}

// ETagFor returns the ETag matching the variant that is about to be served.
func (c *CachedHTML) ETagFor(isLoginEntry bool) string {
	if c == nil {
		return ""
	}
	if isLoginEntry {
		return c.LoginEntryETag
	}
	return c.ETag
}

// NewHTMLCache creates a new HTML cache instance
func NewHTMLCache() *HTMLCache {
	return &HTMLCache{}
}

// SetBaseHTML initializes the cache with the base HTML template
func (c *HTMLCache) SetBaseHTML(baseHTML []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	hash := sha256.Sum256(baseHTML)
	c.baseHTMLHash = hex.EncodeToString(hash[:8]) // First 8 bytes for brevity
}

// Invalidate marks the cache as stale
func (c *HTMLCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.settingsVersion++
	c.cachedHTML = nil
	c.etag = ""
	c.loginEntryETag = ""
}

// Get returns the cached HTML or nil if cache is stale
func (c *HTMLCache) Get() *CachedHTML {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.cachedHTML == nil {
		return nil
	}
	return &CachedHTML{
		Content:        c.cachedHTML,
		ETag:           c.etag,
		LoginEntryETag: c.loginEntryETag,
	}
}

// Set updates the cache with new rendered HTML
func (c *HTMLCache) Set(html []byte, settingsJSON []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cachedHTML = html
	c.etag = c.generateETag(settingsJSON)
	c.loginEntryETag = c.generateLoginEntryETag(settingsJSON)
}

// generateETag creates an ETag from base HTML hash + settings hash
func (c *HTMLCache) generateETag(settingsJSON []byte) string {
	settingsHash := sha256.Sum256(settingsJSON)
	return `"` + c.baseHTMLHash + "-" + hex.EncodeToString(settingsHash[:8]) + `"`
}

// generateLoginEntryETag derives a distinct ETag for the hidden-login variant by
// hashing the settings together with a domain separator. Same length as the normal
// ETag, different value — so conditional requests can never cross between the two.
func (c *HTMLCache) generateLoginEntryETag(settingsJSON []byte) string {
	settingsHash := sha256.Sum256(append([]byte("login-entry\x00"), settingsJSON...))
	return `"` + c.baseHTMLHash + "-" + hex.EncodeToString(settingsHash[:8]) + `"`
}

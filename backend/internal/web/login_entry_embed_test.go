//go:build embed

package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testHiddenLoginPath = "/j7q2m9x4vk3p"

func hiddenLoginServer(t *testing.T) *FrontendServer {
	t.Helper()
	provider := &mockSettingsProvider{
		settings: map[string]any{"version": "test", "login_entry_public": false, "default_home_path": "/key-usage"},
	}
	server, err := NewFrontendServerWithLoginEntry(provider, LoginEntry{Hidden: true, Path: testHiddenLoginPath})
	require.NoError(t, err)
	return server
}

// serveHTML drives the request through a real gin engine: gin only flushes a
// bare c.Status(304) when the engine finishes the chain, so a hand-built test
// context would report 200 for every conditional request.
func serveHTML(t *testing.T, server *FrontendServer, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.CSPNonceKey, "test-nonce")
		c.Next()
	})
	router.Use(server.Middleware())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	router.ServeHTTP(w, req)
	return w
}

// The whole point of the feature: the custom path must not be discoverable from
// any page the public can fetch.
func TestHiddenLoginPath_NeverAppearsInServedHTML(t *testing.T) {
	server := hiddenLoginServer(t)

	for _, path := range []string{"/", "/home", "/login", "/key-usage", "/definitely-not-a-route", testHiddenLoginPath} {
		body := serveHTML(t, server, path, nil).Body.String()
		assert.NotContains(t, body, testHiddenLoginPath,
			"custom login path leaked into the HTML served for %s", path)
		assert.NotContains(t, body, LoginEntryFlagPlaceholder,
			"login entry placeholder must be substituted before serving (%s)", path)
	}
}

func TestHiddenLoginPath_FlagOnlyOnTheEntryPath(t *testing.T) {
	server := hiddenLoginServer(t)

	entry := serveHTML(t, server, testHiddenLoginPath, nil).Body.String()
	assert.Contains(t, entry, "window.__LOGIN_ENTRY__=1;")

	for _, path := range []string{"/", "/home", "/login", "/key-usage", "/j7q2m9x4vk3", "/j7q2m9x4vk3p2", "/nope"} {
		body := serveHTML(t, server, path, nil).Body.String()
		assert.Contains(t, body, "window.__LOGIN_ENTRY__=0;", "expected the off flag for %s", path)
		assert.NotContains(t, body, "window.__LOGIN_ENTRY__=1;", "login flag leaked to %s", path)
	}
}

// A trailing slash is normalized rather than dropped on the floor: /gate/ still
// reaches the login page, so operators do not file bugs about a "broken" path.
func TestHiddenLoginPath_TrailingSlashStillMatches(t *testing.T) {
	server := hiddenLoginServer(t)
	body := serveHTML(t, server, testHiddenLoginPath+"/", nil).Body.String()
	assert.Contains(t, body, "window.__LOGIN_ENTRY__=1;")
}

// Length and timing side channels: both variants must be byte-for-byte the same
// size so the entry cannot be found by diffing Content-Length across a wordlist.
func TestHiddenLoginPath_ResponsesAreTheSameLength(t *testing.T) {
	server := hiddenLoginServer(t)

	entry := serveHTML(t, server, testHiddenLoginPath, nil)
	other := serveHTML(t, server, "/some-other-path", nil)

	assert.Equal(t, other.Body.Len(), entry.Body.Len(),
		"hidden login page and an ordinary page must have identical response sizes")
	assert.Equal(t, len(other.Header().Get("ETag")), len(entry.Header().Get("ETag")))
	assert.Equal(t, other.Header().Get("Cache-Control"), entry.Header().Get("Cache-Control"))
	assert.Equal(t, other.Code, entry.Code)
}

// The shared HTML cache stores the placeholder form, so warming it from the login
// path can never hand the login marker to the next visitor of an ordinary page.
func TestHiddenLoginPath_DoesNotPolluteHTMLCache(t *testing.T) {
	server := hiddenLoginServer(t)

	// Warm the cache through the hidden entry first.
	entry := serveHTML(t, server, testHiddenLoginPath, nil).Body.String()
	require.Contains(t, entry, "window.__LOGIN_ENTRY__=1;")

	cached := server.cache.Get()
	require.NotNil(t, cached)
	assert.Contains(t, string(cached.Content), LoginEntryFlagPlaceholder,
		"cache must hold the placeholder, never a resolved flag")
	assert.NotContains(t, string(cached.Content), "window.__LOGIN_ENTRY__=1;")

	// Every subsequent ordinary page must come back with the flag off.
	for i := 0; i < 3; i++ {
		body := serveHTML(t, server, "/home", nil).Body.String()
		assert.Contains(t, body, "window.__LOGIN_ENTRY__=0;")
		assert.NotContains(t, body, "window.__LOGIN_ENTRY__=1;")
	}
}

// Conditional requests must not cross between the two variants: a browser holding
// the ordinary-page ETag must not get a 304 for the login entry, or it would
// render the wrong page from its cache.
func TestHiddenLoginPath_ETagsDoNotCrossVariants(t *testing.T) {
	server := hiddenLoginServer(t)

	ordinary := serveHTML(t, server, "/home", nil)
	ordinaryETag := ordinary.Header().Get("ETag")
	require.NotEmpty(t, ordinaryETag)

	entry := serveHTML(t, server, testHiddenLoginPath, nil)
	entryETag := entry.Header().Get("ETag")
	require.NotEmpty(t, entryETag)
	require.NotEqual(t, ordinaryETag, entryETag)

	// Ordinary ETag presented on the login path => full 200 with the login flag.
	crossed := serveHTML(t, server, testHiddenLoginPath, map[string]string{"If-None-Match": ordinaryETag})
	assert.Equal(t, http.StatusOK, crossed.Code)
	assert.Contains(t, crossed.Body.String(), "window.__LOGIN_ENTRY__=1;")

	// Login ETag presented on an ordinary path => full 200 with the flag off.
	crossedBack := serveHTML(t, server, "/home", map[string]string{"If-None-Match": entryETag})
	assert.Equal(t, http.StatusOK, crossedBack.Code)
	assert.Contains(t, crossedBack.Body.String(), "window.__LOGIN_ENTRY__=0;")

	// Matching ETags still short-circuit as before.
	assert.Equal(t, http.StatusNotModified,
		serveHTML(t, server, "/home", map[string]string{"If-None-Match": ordinaryETag}).Code)
	assert.Equal(t, http.StatusNotModified,
		serveHTML(t, server, testHiddenLoginPath, map[string]string{"If-None-Match": entryETag}).Code)
}

// Public mode must behave exactly as before: no extra script, no extra assignment.
func TestPublicLoginEntry_InjectsNoMarker(t *testing.T) {
	provider := &mockSettingsProvider{settings: map[string]any{"version": "test"}}
	server, err := NewFrontendServer(provider)
	require.NoError(t, err)

	body := serveHTML(t, server, "/login", nil).Body.String()
	assert.NotContains(t, body, "window.__LOGIN_ENTRY__")
	assert.NotContains(t, body, LoginEntryFlagPlaceholder)

	cached := server.cache.Get()
	require.NotNil(t, cached)
	assert.Equal(t, cached.ETag, cached.ETagFor(false))
}

func TestLoginEntry_Matches(t *testing.T) {
	entry := LoginEntry{Hidden: true, Path: "/gate-abcdef"}
	assert.True(t, entry.Matches("/gate-abcdef"))
	assert.True(t, entry.Matches("/gate-abcdef/"))
	assert.False(t, entry.Matches("/gate-abcde"))
	assert.False(t, entry.Matches("/gate-abcdefg"))
	assert.False(t, entry.Matches("/GATE-ABCDEF"))
	assert.False(t, entry.Matches("/login"))

	// Hidden without a path (or public) never matches anything: falling back to
	// "everything is the login page" would be worse than not hiding at all.
	assert.False(t, LoginEntry{Hidden: true}.Matches("/"))
	assert.False(t, LoginEntry{Path: "/gate-abcdef"}.Matches("/gate-abcdef"))
	assert.False(t, LoginEntry{}.Enabled())
}

// The hidden path lives only in the server's own config. Nothing in the settings
// payload — which is injected into every page and also returned verbatim by
// /api/v1/settings/public — may carry it.
func TestHiddenLoginPath_NotPartOfInjectedSettingsJSON(t *testing.T) {
	server := hiddenLoginServer(t)
	rendered := string(server.injectSettings([]byte(`{"version":"test","login_entry_public":false}`)))

	assert.NotContains(t, rendered, testHiddenLoginPath)
	assert.Contains(t, rendered, LoginEntryFlagPlaceholder)

	configStart := strings.Index(rendered, "window.__APP_CONFIG__=")
	entryStart := strings.Index(rendered, "window.__LOGIN_ENTRY__=")
	require.Greater(t, entryStart, configStart, "login flag is a separate assignment, not a settings field")
}

// 登录入口现在可以在管理后台改，所以布置必须是每次请求现问的，而不是启动时拍下的
// 快照——否则站长在后台藏起入口之后，进程不重启就一直按旧布置服务。
func TestHiddenLoginPath_LayoutIsResolvedPerRequest(t *testing.T) {
	provider := &mockSettingsProvider{
		settings: map[string]any{"version": "test", "login_entry_public": true},
	}
	var current LoginEntry
	server, err := NewFrontendServerWithLoginEntryResolver(provider, func() LoginEntry { return current })
	require.NoError(t, err)

	// 公开布置：既没有标记，也没有占位符。
	body := serveHTML(t, server, "/login", nil).Body.String()
	assert.NotContains(t, body, "window.__LOGIN_ENTRY__")
	assert.NotContains(t, body, LoginEntryFlagPlaceholder)

	// 后台把入口藏起来：设置变更会让 HTML 缓存失效（router 的 onUpdate 回调），
	// 重新渲染之后隐藏路径立刻可用，且路径本身依然不出现在任何响应里。
	current = LoginEntry{Hidden: true, Path: testHiddenLoginPath}
	server.InvalidateCache()

	entry := serveHTML(t, server, testHiddenLoginPath, nil).Body.String()
	assert.Contains(t, entry, "window.__LOGIN_ENTRY__=1;")
	assert.NotContains(t, entry, testHiddenLoginPath)

	other := serveHTML(t, server, "/home", nil).Body.String()
	assert.Contains(t, other, "window.__LOGIN_ENTRY__=0;")
	assert.NotContains(t, other, testHiddenLoginPath)

	// 再翻回公开：旧的隐藏路径不再是登录页。
	current = LoginEntry{}
	server.InvalidateCache()
	reverted := serveHTML(t, server, testHiddenLoginPath, nil).Body.String()
	assert.NotContains(t, reverted, "window.__LOGIN_ENTRY__=1;")
}

// 布置刚翻转、缓存里还留着上一份渲染结果时，占位符绝不能原样漏进响应里：
// 那既会暴露"这里有个登录标记"，也会让页面上的脚本解析失败。
func TestHiddenLoginPath_PlaceholderNeverSurvivesALayoutFlip(t *testing.T) {
	provider := &mockSettingsProvider{settings: map[string]any{"version": "test"}}
	current := LoginEntry{Hidden: true, Path: testHiddenLoginPath}
	server, err := NewFrontendServerWithLoginEntryResolver(provider, func() LoginEntry { return current })
	require.NoError(t, err)

	// 先把带占位符的那份 HTML 灌进缓存。
	require.Contains(t, serveHTML(t, server, testHiddenLoginPath, nil).Body.String(), "window.__LOGIN_ENTRY__=1;")

	// 翻成公开但故意不清缓存（模拟缓存失效与布置翻转之间的那一瞬）。
	current = LoginEntry{}
	body := serveHTML(t, server, "/home", nil).Body.String()
	assert.NotContains(t, body, LoginEntryFlagPlaceholder)
	assert.Contains(t, body, "window.__LOGIN_ENTRY__=0;")
}

package config

import (
	"strings"
	"testing"
)

func webCfg(public bool, entryPath, homePath string) *Config {
	return &Config{
		Web: WebConfig{
			LoginEntryPublic: public,
			LoginEntryPath:   entryPath,
			DefaultHomePath:  homePath,
		},
	}
}

func TestNormalizeAndValidateWeb_Defaults(t *testing.T) {
	cfg := webCfg(true, "", "")
	if err := cfg.normalizeAndValidateWeb(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Web.DefaultHomePath != DefaultHomePathFallback {
		t.Fatalf("default home path = %q, want %q", cfg.Web.DefaultHomePath, DefaultHomePathFallback)
	}
	if cfg.LoginEntryHidden() {
		t.Fatal("login entry must stay public by default")
	}
	if cfg.ResolvedDefaultHomePath() != "/key-usage" {
		t.Fatalf("resolved default home = %q", cfg.ResolvedDefaultHomePath())
	}
}

func TestNormalizeAndValidateWeb_DefaultHomePath(t *testing.T) {
	for _, path := range []string{"/home", "/key-usage"} {
		cfg := webCfg(true, "", path)
		if err := cfg.normalizeAndValidateWeb(); err != nil {
			t.Fatalf("%s should be allowed: %v", path, err)
		}
	}

	// Trailing slash is normalized, not rejected.
	cfg := webCfg(true, "", "/home/")
	if err := cfg.normalizeAndValidateWeb(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Web.DefaultHomePath != "/home" {
		t.Fatalf("normalized default home = %q", cfg.Web.DefaultHomePath)
	}

	// /login is only a valid landing page while the login entry is public.
	cfg = webCfg(true, "", "/login")
	if err := cfg.normalizeAndValidateWeb(); err != nil {
		t.Fatalf("/login should be allowed while public: %v", err)
	}
	cfg = webCfg(false, "/j7q2m9x4vk", "/login")
	if err := cfg.normalizeAndValidateWeb(); err == nil {
		t.Fatal("/login must be rejected as landing page when the login entry is hidden")
	}

	// Pages behind auth would bounce unauthenticated visitors back to the landing
	// page forever, so they are rejected outright.
	for _, path := range []string{"/dashboard", "/admin/dashboard", "/keys", "/nope", "relative"} {
		cfg := webCfg(true, "", path)
		if err := cfg.normalizeAndValidateWeb(); err == nil {
			t.Fatalf("default_home_path %q must be rejected", path)
		}
	}
}

func TestNormalizeAndValidateWeb_HiddenRequiresPath(t *testing.T) {
	cfg := webCfg(false, "", "/key-usage")
	err := cfg.normalizeAndValidateWeb()
	if err == nil {
		t.Fatal("hiding the login entry without a custom path must fail startup")
	}
	if !strings.Contains(err.Error(), "web.login_entry_path is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeAndValidateWeb_ValidHiddenPath(t *testing.T) {
	cfg := webCfg(false, "/j7q2m9x4vk3p/", "/key-usage")
	if err := cfg.normalizeAndValidateWeb(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Web.LoginEntryPath != "/j7q2m9x4vk3p" {
		t.Fatalf("normalized entry path = %q", cfg.Web.LoginEntryPath)
	}
	if !cfg.LoginEntryHidden() {
		t.Fatal("login entry should be hidden")
	}

	// Nested paths are fine as long as no segment is reserved.
	cfg = webCfg(false, "/gate/j7q2m9x4vk3p", "/key-usage")
	if err := cfg.normalizeAndValidateWeb(); err != nil {
		t.Fatalf("nested path should be allowed: %v", err)
	}
}

func TestNormalizeAndValidateWeb_RejectsBadLoginEntryPaths(t *testing.T) {
	cases := map[string]string{
		"missing leading slash": "j7q2m9x4vk",
		"too short":             "/ab",
		"is /login":             "/login",
		"existing route":        "/key-usage",
		"existing route home":   "/home",
		"backend api prefix":    "/api/secret-gate",
		"gateway v1 prefix":     "/v1/secret-gate",
		"setup prefix":          "/setup/secret-gate",
		"admin prefix":          "/admin/secret-gate",
		"auth prefix":           "/auth/secret-gate",
		"legal prefix":          "/legal/secret-gate",
		"static assets":         "/assets/secret-gate",
		"file extension":        "/secret.gate.html",
		"empty segment":         "/secret//gate",
		"query characters":      "/secret?gate=1",
		"space":                 "/secret gate",
		"root":                  "/",
		"too long":              "/" + strings.Repeat("a", 200),
	}
	for name, path := range cases {
		cfg := webCfg(false, path, "/key-usage")
		if err := cfg.normalizeAndValidateWeb(); err == nil {
			t.Fatalf("%s: login_entry_path %q must be rejected", name, path)
		}
	}
}

func TestNormalizeAndValidateWeb_ValidatesPathEvenWhenPublic(t *testing.T) {
	// A bad path stored while the entry is public must fail now, not later when the
	// operator flips login_entry_public to false in production.
	cfg := webCfg(true, "/api/oops", "/home")
	if err := cfg.normalizeAndValidateWeb(); err == nil {
		t.Fatal("invalid login_entry_path must be rejected even while public")
	}
}

// Web entry validation runs early in Config.Validate, so a bad entry layout is
// what aborts startup — the operator gets the reason instead of a service that
// silently came up in the public layout it was told to hide.
func TestConfigValidateRejectsInvalidWebEntry(t *testing.T) {
	cfg := &Config{Web: WebConfig{LoginEntryPublic: false, DefaultHomePath: "/key-usage"}}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "web.login_entry_path is required") {
		t.Fatalf("Config.Validate must reject a hidden login entry without a path, got %v", err)
	}

	cfg = &Config{Web: WebConfig{LoginEntryPublic: true, DefaultHomePath: "/dashboard"}}
	err = cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "web.default_home_path") {
		t.Fatalf("Config.Validate must reject a protected default_home_path, got %v", err)
	}

	cfg = &Config{Web: WebConfig{LoginEntryPublic: false, LoginEntryPath: "/api/oops", DefaultHomePath: "/key-usage"}}
	err = cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "web.login_entry_path") {
		t.Fatalf("Config.Validate must reject a reserved login_entry_path, got %v", err)
	}
}

func TestNormalizeEntryPath(t *testing.T) {
	cases := map[string]string{
		"":            "",
		"/":           "/",
		"  /gate  ":   "/gate",
		"/gate/":      "/gate",
		"/gate///":    "/gate",
		"/a/b/":       "/a/b",
		"/key-usage/": "/key-usage",
	}
	for in, want := range cases {
		if got := NormalizeEntryPath(in); got != want {
			t.Fatalf("NormalizeEntryPath(%q) = %q, want %q", in, got, want)
		}
	}
}

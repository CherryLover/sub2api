package service

import (
	"net/url"
	"strings"
)

// isKimiCodingBaseURL matches the official Kimi Coding endpoints only.
func isKimiCodingBaseURL(baseURL string) bool {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Opaque != "" ||
		u.RawQuery != "" || u.ForceQuery || strings.Contains(baseURL, "#") {
		return false
	}
	host := strings.ToLower(u.Host)
	if host != "api.kimi.com" && host != "api.kimi.ai" && host != "api.kimi.com:443" && host != "api.kimi.ai:443" {
		return false
	}
	switch u.EscapedPath() {
	case "/coding", "/coding/", "/coding/v1", "/coding/v1/":
		return true
	default:
		return false
	}
}

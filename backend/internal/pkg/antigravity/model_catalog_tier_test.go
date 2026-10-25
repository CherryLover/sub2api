//go:build unit

package antigravity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPaidTierVariantsRecognized(t *testing.T) {
	tests := []struct {
		tier string
		plan string
		paid bool
	}{
		{"g1-pro-tier", "Pro", true},
		{"g1-ultra-tier", "Ultra", true},
		{"g1-pro-plus-tier", "Pro", true},
		{"g1-ultra-lite-tier", "Ultra", true},
		{"G1-PRO-PLUS-TIER", "Pro", true},
		{"free-tier", "Free", false},
		{"unknown-tier", "unknown-tier", false},
		{"g1-pro-unknown", "g1-pro-unknown", false},
	}
	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			if got := TierIDToPlanType(tt.tier); got != tt.plan {
				t.Fatalf("TierIDToPlanType(%q) = %q, want %q", tt.tier, got, tt.plan)
			}
			if paid := IsPaidPlanType(tt.tier); paid != tt.paid {
				t.Fatalf("IsPaidPlanType(%q) = %v, want %v", tt.tier, paid, tt.paid)
			}
		})
	}
}

func TestFetchAvailableModelsFromURLUsesOnlyConfiguredEndpoint(t *testing.T) {
	var first, redirected int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirected++
		_, _ = w.Write([]byte(`{"models":{"other-tier-model":{}}}`))
	}))
	defer target.Close()

	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		first++
		if r.URL.Path != "/v1internal:fetchAvailableModels" {
			t.Errorf("request path %q", r.URL.Path)
		}
		w.Header().Set("Location", target.URL+"/v1internal:fetchAvailableModels")
		w.WriteHeader(http.StatusFound)
	}))
	defer primary.Close()

	client := &Client{httpClient: primary.Client()}
	_, _, err := client.FetchAvailableModelsFromURL(context.Background(), primary.URL, "token", "project")
	if err == nil {
		t.Fatal("fixed endpoint must not follow a cross-endpoint redirect")
	}
	if first != 1 || redirected != 0 {
		t.Fatalf("first=%d redirected=%d, want first=1 redirected=0", first, redirected)
	}
}

func TestFetchAvailableModelsFromURLReturnsSelectedModelList(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"models":{"gemini-4-argon":{"displayName":"Argon","supportsThinking":true}}}`))
	}))
	defer server.Close()
	client := &Client{httpClient: server.Client()}
	resp, raw, err := client.FetchAvailableModelsFromURL(context.Background(), server.URL, "token", "project")
	if err != nil {
		t.Fatal(err)
	}
	if hits != 1 || resp.Models["gemini-4-argon"].DisplayName != "Argon" || raw["models"] == nil {
		t.Fatalf("unexpected model catalog response: hits=%d resp=%+v raw=%+v", hits, resp, raw)
	}
}

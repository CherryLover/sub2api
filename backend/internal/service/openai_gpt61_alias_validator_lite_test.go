package service

import "testing"

func TestGPT61SolValidatorAliasInteractionsLite(t *testing.T) {
	for _, body := range []string{
		`{"model":"public-none"}`,
		`{"model":"public-minimal"}`,
		`{"model":"public-none","reasoning_effort":"high"}`,
	} {
		if err := validateGPT61SolCompatRequest([]byte(body), "gpt-6.1-sol"); err != nil {
			t.Errorf("mapped public alias %s unexpectedly rejected: %v", body, err)
		}
	}
	for _, body := range []string{
		`{"model":"gpt-6.1-sol-none"}`,
		`{"model":"openai/GPT-6.1-SOL-minimal"}`,
	} {
		if err := validateGPT61SolCompatRequest([]byte(body), "gpt-6.1-sol"); err == nil {
			t.Errorf("unsupported canonical effort alias %s accepted", body)
		}
	}
}

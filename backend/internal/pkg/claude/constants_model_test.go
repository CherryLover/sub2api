package claude

import "testing"

func TestDefaultModelsContainsOpus55(t *testing.T) {
	for _, model := range DefaultModels {
		if model.ID == "claude-opus-5-5" {
			if model.DisplayName != "Claude Opus 5.5" || model.CreatedAt != "2026-09-22T00:00:00Z" {
				t.Fatalf("unexpected Opus 5.5 descriptor: %+v", model)
			}
			return
		}
	}
	t.Fatal("claude-opus-5-5 missing")
}

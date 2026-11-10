package antigravity

import (
	"encoding/json"
	"testing"
)

func TestBuildPartsToolResultImageForwarded(t *testing.T) {
	content := json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"text","text":"shot"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}]}]`)
	parts, _, err := buildParts(content, map[string]string{"call_1": "screenshot"}, true)
	if err != nil {
		t.Fatal(err)
	}
	var fn, img int
	for _, item := range parts {
		if item.FunctionResponse != nil {
			fn++
		}
		if item.InlineData != nil {
			img++
			if item.InlineData.MimeType != "image/png" || item.InlineData.Data != "AAAA" {
				t.Fatalf("wrong image %+v", item.InlineData)
			}
		}
	}
	if fn != 1 || img != 1 {
		t.Fatalf("expected tool response and image, got %+v", parts)
	}
}

func TestBuildPartsGemini3KeepsToolMediaWithFunctionResponse(t *testing.T) {
	content := json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_1","content":[
		{"type":"text","text":"shot"},
		{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AQID"}},
		{"type":"image","source":{"type":"base64","media_type":"image/webp","data":"BAUG"}}
	]}]`)
	parts, _, err := buildParts(content, map[string]string{"call_1": "screenshot"}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 1 || parts[0].FunctionResponse == nil {
		t.Fatalf("want exactly one function response, got %+v", parts)
	}
	response := parts[0].FunctionResponse
	if response.Name != "screenshot" || len(response.Parts) != 2 {
		t.Fatalf("want ordered media within screenshot response, got %+v", response)
	}
	if response.Parts[0].InlineData == nil || response.Parts[0].InlineData.MimeType != "image/png" || response.Parts[0].InlineData.Data != "AQID" {
		t.Fatalf("unexpected first image: %+v", response.Parts[0])
	}
	if response.Parts[1].InlineData == nil || response.Parts[1].InlineData.MimeType != "image/webp" || response.Parts[1].InlineData.Data != "BAUG" {
		t.Fatalf("unexpected second image: %+v", response.Parts[1])
	}
}

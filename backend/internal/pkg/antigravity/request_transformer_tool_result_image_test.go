package antigravity

import (
 "encoding/json"
 "testing"
)

func TestBuildPartsToolResultImageForwarded(t *testing.T) {
 content := json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"text","text":"shot"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}]}]`)
 parts, _, err := buildParts(content, map[string]string{"call_1":"screenshot"}, true)
 if err != nil { t.Fatal(err) }
 var fn, img int
 for _, item := range parts {
  if item.FunctionResponse != nil { fn++ }
  if item.InlineData != nil { img++; if item.InlineData.MimeType!="image/png" || item.InlineData.Data!="AAAA" { t.Fatalf("wrong image %+v",item.InlineData) } }
 }
 if fn!=1 || img!=1 { t.Fatalf("expected tool response and image, got %+v",parts) }
}

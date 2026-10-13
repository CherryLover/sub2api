package apicompat

import (
 "encoding/json"
 "testing"
)

func TestAnthropicAssistantToResponsesReasoningSummaryRequired(t *testing.T) {
 raw := json.RawMessage(`[{"type":"thinking","thinking":"","signature":"opaque-sig"},{"type":"text","text":"hi"}]`)
 items, err := anthropicAssistantToResponses(raw)
 if err != nil { t.Fatal(err) }
 if len(items)==0 || items[0].Type!="reasoning" { t.Fatalf("missing reasoning: %+v",items) }
 b,err := json.Marshal(items[0]); if err!=nil { t.Fatal(err) }
 var obj map[string]json.RawMessage
 if err:=json.Unmarshal(b,&obj);err!=nil { t.Fatal(err) }
 if string(obj["summary"])!="[]" { t.Fatalf("expected required summary []: %s",b) }
 if string(obj["encrypted_content"])!=`"opaque-sig"` { t.Fatalf("signature lost: %s",b) }
}

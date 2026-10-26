package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	gocache "github.com/patrickmn/go-cache"
	"github.com/tidwall/gjson"
)

const (
	openAIInvalidEncryptedLineageTTL         = 45 * time.Minute
	openAIInvalidEncryptedLineageMaxSessions = 4096
	openAIInvalidEncryptedLineageMaxDigests  = 64
)

// invalidEncryptedLineageKey is scoped by the authenticated API key, group,
// and session. No caller-provided session value or original encrypted content
// is stored in the cache key.
func (s *OpenAIGatewayService) invalidEncryptedLineageKey(c *gin.Context, body []byte) string {
	if s == nil || c == nil {
		return ""
	}
	value, ok := c.Get("api_key")
	if !ok {
		return ""
	}
	apiKey, ok := value.(*APIKey)
	if !ok || apiKey == nil || apiKey.ID <= 0 {
		return ""
	}
	sessionHash := s.GenerateSessionHash(c, body)
	if sessionHash == "" {
		return ""
	}
	return fmt.Sprintf("%d:%d:%s", apiKey.ID, getOpenAIGroupIDFromContext(c), sessionHash)
}

func openAIInvalidEncryptedDigest(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func collectOpenAIInvalidEncryptedDigests(body []byte) []string {
	input := gjson.GetBytes(body, "input")
	if !input.Exists() {
		return nil
	}
	var digests []string
	inspect := func(item gjson.Result) {
		switch item.Get("type").String() {
		case "reasoning", "compaction", "compaction_summary":
			encrypted := item.Get("encrypted_content")
			if encrypted.Type == gjson.String && encrypted.String() != "" {
				digests = append(digests, openAIInvalidEncryptedDigest(encrypted.String()))
			}
		}
	}
	if input.IsArray() {
		for _, item := range input.Array() {
			inspect(item)
		}
	} else if input.IsObject() {
		inspect(input)
	}
	return digests
}

// Caller holds invalidEncryptedLineageMu while storing immutable digest sets.
func (s *OpenAIGatewayService) rememberOpenAIInvalidEncryptedLineage(key string, digests []string) {
	if s == nil || key == "" || len(digests) == 0 {
		return
	}
	s.invalidEncryptedLineageMu.Lock()
	defer s.invalidEncryptedLineageMu.Unlock()
	cache := s.getOpenAIInvalidEncryptedLineageCache()
	if cache.ItemCount() >= openAIInvalidEncryptedLineageMaxSessions {
		cache.Flush()
	}
	invalid := make(map[string]struct{})
	if saved, ok := cache.Get(key); ok {
		if prior, ok := saved.(map[string]struct{}); ok {
			for digest := range prior {
				invalid[digest] = struct{}{}
			}
		}
	}
	for _, digest := range digests {
		if len(invalid) >= openAIInvalidEncryptedLineageMaxDigests {
			// A bounded session set avoids unbounded attacker-controlled memory.
			invalid = make(map[string]struct{})
		}
		invalid[digest] = struct{}{}
	}
	cache.SetDefault(key, invalid)
	s.invalidEncryptedLineageActive.Store(true)
}

func (s *OpenAIGatewayService) getOpenAIInvalidEncryptedLineageCache() *gocache.Cache {
	s.invalidEncryptedLineageOnce.Do(func() {
		s.invalidEncryptedLineage = gocache.New(openAIInvalidEncryptedLineageTTL, 5*time.Minute)
	})
	return s.invalidEncryptedLineage
}

// stripKnownOpenAIInvalidEncrypted removes only encrypted reasoning/compaction
// entries rejected earlier in this same API-key and session scope. Unknown or
// newly generated encrypted entries are preserved intact.
func (s *OpenAIGatewayService) stripKnownOpenAIInvalidEncrypted(c *gin.Context, body []byte) ([]byte, bool) {
	if s == nil || !s.invalidEncryptedLineageActive.Load() {
		return body, false
	}
	key := s.invalidEncryptedLineageKey(c, body)
	if key == "" {
		return body, false
	}
	saved, ok := s.getOpenAIInvalidEncryptedLineageCache().Get(key)
	if !ok {
		return body, false
	}
	invalid, ok := saved.(map[string]struct{})
	if !ok || len(invalid) == 0 {
		return body, false
	}
	input := gjson.GetBytes(body, "input")
	if !input.Exists() || !strings.Contains(input.Raw, "encrypted_content") {
		return body, false
	}
	var decoded map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return body, false
	}
	replace := func(value any) (any, bool, bool) {
		item, ok := value.(map[string]any)
		if !ok {
			return value, false, true
		}
		kind, _ := item["type"].(string)
		if kind != "reasoning" && kind != "compaction" && kind != "compaction_summary" {
			return value, false, true
		}
		ciphertext, _ := item["encrypted_content"].(string)
		if ciphertext == "" {
			return value, false, true
		}
		if _, known := invalid[openAIInvalidEncryptedDigest(ciphertext)]; !known {
			return value, false, true
		}
		return sanitizeEncryptedReasoningInputItem(item)
	}
	changed := false
	switch items := decoded["input"].(type) {
	case []any:
		filtered := make([]any, 0, len(items))
		for _, item := range items {
			next, itemChanged, keep := replace(item)
			changed = changed || itemChanged
			if keep {
				filtered = append(filtered, next)
			}
		}
		if len(filtered) == 0 {
			delete(decoded, "input")
		} else {
			decoded["input"] = filtered
		}
	case map[string]any:
		next, itemChanged, keep := replace(items)
		changed = itemChanged
		if !keep {
			delete(decoded, "input")
		} else if itemChanged {
			decoded["input"] = next
		}
	}
	if !changed {
		return body, false
	}
	rebuilt, err := json.Marshal(decoded)
	if err != nil {
		return body, false
	}
	return rebuilt, true
}

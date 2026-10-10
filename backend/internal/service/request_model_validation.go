package service

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"strings"

	"github.com/tidwall/gjson"
)

// HasDuplicateTopLevelKey reports whether a JSON object contains the same
// decoded top-level key more than once. Key comparison is case-insensitive to
// match encoding/json struct-field binding semantics. Nested objects are not
// inspected.
func HasDuplicateTopLevelKey(body []byte, key string) bool {
	object := gjson.ParseBytes(body)
	if !object.IsObject() {
		return false
	}
	count := 0
	object.ForEach(func(k, _ gjson.Result) bool {
		if strings.EqualFold(k.String(), key) {
			count++
		}
		return count < 2
	})
	return count > 1
}

// ValidateRequestModelCarriers rejects ambiguous model carriers before model
// routing, rewriting, scheduling, or billing. Invalid JSON is left to the
// endpoint's existing parser so error behavior remains unchanged.
func ValidateRequestModelCarriers(contentType string, body []byte, includeSession bool) error {
	trimmedType := strings.TrimSpace(contentType)
	if strings.HasPrefix(strings.ToLower(trimmedType), "multipart/form-data") {
		_, params, err := mime.ParseMediaType(trimmedType)
		if err != nil {
			return err
		}
		boundary := strings.TrimSpace(params["boundary"])
		if boundary == "" {
			return errors.New("multipart boundary is required")
		}
		reader := multipart.NewReader(bytes.NewReader(body), boundary)
		modelCount, sessionCount := 0, 0
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("read multipart model fields: %w", err)
			}
			if part.FileName() != "" {
				continue
			}
			field := strings.TrimSpace(part.FormName())
			switch {
			case strings.EqualFold(field, "model"):
				modelCount++
				if modelCount > 1 {
					return errors.New("model is specified more than once")
				}
			case includeSession && strings.EqualFold(field, "session"):
				sessionCount++
				if sessionCount > 1 {
					return errors.New("session is specified more than once")
				}
				session, err := io.ReadAll(part)
				if err != nil {
					return fmt.Errorf("read multipart session: %w", err)
				}
				if gjson.ValidBytes(session) && HasDuplicateTopLevelKey(session, "model") {
					return errors.New("session.model is specified more than once")
				}
			}
		}
	}
	if !gjson.ValidBytes(body) {
		return nil
	}
	if HasDuplicateTopLevelKey(body, "model") {
		return errors.New("model is specified more than once")
	}
	if !includeSession {
		return nil
	}
	if HasDuplicateTopLevelKey(body, "session") {
		return errors.New("session is specified more than once")
	}
	var sessionErr error
	gjson.ParseBytes(body).ForEach(func(key, value gjson.Result) bool {
		if strings.EqualFold(key.String(), "session") && HasDuplicateTopLevelKey([]byte(value.Raw), "model") {
			sessionErr = errors.New("session.model is specified more than once")
			return false
		}
		return true
	})
	return sessionErr
}

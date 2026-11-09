package httputil

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	requestBodyReadInitCap    = 512
	requestBodyReadMaxInitCap = 1 << 20
	jsonUTF8BOMLen            = 3
	// maxDecompressedBodySize limits the decompressed request body to 64 MB
	// to prevent decompression bomb attacks.
	maxDecompressedBodySize = 64 << 20
)

// ReadRequestBodyWithPrealloc reads request body with preallocated buffer based
// on content length, transparently decoding any Content-Encoding the upstream
// client used to compress the body (zstd, gzip, deflate).
func ReadRequestBodyWithPrealloc(req *http.Request) ([]byte, error) {
	body, err := readRequestBodyWithBudget(req)
	return body.data, err
}

func readRequestBodyWithBudget(req *http.Request) (requestBodyBuffer, error) {
	if req == nil || req.Body == nil {
		return requestBodyBuffer{}, nil
	}
	if err := req.Context().Err(); err != nil {
		return requestBodyBuffer{}, err
	}
	capHint := requestBodyReadInitCap
	if req.ContentLength > 0 {
		switch {
		case req.ContentLength < int64(requestBodyReadInitCap):
			capHint = requestBodyReadInitCap
		case req.ContentLength > int64(requestBodyReadMaxInitCap):
			capHint = requestBodyReadMaxInitCap
		default:
			capHint = int(req.ContentLength)
		}
	}
	raw, err := readRequestBodyChunksWithBudget(req.Context(), req.Body, capHint, req.ContentLength)
	if err != nil {
		return requestBodyBuffer{}, err
	}
	enc := strings.ToLower(strings.TrimSpace(req.Header.Get("Content-Encoding")))
	if enc == "" || enc == "identity" {
		return raw, nil
	}
	defer raw.release(req.Context())
	decoded, err := decompressRequestBodyWithBudget(req.Context(), enc, raw.data)
	if err != nil {
		return requestBodyBuffer{}, fmt.Errorf("decode Content-Encoding %q: %w", enc, err)
	}
	req.Header.Del("Content-Encoding")
	req.Header.Del("Content-Length")
	req.ContentLength = int64(len(decoded.data))
	return decoded, nil
}

// ReadLenientJSONRequestBodyWithPrealloc reads a request body and normalizes
// JSON string control bytes before strict validation.
func ReadLenientJSONRequestBodyWithPrealloc(req *http.Request, maxNormalizedBytes int64) ([]byte, error) {
	if req == nil {
		return nil, nil
	}
	body, err := readRequestBodyWithBudget(req)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeLenientJSONRequestBody(req.Context(), body.data, maxNormalizedBytes)
	if err != nil || normalized.charged > 0 {
		body.release(req.Context())
	}
	return normalized.data, err
}

func decompressRequestBody(encoding string, raw []byte) ([]byte, error) {
	body, err := decompressRequestBodyWithBudget(context.Background(), encoding, raw)
	return body.data, err
}

// readBoundedDecodedBody consumes one byte past the maximum to distinguish
// a valid maximum-size payload from a compressed bomb. Returning truncated
// content as a successful request would make downstream protocol parsing unsafe.
func readBoundedDecodedBody(reader io.Reader, limit int64) ([]byte, error) {
	if limit < 0 {
		return nil, fmt.Errorf("negative decoded body limit")
	}
	decoded, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(decoded)) > limit {
		return nil, &http.MaxBytesError{Limit: limit}
	}
	return decoded, nil
}

// NormalizeLenientJSONRequestBody escapes raw control bytes that broken
// OpenAI-compatible clients sometimes place inside JSON strings.
func NormalizeLenientJSONRequestBody(body []byte, maxNormalizedBytes int64) ([]byte, error) {
	normalized, err := normalizeLenientJSONRequestBody(context.Background(), body, maxNormalizedBytes)
	return normalized.data, err
}

func normalizeLenientJSONRequestBody(ctx context.Context, body []byte, maxNormalizedBytes int64) (requestBodyBuffer, error) {
	if err := ctx.Err(); err != nil {
		return requestBodyBuffer{}, err
	}
	if maxNormalizedBytes <= 0 {
		maxNormalizedBytes = maxDecompressedBodySize
	}

	body = trimUTF8BOM(body)
	if len(body) == 0 {
		return requestBodyBuffer{data: body}, nil
	}
	if int64(len(body)) > maxNormalizedBytes {
		return requestBodyBuffer{}, &http.MaxBytesError{Limit: maxNormalizedBytes}
	}

	normalizedSize := int64(len(body))
	inString := false
	escaped := false
	for _, b := range body {
		if inString && isJSONControlByte(b) {
			if maxNormalizedBytes-normalizedSize < 5 {
				return requestBodyBuffer{}, &http.MaxBytesError{Limit: maxNormalizedBytes}
			}
			normalizedSize += 5
			escaped = false
			continue
		}

		switch {
		case escaped:
			escaped = false
		case inString && b == '\\':
			escaped = true
		case b == '"':
			inString = !inString
		}
	}
	if err := ctx.Err(); err != nil {
		return requestBodyBuffer{}, err
	}
	if normalizedSize == int64(len(body)) {
		return requestBodyBuffer{data: body}, nil
	}
	if err := reserveRequestBodyMemory(ctx, normalizedSize); err != nil {
		return requestBodyBuffer{}, err
	}
	// Allocate once after counting expansion, so escaped control bytes cannot
	// bypass the ingress budget through append's geometric capacity growth.
	out := make([]byte, 0, int(normalizedSize))
	inString, escaped = false, false
	for _, b := range body {
		if inString && isJSONControlByte(b) {
			out = appendJSONUnicodeEscape(out, b)
			escaped = false
			continue
		}
		switch {
		case escaped:
			escaped = false
		case inString && b == '\\':
			escaped = true
		case b == '"':
			inString = !inString
		}
		out = append(out, b)
	}
	if err := ctx.Err(); err != nil {
		releaseRequestBodyMemory(ctx, normalizedSize)
		return requestBodyBuffer{}, err
	}
	return requestBodyBuffer{data: out, charged: normalizedSize}, nil
}

func trimUTF8BOM(body []byte) []byte {
	if len(body) >= jsonUTF8BOMLen && body[0] == 0xef && body[1] == 0xbb && body[2] == 0xbf {
		return body[jsonUTF8BOMLen:]
	}
	return body
}

func isJSONControlByte(b byte) bool {
	return b < 0x20 || b == 0x7f
}

func appendJSONUnicodeEscape(dst []byte, b byte) []byte {
	const hex = "0123456789abcdef"
	return append(dst, '\\', 'u', '0', '0', hex[b>>4], hex[b&0x0f])
}

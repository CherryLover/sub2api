package service

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"unsafe"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const defaultSSEEventMaxLines = 4096

var errSSETransportLimit = errors.New("upstream SSE transport limit exceeded")

type sseEventLimits struct {
	maxBytes int64
	maxLines int
	bytes    int64
	lines    int
}

func newSSEEventLimits(cfg *config.Config, maxLineSize int) *sseEventLimits {
	if maxLineSize <= 0 {
		maxLineSize = defaultMaxLineSize
	}
	limits := &sseEventLimits{maxBytes: int64(maxLineSize), maxLines: defaultSSEEventMaxLines}
	if cfg != nil {
		if cfg.Gateway.SSEEventMaxBytes > 0 {
			limits.maxBytes = cfg.Gateway.SSEEventMaxBytes
		}
		if cfg.Gateway.SSEEventMaxLines > 0 {
			limits.maxLines = cfg.Gateway.SSEEventMaxLines
		}
	}
	return limits
}

func (l *sseEventLimits) observe(line []byte, wireBytes int) error {
	if len(bytes.TrimRight(line, "\r\n")) == 0 {
		l.bytes, l.lines = 0, 0
		return nil
	}
	if int64(wireBytes) > l.maxBytes-l.bytes || l.lines >= l.maxLines {
		return l.exceeded()
	}
	l.bytes += int64(wireBytes)
	l.lines++
	return nil
}

func (l *sseEventLimits) exceeded() error {
	// Preserve the existing upstream response_too_large error paths. This is
	// an SSE framing limit, independent of JSON/protocol translation.
	return fmt.Errorf("%w: %w: SSE event exceeds %d bytes or %d lines", errSSETransportLimit, bufio.ErrTooLong, l.maxBytes, l.maxLines)
}

// boundedSSELineReader preserves the original bytes (including CRLF and the
// final unterminated line) used by image passthrough. ReadSlice bounds individual
// allocations; oversized/incomplete lines are rejected without forwarding a
// truncated prefix as a successful SSE event.
type boundedSSELineReader struct {
	reader      *bufio.Reader
	maxLineSize int
	limits      *sseEventLimits
}

func newBoundedSSELineReader(reader io.Reader, cfg *config.Config) *boundedSSELineReader {
	maxLineSize := defaultMaxLineSize
	if cfg != nil && cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = cfg.Gateway.MaxLineSize
	}
	return &boundedSSELineReader{reader: bufio.NewReader(reader), maxLineSize: maxLineSize, limits: newSSEEventLimits(cfg, maxLineSize)}
}

func (r *boundedSSELineReader) ReadLine() ([]byte, error) {
	var line []byte
	for {
		part, err := r.reader.ReadSlice('\n')
		if len(part) > r.maxLineSize-len(line) || (len(bytes.TrimRight(part, "\r\n")) != 0 || len(line) != 0) && int64(len(line)+len(part)) > r.limits.maxBytes-r.limits.bytes {
			return nil, r.limits.exceeded()
		}
		line = append(line, part...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if len(line) > 0 {
			if limitErr := r.limits.observe(line, len(line)); limitErr != nil {
				return nil, limitErr
			}
		}
		return line, err
	}
}

// ReadLineString is ReadLine for callers that keep the line as a string.
// ReadLine returns a fresh allocation that nothing else retains, so the line is
// handed over without a second copy.
func (r *boundedSSELineReader) ReadLineString() (string, error) {
	line, err := r.ReadLine()
	return unsafe.String(unsafe.SliceData(line), len(line)), err
}

func (s *AntigravityGatewayService) antigravitySSEConfig() *config.Config {
	if s == nil || s.settingService == nil {
		return nil
	}
	return s.settingService.cfg
}

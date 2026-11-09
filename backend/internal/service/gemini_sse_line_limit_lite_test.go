package service

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestBoundedGeminiSSEResponseLineLimit(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: 128}}
	reader := newBoundedSSELineReader(strings.NewReader("data: "+strings.Repeat("x", 512)), cfg)
	line, err := reader.ReadLineString()
	if !errors.Is(err, bufio.ErrTooLong) || line != "" {
		t.Fatalf("oversized SSE line must not forward prefix: line=%q err=%v", line, err)
	}
	_, _, err = collectGeminiSSE(cfg, strings.NewReader("data: "+strings.Repeat("x", 512)), false)
	if !errors.Is(err, bufio.ErrTooLong) {
		t.Fatalf("collector must propagate SSE size limit: %v", err)
	}
}

func TestBoundedGeminiSSEEventLineCountAndEOF(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{
		MaxLineSize: 4096,
		SSEEventMaxLines: 2,
	}}
	reader := newBoundedSSELineReader(strings.NewReader("data: 1\ndata: 2\ndata: 3\n"), cfg)
	for i := 0; i < 2; i++ {
		line, err := reader.ReadLineString()
		if err != nil || line == "" {
			t.Fatalf("line %d: %q, %v", i, line, err)
		}
	}
	line, err := reader.ReadLineString()
	if !errors.Is(err, bufio.ErrTooLong) || line != "" {
		t.Fatalf("event line limit must reject third line: line=%q err=%v", line, err)
	}

	normal := newBoundedSSELineReader(strings.NewReader("data: ok\n\ndata: final"), cfg)
	for i, want := range []string{"data: ok\n", "\n", "data: final"} {
		got, readErr := normal.ReadLineString()
		if got != want || (readErr != nil && !errors.Is(readErr, io.EOF)) {
			t.Fatalf("normal line %d: got=%q err=%v, want=%q", i, got, readErr, want)
		}
	}
}

func TestBoundedGeminiSSEEventByteLimit(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{
		MaxLineSize: 4096,
		SSEEventMaxBytes: 16,
	}}
	reader := newBoundedSSELineReader(strings.NewReader("data: first\ndata: second\n"), cfg)
	if _, err := reader.ReadLineString(); err != nil {
		t.Fatal(err)
	}
	line, err := reader.ReadLineString()
	if !errors.Is(err, bufio.ErrTooLong) || line != "" {
		t.Fatalf("event byte limit must reject total overflow: line=%q err=%v", line, err)
	}
}

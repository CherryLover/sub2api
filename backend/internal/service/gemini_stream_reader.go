package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

var errGeminiStreamIdle = errors.New("gemini upstream stream idle timeout")

type geminiStreamLine struct {
	text string
	err  error
}
type geminiStreamBody struct {
	io.ReadCloser
	once sync.Once
	err  error
}

func (b *geminiStreamBody) Close() error {
	b.once.Do(func() { b.err = b.ReadCloser.Close() })
	return b.err
}

type geminiStreamActivityReader struct {
	io.Reader
	last *atomic.Int64
}

func (r geminiStreamActivityReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n > 0 {
		r.last.Store(time.Now().UnixNano())
	}
	return n, err
}

// Only the forwarding goroutine owns Gin/the downstream writer. The producer
// owns upstream reads and sends bounded lines through an unbuffered channel.
// Cancellation/idle expiry closes the HTTP body, unblocking a socket read.
type geminiStreamReader struct {
	ctx             context.Context
	c               *gin.Context
	body            *geminiStreamBody
	lines           chan geminiStreamLine
	done            chan struct{}
	once            sync.Once
	last            atomic.Int64
	idle            time.Duration
	keepalive       *time.Ticker
	pending         []geminiStreamLine
	commentsWritten bool
}

func newGeminiStreamReader(c *gin.Context, resp *http.Response, cfg *config.Config) *geminiStreamReader {
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	body := &geminiStreamBody{ReadCloser: resp.Body}
	resp.Body = body
	r := &geminiStreamReader{ctx: ctx, c: c, body: body, lines: make(chan geminiStreamLine), done: make(chan struct{})}
	r.last.Store(time.Now().UnixNano())
	if cfg != nil {
		if cfg.Gateway.StreamDataIntervalTimeout > 0 {
			r.idle = time.Duration(cfg.Gateway.StreamDataIntervalTimeout) * time.Second
		}
		if cfg.Gateway.StreamKeepaliveInterval > 0 {
			r.keepalive = time.NewTicker(time.Duration(cfg.Gateway.StreamKeepaliveInterval) * time.Second)
		}
	}
	go func() {
		defer close(r.lines)
		reader := newBoundedSSELineReader(geminiStreamActivityReader{Reader: body, last: &r.last}, cfg)
		for {
			line, err := reader.ReadLineString()
			select {
			case r.lines <- geminiStreamLine{line, err}:
			case <-r.done:
				return
			}
			if err != nil {
				return
			}
		}
	}()
	return r
}
func (r *geminiStreamReader) Close() {
	r.once.Do(func() {
		close(r.done)
		if r.keepalive != nil {
			r.keepalive.Stop()
		}
		_ = r.body.Close()
	})
}
func (r *geminiStreamReader) writeComment(line string) error {
	if r.c == nil || r.c.Writer == nil {
		return nil
	}
	r.c.Header("Content-Type", "text/event-stream")
	r.c.Header("Cache-Control", "no-cache")
	r.c.Header("X-Accel-Buffering", "no")
	if _, err := io.WriteString(r.c.Writer, line); err != nil {
		return context.Canceled
	}
	r.c.Writer.Flush()
	r.commentsWritten = true
	return nil
}
func (r *geminiStreamReader) readWire() (string, error) {
	if err := r.ctx.Err(); err != nil {
		return "", err
	}
	var idle <-chan time.Time
	var timer *time.Timer
	if r.idle > 0 {
		left := r.idle - time.Since(time.Unix(0, r.last.Load()))
		if left < 0 {
			left = 0
		}
		timer = time.NewTimer(left)
		idle = timer.C
		defer timer.Stop()
	}
	var ping <-chan time.Time
	if r.keepalive != nil {
		ping = r.keepalive.C
	}
	for {
		select {
		case <-r.ctx.Done():
			return "", r.ctx.Err()
		case <-r.done:
			return "", context.Canceled
		case item, ok := <-r.lines:
			if !ok {
				return "", io.EOF
			}
			return item.text, item.err
		case <-idle:
			remaining := r.idle - time.Since(time.Unix(0, r.last.Load()))
			if remaining > 0 {
				timer.Reset(remaining)
				continue
			}
			return "", errGeminiStreamIdle
		case <-ping:
			if err := r.writeComment(": ping\n\n"); err != nil {
				return "", err
			}
		}
	}
}
func (r *geminiStreamReader) ReadLineString() (string, error) {
	if err := r.ctx.Err(); err != nil {
		return "", err
	}
	if len(r.pending) > 0 {
		item := r.pending[0]
		r.pending[0] = geminiStreamLine{}
		r.pending = r.pending[1:]
		return item.text, item.err
	}
	return r.readWire()
}

// Do not emit a protocol's initial success frame for comments, metadata or an
// empty HTTP 200 stream. Real input/usage remains in the prefix so converters
// still see every observation once a meaningful event arrives.
func (s *GeminiMessagesCompatService) prepareGeminiStream(c *gin.Context, resp *http.Response, protocol string) (*geminiStreamReader, *ClaudeUsage, error) {
	r := newGeminiStreamReader(c, resp, s.cfg)
	usage := &ClaudeUsage{}
	prefixBytes := 0
	fail := func(cause error) (*geminiStreamReader, *ClaudeUsage, error) {
		r.Close()
		if r.ctx.Err() != nil {
			return nil, usage, r.ctx.Err()
		}
		if errors.Is(cause, context.Canceled) {
			return nil, usage, cause
		}
		return nil, usage, &UpstreamFailoverError{StatusCode: http.StatusBadGateway, EmptyStream: true, RequestScopedTransient: true, SafeToFailoverAfterWrite: r.commentsWritten, ResponseBody: []byte(`{"error":{"message":"Gemini stream failed before content"}}`)}
	}
	for {
		line, readErr := r.readWire()
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return fail(readErr)
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ":") {
			if err := r.writeComment(line); err != nil {
				return fail(err)
			}
		} else {
			if line != "" {
				r.pending = append(r.pending, geminiStreamLine{line, readErr})
				prefixBytes += len(line)
			}
			payload, hasData := strings.CutPrefix(trimmed, "data:")
			if hasData || strings.HasPrefix(trimmed, "{") {
				if !hasData {
					payload = trimmed
				}
				payload = strings.TrimSpace(payload)
				if gjson.Valid(payload) {
					raw := []byte(payload)
					root := gjson.Parse(payload)
					if v := root.Get("response"); v.IsObject() {
						root = v
						raw = []byte(v.Raw)
					}
					if u := extractGeminiUsage(raw); u != nil {
						usage = u
					}
					if root.Get("error").IsObject() {
						status := int(root.Get("error.code").Int())
						if status < 400 || status > 599 {
							status = http.StatusBadGateway
						}
						r.Close()
						if status >= 500 || status == 429 {
							return nil, usage, &UpstreamFailoverError{StatusCode: status, SafeToFailoverAfterWrite: r.commentsWritten, ResponseBody: []byte(root.Raw)}
						}
						// Permission/content/input failures are not replayed under another
						// identity merely because they arrived inside an HTTP 200 stream.
						writeGeminiPreludeClientError(c, protocol, status)
						return nil, usage, errors.New("gemini upstream rejected stream before content")
					}
					semantic := false
					root.Get("candidates.0.content.parts").ForEach(func(_, part gjson.Result) bool {
						semantic = part.Get("text").String() != "" || part.Get("functionCall").IsObject() || part.Get("inlineData.data").String() != ""
						return !semantic
					})
					reason := geminiStreamTerminalReason(raw)
					if !semantic {
						switch reason {
						case "OTHER", "MALFORMED_FUNCTION_CALL", "MALFORMED_RESPONSE", "UNEXPECTED_TOOL_CALL", "TOO_MANY_TOOL_CALLS":
							return fail(errGeminiStreamIncomplete)
						}
					}
					if semantic || reason != "" {
						return r, usage, nil
					}
				}
			}
			// Only the metadata prelude is capped here; a valid first image/text
			// event still uses the configured SSE line/event limits.
			if prefixBytes > 1<<20 {
				return fail(errGeminiStreamIncomplete)
			}
		}
		if errors.Is(readErr, io.EOF) {
			return fail(errGeminiStreamIncomplete)
		}
	}
}
func writeGeminiPreludeClientError(c *gin.Context, protocol string, status int) {
	const message = "Upstream denied this request"
	payload := gin.H{"error": gin.H{"code": status, "status": "INVALID_ARGUMENT", "message": message}}
	if protocol == "messages" {
		payload = gin.H{"type": "error", "error": gin.H{"type": "invalid_request_error", "message": message}}
	}
	if protocol == "chat" {
		payload = gin.H{"error": gin.H{"type": "invalid_request_error", "message": message}}
	}
	if c.Writer.Written() {
		c.SSEvent("error", payload)
		c.Writer.Flush()
	} else {
		c.JSON(status, payload)
	}
	MarkResponseCommitted(c)
}

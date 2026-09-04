//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// barkTestServer 记录最近一次 /push 请求，并按 handler 返回可配置的响应。
type barkTestServer struct {
	*httptest.Server

	mu         sync.Mutex
	lastPath   string
	lastCT     string
	lastBody   map[string]any
	pushStatus int
	pushBody   string
	pingStatus int
	delay      time.Duration
}

func newBarkTestServer(t *testing.T) *barkTestServer {
	t.Helper()
	s := &barkTestServer{pushStatus: http.StatusOK, pushBody: `{"code":200,"message":"success","timestamp":1}`, pingStatus: http.StatusOK}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		delay := s.delay
		s.mu.Unlock()
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		switch r.URL.Path {
		case "/ping":
			s.mu.Lock()
			status := s.pingStatus
			s.lastPath = r.URL.Path
			s.mu.Unlock()
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"code":200,"message":"pong"}`)
		case "/push":
			raw, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(raw, &payload)
			s.mu.Lock()
			s.lastPath = r.URL.Path
			s.lastCT = r.Header.Get("Content-Type")
			s.lastBody = payload
			status, body := s.pushStatus, s.pushBody
			s.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, body)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *barkTestServer) setPush(status int, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pushStatus, s.pushBody = status, body
}

func (s *barkTestServer) snapshot() (string, string, map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastPath, s.lastCT, s.lastBody
}

func TestBarkNotifier_SendSuccess(t *testing.T) {
	t.Parallel()

	srv := newBarkTestServer(t)
	n := NewBarkNotifier(nil)

	// 末尾多个 / 也应被规范化掉，不会拼出 //push。
	res, err := n.Send(context.Background(), BarkTarget{ServerURL: srv.URL + "//", DeviceKey: "dev-key-123"}, BarkMessage{
		Title: "标题",
		Body:  "正文",
		Group: "sub2api",
		Level: BarkLevelTimeSensitive,
		URL:   "https://example.com/ops",
		Sound: "bell",
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "success", res.Message)
	require.GreaterOrEqual(t, res.Latency, time.Duration(0))

	path, ct, body := srv.snapshot()
	require.Equal(t, "/push", path)
	require.Equal(t, "application/json", ct)
	require.Equal(t, "dev-key-123", body["device_key"])
	require.Equal(t, "正文", body["body"])
	require.Equal(t, "标题", body["title"])
	require.Equal(t, "sub2api", body["group"])
	require.Equal(t, BarkLevelTimeSensitive, body["level"])
	require.Equal(t, "https://example.com/ops", body["url"])
	require.Equal(t, "bell", body["sound"])
}

func TestBarkNotifier_SendOmitsEmptyOptionalFields(t *testing.T) {
	t.Parallel()

	srv := newBarkTestServer(t)
	n := NewBarkNotifier(nil)

	_, err := n.Send(context.Background(), BarkTarget{ServerURL: srv.URL, DeviceKey: "device-key-0001"}, BarkMessage{Body: "only body"})
	require.NoError(t, err)

	_, _, body := srv.snapshot()
	require.Equal(t, "device-key-0001", body["device_key"])
	require.Equal(t, "only body", body["body"])
	for _, key := range []string{"title", "group", "level", "url", "sound"} {
		require.NotContains(t, body, key)
	}
}

func TestBarkNotifier_SendHTTP500(t *testing.T) {
	t.Parallel()

	srv := newBarkTestServer(t)
	srv.setPush(http.StatusInternalServerError, `{"code":500,"message":"push failed: dev-key-123 boom"}`)
	n := NewBarkNotifier(nil)

	_, err := n.Send(context.Background(), BarkTarget{ServerURL: srv.URL, DeviceKey: "dev-key-123"}, BarkMessage{Body: "x"})
	require.Error(t, err)
	var sendErr *BarkSendError
	require.ErrorAs(t, err, &sendErr)
	require.Equal(t, http.StatusInternalServerError, sendErr.StatusCode)
	require.Contains(t, sendErr.Snippet, "push failed")
	// 上游把 device_key 回显在响应里也不能带出去。
	require.NotContains(t, err.Error(), "dev-key-123")
	require.Contains(t, err.Error(), "***")
}

func TestBarkNotifier_SendCodeNot200(t *testing.T) {
	t.Parallel()

	srv := newBarkTestServer(t)
	srv.setPush(http.StatusOK, `{"code":400,"message":"device key is invalid"}`)
	n := NewBarkNotifier(nil)

	_, err := n.Send(context.Background(), BarkTarget{ServerURL: srv.URL, DeviceKey: "device-key-0001"}, BarkMessage{Body: "x"})
	require.Error(t, err)
	var sendErr *BarkSendError
	require.ErrorAs(t, err, &sendErr)
	require.Equal(t, http.StatusOK, sendErr.StatusCode)
	require.Contains(t, sendErr.Snippet, "device key is invalid")
}

func TestBarkNotifier_SendSnippetTruncated(t *testing.T) {
	t.Parallel()

	srv := newBarkTestServer(t)
	srv.setPush(http.StatusBadGateway, strings.Repeat("x", 1000))
	n := NewBarkNotifier(nil)

	_, err := n.Send(context.Background(), BarkTarget{ServerURL: srv.URL, DeviceKey: "device-key-0001"}, BarkMessage{Body: "x"})
	var sendErr *BarkSendError
	require.ErrorAs(t, err, &sendErr)
	require.LessOrEqual(t, len([]rune(sendErr.Snippet)), barkResponseSnippetLimit+1)
}

func TestBarkNotifier_SendTimeout(t *testing.T) {
	t.Parallel()

	srv := newBarkTestServer(t)
	srv.mu.Lock()
	srv.delay = 2 * time.Second
	srv.mu.Unlock()
	n := NewBarkNotifier(&http.Client{Timeout: 100 * time.Millisecond})

	started := time.Now()
	_, err := n.Send(context.Background(), BarkTarget{ServerURL: srv.URL, DeviceKey: "test-device-key"}, BarkMessage{Body: "x"})
	require.Error(t, err)
	var sendErr *BarkSendError
	require.False(t, errors.As(err, &sendErr), "超时是网络错误，不该被包装成上游拒绝")
	require.Less(t, time.Since(started), time.Second)
}

func TestBarkNotifier_SendContextTimeout(t *testing.T) {
	t.Parallel()

	srv := newBarkTestServer(t)
	srv.mu.Lock()
	srv.delay = 2 * time.Second
	srv.mu.Unlock()
	n := NewBarkNotifier(nil)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := n.Send(ctx, BarkTarget{ServerURL: srv.URL, DeviceKey: "test-device-key"}, BarkMessage{Body: "x"})
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestBarkNotifier_SendValidation(t *testing.T) {
	t.Parallel()

	n := NewBarkNotifier(nil)
	_, err := n.Send(context.Background(), BarkTarget{ServerURL: "ftp://x", DeviceKey: "device-key-0001"}, BarkMessage{Body: "x"})
	require.Error(t, err)
	_, err = n.Send(context.Background(), BarkTarget{ServerURL: "https://api.day.app", DeviceKey: ""}, BarkMessage{Body: "x"})
	require.Error(t, err)
	_, err = n.Send(context.Background(), BarkTarget{ServerURL: "https://api.day.app", DeviceKey: "device-key-0001"}, BarkMessage{Body: "  "})
	require.Error(t, err)
}

func TestBarkNotifier_Ping(t *testing.T) {
	t.Parallel()

	srv := newBarkTestServer(t)
	n := NewBarkNotifier(nil)

	require.NoError(t, n.Ping(context.Background(), srv.URL+"/"))
	path, _, _ := srv.snapshot()
	require.Equal(t, "/ping", path)

	srv.mu.Lock()
	srv.pingStatus = http.StatusInternalServerError
	srv.mu.Unlock()
	err := n.Ping(context.Background(), srv.URL)
	require.Error(t, err)
	require.Contains(t, err.Error(), "500")

	require.Error(t, n.Ping(context.Background(), "not a url"))
}

func TestNormalizeBarkServerURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "https://api.day.app", want: "https://api.day.app"},
		{in: "https://api.day.app/", want: "https://api.day.app"},
		{in: "  https://bark.example.com/base/// ", want: "https://bark.example.com/base"},
		{in: "http://10.0.0.2:8080", want: "http://10.0.0.2:8080"},
		{in: "", wantErr: true},
		{in: "api.day.app", wantErr: true},
		{in: "ftp://api.day.app", wantErr: true},
		{in: "https://", wantErr: true},
		{in: "https://api.day.app/?x=1", wantErr: true},
	}
	for _, tc := range cases {
		got, err := NormalizeBarkServerURL(tc.in)
		if tc.wantErr {
			require.Errorf(t, err, "input %q", tc.in)
			continue
		}
		require.NoErrorf(t, err, "input %q", tc.in)
		require.Equal(t, tc.want, got)
	}
}

func TestIsValidBarkLevel(t *testing.T) {
	t.Parallel()

	for _, level := range []string{BarkLevelActive, BarkLevelTimeSensitive, BarkLevelPassive, BarkLevelCritical} {
		require.True(t, IsValidBarkLevel(level))
	}
	for _, level := range []string{"", "Active", "urgent", "time-sensitive"} {
		require.False(t, IsValidBarkLevel(level))
	}
}

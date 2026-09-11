//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestSplitBarkDeviceKeys(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want []string
	}{
		{name: "老配置的单 key 原样返回", in: "device-key-0001", want: []string{"device-key-0001"}},
		{name: "英文逗号", in: "a-key-1,b-key-2,c-key-3", want: []string{"a-key-1", "b-key-2", "c-key-3"}},
		{name: "中文逗号", in: "a-key-1，b-key-2", want: []string{"a-key-1", "b-key-2"}},
		{name: "换行与 CRLF", in: "a-key-1\nb-key-2\r\nc-key-3", want: []string{"a-key-1", "b-key-2", "c-key-3"}},
		{name: "混用分隔符", in: "a-key-1,b-key-2，\nc-key-3", want: []string{"a-key-1", "b-key-2", "c-key-3"}},
		{name: "逐个 trim 并丢掉空段", in: "  a-key-1 , ,, \tb-key-2  ,", want: []string{"a-key-1", "b-key-2"}},
		{name: "去重且保持首次出现顺序", in: "b-key,a-key,b-key,c-key,a-key", want: []string{"b-key", "a-key", "c-key"}},
		{name: "只有分隔符时为空", in: " , ，\n\t ", want: []string{}},
		{name: "空串为空", in: "", want: []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, splitBarkDeviceKeys(tc.in))
		})
	}
}

func TestParseBarkDeviceKeysEnforcesLimit(t *testing.T) {
	t.Parallel()

	atLimit := make([]string, 0, barkMaxDeviceKeys)
	for i := 0; i < barkMaxDeviceKeys; i++ {
		atLimit = append(atLimit, fmt.Sprintf("device-key-%02d", i))
	}

	keys, err := ParseBarkDeviceKeys(strings.Join(atLimit, ","))
	require.NoError(t, err, "刚好到上限应放行")
	require.Equal(t, atLimit, keys)

	_, err = ParseBarkDeviceKeys(strings.Join(append(atLimit, "device-key-99"), ","))
	require.ErrorIs(t, err, ErrBarkDeviceKeyTooMany, "超出上限要报明确的错误")

	// 先去重再数：同一个 key 粘贴三遍不该把人挡在上限外面。
	tripled := append(append(append([]string{}, atLimit...), atLimit...), atLimit...)
	keys, err = ParseBarkDeviceKeys(strings.Join(tripled, "\n"))
	require.NoError(t, err)
	require.Len(t, keys, barkMaxDeviceKeys)
}

func TestMaskBarkDeviceKey(t *testing.T) {
	t.Parallel()

	require.Equal(t, "dev***", MaskBarkDeviceKey("device-key-0001"))
	require.Equal(t, "dev***", MaskBarkDeviceKey("  device-key-0001  "))
	require.Equal(t, "***", MaskBarkDeviceKey("short"), "短 key 整串打码")
	require.Equal(t, "***", MaskBarkDeviceKey(""))

	// 打码片段必须短于 scrub 下限，否则它自己会被当成密钥再抹一遍。
	require.Less(t, barkMaskedKeyPrefixLen, barkSecretScrubMinLen)
}

// TestScrubBarkSecretScrubsEveryDeviceKey 多设备场景的关键安全用例：
// 只要列表里的任何一个 key 漏进错误信息就是事故。
func TestScrubBarkSecretScrubsEveryDeviceKey(t *testing.T) {
	t.Parallel()

	keys := []string{"device-key-0001", "device-key-0002", "device-key-0003"}
	err := errors.New("push failed for device-key-0002 then device-key-0003 (device-key-0001 was fine)")

	scrubbed := scrubBarkSecret(err, keys...)
	for _, key := range keys {
		require.NotContainsf(t, scrubbed.Error(), key, "device_key %q 泄进了错误信息", key)
	}
	require.Equal(t, "push failed for *** then *** (*** was fine)", scrubbed.Error())

	// 一个 key 都没命中时原样返回，保住错误链。
	clean := errors.New("dial tcp: i/o timeout")
	require.Same(t, clean, scrubBarkSecret(clean, keys...))
	require.NoError(t, scrubBarkSecret(nil, keys...))

	// 过短的 key 仍然跳过替换，免得把响应里的普通单词也抹了。
	require.Equal(t, "bad key abc", scrubBarkSecret(errors.New("bad key abc"), "abc").Error())
}

func TestBarkResponseSnippetScrubsEveryDeviceKey(t *testing.T) {
	t.Parallel()

	keys := []string{"device-key-0001", "device-key-0002"}
	snippet := barkResponseSnippet(
		[]byte(`{"code":400,"message":"device-key-0001 and device-key-0002 rejected"}`),
		keys...,
	)
	require.Equal(t, `{"code":400,"message":"*** and *** rejected"}`, snippet)

	// 先抹后截断：key 正好骑在 200 字边界上时，截断在前会剩下半截密钥。
	straddling := strings.Repeat("x", barkResponseSnippetLimit-5) + "device-key-0001 tail"
	snippet = barkResponseSnippet([]byte(straddling), keys...)
	require.NotContains(t, snippet, "devic", "截断不能把 key 切成能辨认的半截")
}

// TestBarkNotifier_SendScrubsOtherDeviceKeysFromUpstreamEcho 上游把"别的设备"的 key
// 回显在响应里时，同样不能带出去——多设备下这才是最容易漏的口子。
func TestBarkNotifier_SendScrubsOtherDeviceKeysFromUpstreamEcho(t *testing.T) {
	t.Parallel()

	other := "device-key-0002"
	srv := newBarkTestServer(t)
	srv.setPush(http.StatusBadRequest, `{"code":400,"message":"`+other+` is not registered"}`)
	n := NewBarkNotifier(nil)

	_, err := n.Send(
		context.Background(),
		BarkTarget{ServerURL: srv.URL, DeviceKey: "device-key-0001"},
		BarkMessage{Body: "x"},
	)
	require.Error(t, err)

	// Send 只认识自己这一个 key，兜底靠服务层用整份列表再抹一遍。
	var sendErr *BarkSendError
	require.ErrorAs(t, err, &sendErr)
	require.NotContains(
		t,
		scrubBarkSecretText(sendErr.Error(), []string{"device-key-0001", other}),
		other,
	)
}

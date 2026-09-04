package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Bark 推送通道（https://github.com/Finb/bark-server V2 API）。
//
// 邮件体系在裁剪批次 3 整删之后，系统没有任何主动推送出口；Bark 是补回来的第一条通道，
// 目前只接运维告警引擎（OpsAlertEvaluatorService）的触发 / 恢复两类事件。
// 协议：POST {server_url}/push，JSON 体里 device_key 与 body 必填；探活 GET {server_url}/ping。
// 成功响应形如 {"code":200,"message":"success"}；HTTP 非 2xx 或 code 非 200 都算失败。

const (
	BarkLevelActive        = "active"
	BarkLevelTimeSensitive = "timeSensitive"
	BarkLevelPassive       = "passive"
	BarkLevelCritical      = "critical"

	// barkHTTPTimeout 单次 push 的整体超时；Bark 服务端通常几百毫秒内返回。
	barkHTTPTimeout = 10 * time.Second
	// barkPingTimeout 探活超时；探活失败不阻断 push，只记入测试结果。
	barkPingTimeout = 5 * time.Second
	// barkResponseSnippetLimit 错误信息里带回的上游响应片段上限（字符）。
	barkResponseSnippetLimit = 200
	// barkResponseReadLimit 读取上游响应体的上限，防止异常服务端灌大包。
	barkResponseReadLimit = 64 << 10
	// barkSecretScrubMinLen 短于此长度的 device_key 不做文本替换：真实 key 约 22 位，
	// 太短的串会把响应里的普通单词（如 "key"）也抹掉，反而让错误信息失真。
	barkSecretScrubMinLen = 6
)

// BarkMessage 一条待推送的通知；device_key 与服务器地址由 BarkTarget 单独携带，
// 避免消息体被日志 / 错误信息顺手打印出来时把密钥带出去。
type BarkMessage struct {
	Title string
	Body  string
	Group string
	Level string
	URL   string
	Sound string
}

// BarkTarget 推送目标：服务器地址（已规范化、无末尾 /）与解密后的 device_key。
type BarkTarget struct {
	ServerURL string
	DeviceKey string
}

// BarkSendResult push 成功时的上游回执。
type BarkSendResult struct {
	StatusCode int
	Message    string
	Latency    time.Duration
}

// BarkSendError push 被上游拒绝（HTTP 非 2xx 或响应 code 非 200）。
// Snippet 已截断且已抹掉 device_key，可以直接放进对外错误信息。
type BarkSendError struct {
	StatusCode int
	Snippet    string
}

func (e *BarkSendError) Error() string {
	if e == nil {
		return "bark push failed"
	}
	if e.Snippet == "" {
		return fmt.Sprintf("bark push failed (status %d)", e.StatusCode)
	}
	return fmt.Sprintf("bark push failed (status %d): %s", e.StatusCode, e.Snippet)
}

// BarkSender 是 BarkNotificationService 依赖的发送面；测试里用假实现替换 HTTP 客户端。
type BarkSender interface {
	Send(ctx context.Context, target BarkTarget, msg BarkMessage) (*BarkSendResult, error)
	Ping(ctx context.Context, serverURL string) error
}

// BarkNotifier 直连 Bark 服务器的 HTTP 客户端。刻意不走上游代理池：
// Bark 服务器是站长自己的地址，不该被账号代理设置牵着走。
type BarkNotifier struct {
	client *http.Client
}

// NewBarkNotifier 构造推送客户端；client 为 nil 时使用 10 秒超时的独立客户端。
func NewBarkNotifier(client *http.Client) *BarkNotifier {
	if client == nil {
		client = &http.Client{Timeout: barkHTTPTimeout}
	}
	return &BarkNotifier{client: client}
}

// IsValidBarkLevel 报告 level 是否为 Bark 支持的四个枚举值之一。
func IsValidBarkLevel(level string) bool {
	switch level {
	case BarkLevelActive, BarkLevelTimeSensitive, BarkLevelPassive, BarkLevelCritical:
		return true
	default:
		return false
	}
}

// NormalizeBarkServerURL 校验并规范化服务器地址：必须是 http/https 绝对地址，去掉末尾的 /。
func NormalizeBarkServerURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("server_url is required")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("server_url is not a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("server_url must start with http:// or https://")
	}
	if u.Host == "" {
		return "", errors.New("server_url must be an absolute URL with a host")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("server_url must not contain query or fragment")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	return u.String(), nil
}

// Send 推送一条通知。device_key 只出现在请求体里；调用方打日志时只应记录 server_url 与状态码。
func (n *BarkNotifier) Send(ctx context.Context, target BarkTarget, msg BarkMessage) (*BarkSendResult, error) {
	if n == nil || n.client == nil {
		return nil, errors.New("bark notifier not initialized")
	}
	serverURL, err := NormalizeBarkServerURL(target.ServerURL)
	if err != nil {
		return nil, err
	}
	deviceKey := strings.TrimSpace(target.DeviceKey)
	if deviceKey == "" {
		return nil, errors.New("device_key is required")
	}
	body := strings.TrimSpace(msg.Body)
	if body == "" {
		return nil, errors.New("message body is required")
	}

	payload := map[string]any{
		"device_key": deviceKey,
		"body":       body,
	}
	if title := strings.TrimSpace(msg.Title); title != "" {
		payload["title"] = title
	}
	if group := strings.TrimSpace(msg.Group); group != "" {
		payload["group"] = group
	}
	if level := strings.TrimSpace(msg.Level); level != "" {
		payload["level"] = level
	}
	if link := strings.TrimSpace(msg.URL); link != "" {
		payload["url"] = link
	}
	if sound := strings.TrimSpace(msg.Sound); sound != "" {
		payload["sound"] = sound
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal bark payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/push", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build bark request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	startedAt := time.Now()
	resp, err := n.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bark push request failed: %w", scrubBarkSecret(err, deviceKey))
	}
	defer func() { _ = resp.Body.Close() }()
	latency := time.Since(startedAt)

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, barkResponseReadLimit))
	snippet := barkResponseSnippet(respBody, deviceKey)

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &BarkSendError{StatusCode: resp.StatusCode, Snippet: snippet}
	}

	var parsed struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, &BarkSendError{StatusCode: resp.StatusCode, Snippet: snippet}
	}
	if parsed.Code != http.StatusOK {
		return nil, &BarkSendError{StatusCode: resp.StatusCode, Snippet: snippet}
	}
	message := strings.TrimSpace(parsed.Message)
	if message == "" {
		message = "success"
	}
	return &BarkSendResult{StatusCode: resp.StatusCode, Message: message, Latency: latency}, nil
}

// Ping 探活 GET {server_url}/ping；非 2xx 视为失败。
func (n *BarkNotifier) Ping(ctx context.Context, serverURL string) error {
	if n == nil || n.client == nil {
		return errors.New("bark notifier not initialized")
	}
	normalized, err := NormalizeBarkServerURL(serverURL)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, normalized+"/ping", nil)
	if err != nil {
		return fmt.Errorf("build bark ping request: %w", err)
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("bark ping request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, barkResponseReadLimit))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("bark ping returned status %d", resp.StatusCode)
	}
	return nil
}

// barkResponseSnippet 把上游响应压成一行、截断到 200 字，并抹掉 device_key。
func barkResponseSnippet(body []byte, deviceKey string) string {
	s := strings.TrimSpace(string(body))
	s = strings.Join(strings.Fields(s), " ")
	if len(deviceKey) >= barkSecretScrubMinLen {
		s = strings.ReplaceAll(s, deviceKey, "***")
	}
	runes := []rune(s)
	if len(runes) > barkResponseSnippetLimit {
		s = string(runes[:barkResponseSnippetLimit]) + "…"
	}
	return s
}

// scrubBarkSecret 防御性处理：http 客户端错误一般只含 URL，但仍确保 device_key 不会随错误外泄。
func scrubBarkSecret(err error, deviceKey string) error {
	if err == nil || len(deviceKey) < barkSecretScrubMinLen {
		return err
	}
	text := err.Error()
	if !strings.Contains(text, deviceKey) {
		return err
	}
	return errors.New(strings.ReplaceAll(text, deviceKey, "***"))
}

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
//
// 这一层刻意只负责"发一条给一个设备"：多设备是配置层的概念（一串逗号分隔的 device_key），
// 由 BarkNotificationService 拆开后逐个调 Send，HTTP 客户端本身不需要知道有几个人要收。

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

	// barkMaxDeviceKeys 一份配置里允许的 device_key 数量上限。设上限纯粹是防呆：
	// 站长很容易整段粘贴一大片文本进来，而每个 key 都要独立发一次 HTTP 请求。
	barkMaxDeviceKeys = 10
	// barkMaskedKeyPrefixLen 打码后保留的前缀长度。只留 3 位，远小于 barkSecretScrubMinLen，
	// 这样打码片段自己不会反过来被 scrub 逻辑当成密钥再抹一遍。
	barkMaskedKeyPrefixLen = 3
	// barkMaskMinLen 短于此长度的 key 整串打码：真实 key 约 22 位，更短的多半是测试值，
	// 留前缀既没有辨识价值又白白多露几位。
	barkMaskMinLen = 8
	// barkDeviceKeySeparator 规范化存储时用的分隔符。落库前统一拼成这个格式，
	// 读出来再切开，历史上的单 key 配置就是"没有分隔符"的特例，天然兼容。
	barkDeviceKeySeparator = ","
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

// isBarkDeviceKeySeparator 报告 r 是否是 device_key 列表的分隔符。
//
// 约定的写法是英文逗号，但同时接受中文逗号与换行：站长多半是从聊天记录、备忘录里
// 一段一段粘过来的，输入法留下的 "，" 和粘贴带来的换行是常态而不是意外，
// 与其报错不如直接认下来。
func isBarkDeviceKeySeparator(r rune) bool {
	switch r {
	case ',', '，', '\n', '\r':
		return true
	default:
		return false
	}
}

// splitBarkDeviceKeys 把一串 device_key 切成列表：按分隔符切分后逐个 trim、丢掉空串、
// 按首次出现顺序去重（重复的 key 会让同一台手机连收两条，纯属噪音）。
//
// 这里刻意不校验数量上限，读取路径（解密已存配置）用它：历史数据不该因为超限而整串读不出来，
// 那样等于配置突然"消失"。上限只在写入路径由 ParseBarkDeviceKeys 把关。
func splitBarkDeviceKeys(raw string) []string {
	fields := strings.FieldsFunc(raw, isBarkDeviceKeySeparator)
	keys := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		key := strings.TrimSpace(field)
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys
}

// ParseBarkDeviceKeys 在 splitBarkDeviceKeys 之上加数量上限，供所有写入路径使用
// （保存配置、测试推送时请求体里现填的值）。超限时返回 ErrBarkDeviceKeyTooMany，
// 该错误与其它 Bark 错误一起定义在 notify_bark_settings.go，管理端接口会直接映射成 400。
func ParseBarkDeviceKeys(raw string) ([]string, error) {
	keys := splitBarkDeviceKeys(raw)
	if len(keys) > barkMaxDeviceKeys {
		return nil, ErrBarkDeviceKeyTooMany
	}
	return keys, nil
}

// JoinBarkDeviceKeys 把列表拼回落库用的规范格式（去重、去空白后的逗号分隔串）。
func JoinBarkDeviceKeys(keys []string) string {
	return strings.Join(keys, barkDeviceKeySeparator)
}

// MaskBarkDeviceKey 把 device_key 压成可以安全出现在日志 / 接口返回里的片段。
//
// 只保留前 3 位：定位"是第几个设备失败了"靠的是序号，前缀只是让站长对着自己填的顺序
// 再确认一眼，不承担"能认出这是谁的 key"的职责，所以露得越少越好。
func MaskBarkDeviceKey(key string) string {
	runes := []rune(strings.TrimSpace(key))
	if len(runes) < barkMaskMinLen {
		return "***"
	}
	return string(runes[:barkMaskedKeyPrefixLen]) + "***"
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

// barkResponseSnippet 把上游响应压成一行、截断到 200 字，并抹掉所有已知 device_key。
// 先抹再截断：反过来的话跨越截断边界的 key 会被切成两半，躲过替换后半截仍然泄露。
func barkResponseSnippet(body []byte, deviceKeys ...string) string {
	s := strings.TrimSpace(string(body))
	s = strings.Join(strings.Fields(s), " ")
	s = scrubBarkSecretText(s, deviceKeys)
	runes := []rune(s)
	if len(runes) > barkResponseSnippetLimit {
		s = string(runes[:barkResponseSnippetLimit]) + "…"
	}
	return s
}

// scrubBarkSecretText 把文本里出现过的每一个 device_key 换成 ***。
//
// 多设备场景下必须整份列表一起抹，只抹"当次用的那一个"是不够的：上游回显、代理错误信息
// 里完全可能带上别的设备的 key，漏出去任意一个都是事故。
func scrubBarkSecretText(text string, deviceKeys []string) string {
	for _, key := range deviceKeys {
		key = strings.TrimSpace(key)
		// 短 key 跳过替换，理由见 barkSecretScrubMinLen。
		if len(key) < barkSecretScrubMinLen {
			continue
		}
		text = strings.ReplaceAll(text, key, "***")
	}
	return text
}

// scrubBarkSecret 防御性处理：http 客户端错误一般只含 URL，但仍确保 device_key 不会随错误外泄。
// 变参形式让调用方一次把整份 key 列表传进来，单 key 的老调用点写法不变。
func scrubBarkSecret(err error, deviceKeys ...string) error {
	if err == nil {
		return err
	}
	text := err.Error()
	scrubbed := scrubBarkSecretText(text, deviceKeys)
	if scrubbed == text {
		return err
	}
	return errors.New(scrubbed)
}

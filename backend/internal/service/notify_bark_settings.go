package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// Bark 通知设置（批次 6 / A6-2 第一步）。
//
// 存储照备份 S3 配置三件套的模式：整段 JSON 存 settings 表的 notify_bark_config 键，
// device_key 字段落库前用 SecretEncryptor 加密；读取接口永远不回显 device_key，
// 只用 has_device_key 表示"已配置"。该键只走管理端接口，绝不进 /api/v1/settings/public。

const (
	settingKeyNotifyBarkConfig = "notify_bark_config"

	barkDefaultGroup = "sub2api"
	// barkConfigCacheTTL 评估器每轮都要判断"Bark 是否启用"，配置带 30 秒短缓存避免每次查库。
	barkConfigCacheTTL = 30 * time.Second
	// barkNotifyTimeout 告警出口单次推送的超时上限，评估流程不会被慢服务器拖住。
	barkNotifyTimeout = 10 * time.Second

	barkTestDefaultTitle = "Sub2API 测试通知"
	barkTimeLayout       = "2006-01-02 15:04:05"
	// barkTestNoDeviceKeyMessage 测试接口在没有任何 device_key 时只探活不推送，用这句话告诉前端。
	barkTestNoDeviceKeyMessage = "未配置设备 Key，仅测试了服务器连通性"
)

var (
	ErrBarkLevelInvalid                 = infraerrors.BadRequest("BARK_LEVEL_INVALID", "level must be one of: active, timeSensitive, passive, critical")
	ErrBarkDeviceKeyRequiredWhenEnabled = infraerrors.BadRequest("BARK_DEVICE_KEY_REQUIRED", "device_key is required when Bark notification is enabled: provide one in the request or save it first")
	ErrBarkConfigCorrupt                = infraerrors.InternalServer("BARK_CONFIG_CORRUPT", "bark notification config data is corrupted")

	// ErrBarkEncryptionKeyNotConfigured 与备份 S3 密钥同一条护栏（#4524）：
	// 自动生成的加密密钥每次重启都变，落库的 device_key 密文会在重启后解不开。
	ErrBarkEncryptionKeyNotConfigured = infraerrors.BadRequest(
		"SECRET_ENCRYPTION_KEY_NOT_CONFIGURED",
		"cannot store the Bark device key: no fixed secret encryption key is configured, so the auto-generated key would change on every restart and make the stored device key undecryptable. Set a fixed TOTP_ENCRYPTION_KEY (e.g. generate one with `openssl rand -hex 32`) and try again",
	)
)

// BarkConfig 落库结构；DeviceKey 字段存的是密文。
type BarkConfig struct {
	Enabled         bool      `json:"enabled"`
	ServerURL       string    `json:"server_url"`
	DeviceKey       string    `json:"device_key"`
	Group           string    `json:"group"`
	Level           string    `json:"level"`
	Sound           string    `json:"sound"`
	ClickURL        string    `json:"click_url"`
	NotifyOnResolve bool      `json:"notify_on_resolve"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// BarkConfigView 管理端 GET / PUT 的返回结构：device_key 永远为空串，用 has_device_key 表示已配置。
type BarkConfigView struct {
	Enabled         bool       `json:"enabled"`
	ServerURL       string     `json:"server_url"`
	DeviceKey       string     `json:"device_key"`
	HasDeviceKey    bool       `json:"has_device_key"`
	Group           string     `json:"group"`
	Level           string     `json:"level"`
	Sound           string     `json:"sound"`
	ClickURL        string     `json:"click_url"`
	NotifyOnResolve bool       `json:"notify_on_resolve"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

// BarkConfigInput 管理端 PUT 的请求体。NotifyOnResolve 缺省（nil）按默认值 true 处理。
type BarkConfigInput struct {
	Enabled         bool   `json:"enabled"`
	ServerURL       string `json:"server_url"`
	DeviceKey       string `json:"device_key"`
	Group           string `json:"group"`
	Level           string `json:"level"`
	Sound           string `json:"sound"`
	ClickURL        string `json:"click_url"`
	NotifyOnResolve *bool  `json:"notify_on_resolve"`
}

// BarkTestInput 测试推送的请求体：配置同 PUT，另可指定标题与正文。
type BarkTestInput struct {
	BarkConfigInput
	Title string `json:"title"`
	Body  string `json:"body"`
}

// BarkTestResult 测试推送的返回。
type BarkTestResult struct {
	OK         bool   `json:"ok"`
	PingOK     bool   `json:"ping_ok"`
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	LatencyMs  int64  `json:"latency_ms"`
}

// OpsAlertNotification 评估器交给推送通道的一条告警摘要（触发与恢复共用）。
type OpsAlertNotification struct {
	RuleName   string
	Severity   string
	MetricType string
	Operator   string
	Threshold  float64
	Value      float64
	Scope      string
	FiredAt    time.Time
	ResolvedAt *time.Time
}

// BarkNotificationService 读写 Bark 配置、提供测试推送，并作为运维告警的推送出口。
type BarkNotificationService struct {
	settingRepo             SettingRepository
	encryptor               SecretEncryptor
	sender                  BarkSender
	encryptionKeyConfigured bool

	now func() time.Time

	cacheMu  sync.Mutex
	cached   *BarkConfig // 解密后的运行时配置；nil 表示未配置或不可用
	cachedAt time.Time
}

// NewBarkNotificationService 构造 Bark 设置服务。encryptionKeyConfigured 为假时拒绝保存新的 device_key。
func NewBarkNotificationService(settingRepo SettingRepository, encryptor SecretEncryptor, sender BarkSender, encryptionKeyConfigured bool) *BarkNotificationService {
	return &BarkNotificationService{
		settingRepo:             settingRepo,
		encryptor:               encryptor,
		sender:                  sender,
		encryptionKeyConfigured: encryptionKeyConfigured,
		now:                     time.Now,
	}
}

func defaultBarkConfig() *BarkConfig {
	return &BarkConfig{
		Group:           barkDefaultGroup,
		Level:           BarkLevelActive,
		NotifyOnResolve: true,
	}
}

// ─── 管理端接口 ───

// GetBarkConfig 返回脱敏后的配置；从未保存过时返回默认值。
func (s *BarkNotificationService) GetBarkConfig(ctx context.Context) (*BarkConfigView, error) {
	stored, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return toBarkConfigView(defaultBarkConfig()), nil
	}
	return toBarkConfigView(stored), nil
}

// UpdateBarkConfig 保存配置：device_key 为空则保留已存的密文，非空则加密后覆盖。
func (s *BarkNotificationService) UpdateBarkConfig(ctx context.Context, in BarkConfigInput) (*BarkConfigView, error) {
	next, err := normalizeBarkConfigInput(in, false)
	if err != nil {
		return nil, err
	}

	// 旧配置读不出来（含 JSON 损坏）时按"没有旧值"处理，保证坏数据能被这次保存覆盖掉。
	old, _ := s.load(ctx)

	deviceKey := strings.TrimSpace(in.DeviceKey)
	switch {
	case deviceKey != "":
		if !s.encryptionKeyConfigured {
			return nil, ErrBarkEncryptionKeyNotConfigured
		}
		encrypted, err := s.encryptor.Encrypt(deviceKey)
		if err != nil {
			return nil, fmt.Errorf("encrypt bark device key: %w", err)
		}
		next.DeviceKey = encrypted
	case old != nil:
		next.DeviceKey = old.DeviceKey
	}

	if next.Enabled && next.DeviceKey == "" {
		return nil, ErrBarkDeviceKeyRequiredWhenEnabled
	}

	next.UpdatedAt = s.now().UTC()
	data, err := json.Marshal(next)
	if err != nil {
		return nil, fmt.Errorf("marshal bark config: %w", err)
	}
	if err := s.settingRepo.Set(ctx, settingKeyNotifyBarkConfig, string(data)); err != nil {
		return nil, fmt.Errorf("save bark config: %w", err)
	}
	s.invalidateCache()
	return toBarkConfigView(next), nil
}

// TestBark 用请求体里的配置直接发一条测试通知：先探活（失败不阻断），再 push。
// 请求里没带 device_key、库里也没存时不算错误：只做探活，返回 ok=false + ping_ok，
// 让前端能在还没填 Key 的阶段就验证服务器地址是否可达。
func (s *BarkNotificationService) TestBark(ctx context.Context, in BarkTestInput) (*BarkTestResult, error) {
	cfg, err := normalizeBarkConfigInput(in.BarkConfigInput, true)
	if err != nil {
		return nil, err
	}
	if s.sender == nil {
		return nil, errors.New("bark sender not initialized")
	}

	deviceKey := strings.TrimSpace(in.DeviceKey)
	if deviceKey == "" {
		deviceKey = s.storedDeviceKey(ctx)
	}
	if deviceKey == "" {
		pingOK, pingLatency := s.ping(ctx, cfg.ServerURL)
		return &BarkTestResult{
			OK:         false,
			PingOK:     pingOK,
			StatusCode: 0,
			Message:    barkTestNoDeviceKeyMessage,
			LatencyMs:  pingLatency.Milliseconds(),
		}, nil
	}

	result := &BarkTestResult{}
	result.PingOK, _ = s.ping(ctx, cfg.ServerURL)

	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = barkTestDefaultTitle
	}
	body := strings.TrimSpace(in.Body)
	if body == "" {
		body = fmt.Sprintf("这是一条来自 Sub2API 的测试通知。\n时间：%s\n服务器：%s", formatBarkTime(s.now()), cfg.ServerURL)
	}

	sendCtx, cancelSend := context.WithTimeout(ctx, barkHTTPTimeout)
	defer cancelSend()
	sent, err := s.sender.Send(sendCtx, BarkTarget{ServerURL: cfg.ServerURL, DeviceKey: deviceKey}, BarkMessage{
		Title: title,
		Body:  body,
		Group: cfg.Group,
		Level: cfg.Level,
		URL:   cfg.ClickURL,
		Sound: cfg.Sound,
	})
	if err != nil {
		var sendErr *BarkSendError
		if errors.As(err, &sendErr) {
			slog.Warn("bark_test_push_rejected", "server_url", cfg.ServerURL, "status_code", sendErr.StatusCode)
			return nil, infraerrors.New(http.StatusBadGateway, "BARK_PUSH_FAILED", sendErr.Error())
		}
		slog.Warn("bark_test_push_failed", "server_url", cfg.ServerURL, "error", err)
		return nil, infraerrors.New(http.StatusBadGateway, "BARK_PUSH_FAILED", "bark push failed: "+scrubBarkSecret(err, deviceKey).Error())
	}

	result.OK = true
	result.StatusCode = sent.StatusCode
	result.Message = sent.Message
	result.LatencyMs = sent.Latency.Milliseconds()
	return result, nil
}

// ping 探活一次，返回是否成功与耗时；失败只记日志，不向上返回错误。
func (s *BarkNotificationService) ping(ctx context.Context, serverURL string) (bool, time.Duration) {
	pingCtx, cancel := context.WithTimeout(ctx, barkPingTimeout)
	defer cancel()
	startedAt := time.Now()
	err := s.sender.Ping(pingCtx, serverURL)
	elapsed := time.Since(startedAt)
	if err != nil {
		slog.Warn("bark_test_ping_failed", "server_url", serverURL, "error", err)
		return false, elapsed
	}
	return true, elapsed
}

// ─── 告警出口 ───

// NotifyOpsAlertFired 告警触发时推一条；Bark 未启用或配置不可用时静默返回 nil。
func (s *BarkNotificationService) NotifyOpsAlertFired(ctx context.Context, n OpsAlertNotification) error {
	cfg, ok := s.runtimeConfig(ctx)
	if !ok {
		return nil
	}
	title := fmt.Sprintf("[Sub2API] %s %s", strings.TrimSpace(n.Severity), strings.TrimSpace(n.RuleName))
	return s.push(ctx, cfg, strings.TrimSpace(title), buildOpsAlertBarkBody(n, false))
}

// NotifyOpsAlertResolved 告警解除时按 notify_on_resolve 开关推一条「已恢复」。
func (s *BarkNotificationService) NotifyOpsAlertResolved(ctx context.Context, n OpsAlertNotification) error {
	cfg, ok := s.runtimeConfig(ctx)
	if !ok || !cfg.NotifyOnResolve {
		return nil
	}
	title := fmt.Sprintf("[Sub2API] 已恢复 %s", strings.TrimSpace(n.RuleName))
	return s.push(ctx, cfg, strings.TrimSpace(title), buildOpsAlertBarkBody(n, true))
}

func (s *BarkNotificationService) push(ctx context.Context, cfg *BarkConfig, title, body string) error {
	if s == nil || s.sender == nil {
		return errors.New("bark sender not initialized")
	}
	sendCtx, cancel := context.WithTimeout(ctx, barkNotifyTimeout)
	defer cancel()
	_, err := s.sender.Send(sendCtx, BarkTarget{ServerURL: cfg.ServerURL, DeviceKey: cfg.DeviceKey}, BarkMessage{
		Title: title,
		Body:  body,
		Group: cfg.Group,
		Level: cfg.Level,
		URL:   cfg.ClickURL,
		Sound: cfg.Sound,
	})
	if err != nil {
		return scrubBarkSecret(err, cfg.DeviceKey)
	}
	return nil
}

func buildOpsAlertBarkBody(n OpsAlertNotification, resolved bool) string {
	scope := strings.TrimSpace(n.Scope)
	if scope == "" {
		scope = "全局"
	}
	lines := []string{
		"指标：" + strings.TrimSpace(n.MetricType),
		fmt.Sprintf("当前值：%s（阈值 %s %s）", formatBarkNumber(n.Value), strings.TrimSpace(n.Operator), formatBarkNumber(n.Threshold)),
		"作用域：" + scope,
		"触发时间：" + formatBarkTime(n.FiredAt),
	}
	if resolved && n.ResolvedAt != nil {
		line := "恢复时间：" + formatBarkTime(*n.ResolvedAt)
		if d := n.ResolvedAt.Sub(n.FiredAt); d > 0 {
			line += "，持续 " + formatBarkDuration(d)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// FormatOpsAlertScope 把规则的 filters（platform / group_id / region）压成一行作用域说明；无过滤时返回空串。
func FormatOpsAlertScope(filters map[string]any) string {
	platform, groupID, region := parseOpsAlertRuleScope(filters)
	parts := make([]string, 0, 3)
	if platform != "" {
		parts = append(parts, "platform="+platform)
	}
	if groupID != nil && *groupID > 0 {
		parts = append(parts, fmt.Sprintf("group_id=%d", *groupID))
	}
	if region != nil && *region != "" {
		parts = append(parts, "region="+*region)
	}
	return strings.Join(parts, " ")
}

func formatBarkTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.In(timezone.Location()).Format(barkTimeLayout) + " (" + timezone.Name() + ")"
}

func formatBarkNumber(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "-"
	}
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

func formatBarkDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d 秒", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟", int(d.Minutes()))
	default:
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		if minutes == 0 {
			return fmt.Sprintf("%d 小时", hours)
		}
		return fmt.Sprintf("%d 小时 %d 分钟", hours, minutes)
	}
}

// ─── 内部：加载 / 校验 / 缓存 ───

// load 读取落库的原始配置（device_key 仍是密文）；从未保存过时返回 nil。
func (s *BarkNotificationService) load(ctx context.Context) (*BarkConfig, error) {
	if s == nil || s.settingRepo == nil {
		return nil, nil //nolint:nilnil // no config is a valid state
	}
	raw, err := s.settingRepo.GetValue(ctx, settingKeyNotifyBarkConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return nil, nil //nolint:nilnil // no config is a valid state
		}
		return nil, fmt.Errorf("load bark config: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return nil, nil //nolint:nilnil // no config is a valid state
	}
	cfg := defaultBarkConfig()
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return nil, ErrBarkConfigCorrupt
	}
	if strings.TrimSpace(cfg.Level) == "" {
		cfg.Level = BarkLevelActive
	}
	// 旧数据或手改过的行 group 可能是空串：读出来统一回落默认分组，GET 回显与推送都用得上。
	if strings.TrimSpace(cfg.Group) == "" {
		cfg.Group = barkDefaultGroup
	}
	return cfg, nil
}

// storedDeviceKey 返回已存 device_key 的明文；没有或解不开时返回空串。
func (s *BarkNotificationService) storedDeviceKey(ctx context.Context) string {
	stored, err := s.load(ctx)
	if err != nil || stored == nil || stored.DeviceKey == "" {
		return ""
	}
	if s.encryptor == nil {
		return ""
	}
	plain, err := s.encryptor.Decrypt(stored.DeviceKey)
	if err != nil {
		slog.Warn("bark_device_key_decrypt_failed", "error", err)
		return ""
	}
	return strings.TrimSpace(plain)
}

// runtimeConfig 返回可直接用于推送的配置（device_key 已解密），带 30 秒缓存。
// 第二个返回值为假表示未启用或配置不完整，调用方应直接跳过推送。
func (s *BarkNotificationService) runtimeConfig(ctx context.Context) (*BarkConfig, bool) {
	if s == nil {
		return nil, false
	}
	now := s.now()

	s.cacheMu.Lock()
	if !s.cachedAt.IsZero() && now.Sub(s.cachedAt) < barkConfigCacheTTL {
		cfg := s.cached
		s.cacheMu.Unlock()
		return cfg, cfg != nil
	}
	s.cacheMu.Unlock()

	resolved := s.resolveRuntimeConfig(ctx)

	s.cacheMu.Lock()
	s.cached = resolved
	s.cachedAt = now
	s.cacheMu.Unlock()
	return resolved, resolved != nil
}

func (s *BarkNotificationService) resolveRuntimeConfig(ctx context.Context) *BarkConfig {
	stored, err := s.load(ctx)
	if err != nil {
		slog.Warn("bark_config_load_failed", "error", err)
		return nil
	}
	if stored == nil || !stored.Enabled {
		return nil
	}
	serverURL, err := NormalizeBarkServerURL(stored.ServerURL)
	if err != nil {
		slog.Warn("bark_config_invalid_server_url", "error", err)
		return nil
	}
	deviceKey := s.storedDeviceKey(ctx)
	if deviceKey == "" {
		slog.Warn("bark_config_device_key_unavailable", "server_url", serverURL)
		return nil
	}
	out := *stored
	out.ServerURL = serverURL
	out.DeviceKey = deviceKey
	if !IsValidBarkLevel(out.Level) {
		out.Level = BarkLevelActive
	}
	return &out
}

func (s *BarkNotificationService) invalidateCache() {
	if s == nil {
		return
	}
	s.cacheMu.Lock()
	s.cached = nil
	s.cachedAt = time.Time{}
	s.cacheMu.Unlock()
}

// normalizeBarkConfigInput 校验并规范化请求体（不处理 device_key）。
// requireServerURL 为真（测试推送）时 server_url 必填；保存配置时只有启用才必填。
func normalizeBarkConfigInput(in BarkConfigInput, requireServerURL bool) (*BarkConfig, error) {
	cfg := defaultBarkConfig()
	cfg.Enabled = in.Enabled

	serverURL := strings.TrimSpace(in.ServerURL)
	if serverURL != "" || requireServerURL || in.Enabled {
		normalized, err := NormalizeBarkServerURL(serverURL)
		if err != nil {
			return nil, infraerrors.BadRequest("BARK_SERVER_URL_INVALID", "server_url must be an absolute http(s) URL: "+err.Error())
		}
		serverURL = normalized
	}
	cfg.ServerURL = serverURL

	level := strings.TrimSpace(in.Level)
	if level == "" {
		level = BarkLevelActive
	}
	if !IsValidBarkLevel(level) {
		return nil, ErrBarkLevelInvalid
	}
	cfg.Level = level

	cfg.Group = strings.TrimSpace(in.Group)
	if cfg.Group == "" {
		cfg.Group = barkDefaultGroup
	}
	cfg.Sound = strings.TrimSpace(in.Sound)

	clickURL := strings.TrimSpace(in.ClickURL)
	if clickURL != "" && !strings.HasPrefix(clickURL, "http://") && !strings.HasPrefix(clickURL, "https://") {
		return nil, infraerrors.BadRequest("BARK_CLICK_URL_INVALID", "click_url must start with http:// or https://")
	}
	cfg.ClickURL = clickURL

	if in.NotifyOnResolve != nil {
		cfg.NotifyOnResolve = *in.NotifyOnResolve
	}
	return cfg, nil
}

func toBarkConfigView(cfg *BarkConfig) *BarkConfigView {
	if cfg == nil {
		cfg = defaultBarkConfig()
	}
	view := &BarkConfigView{
		Enabled:         cfg.Enabled,
		ServerURL:       cfg.ServerURL,
		DeviceKey:       "",
		HasDeviceKey:    cfg.DeviceKey != "",
		Group:           cfg.Group,
		Level:           cfg.Level,
		Sound:           cfg.Sound,
		ClickURL:        cfg.ClickURL,
		NotifyOnResolve: cfg.NotifyOnResolve,
	}
	if !cfg.UpdatedAt.IsZero() {
		updatedAt := cfg.UpdatedAt.UTC()
		view.UpdatedAt = &updatedAt
	}
	return view
}

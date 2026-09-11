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
// 只用 has_device_key / device_key_count 表示"已配置几个设备"。
// 该键只走管理端接口，绝不进 /api/v1/settings/public。
//
// 多设备推送（站长要求「一条通知同时推给所有人」）刻意不新增字段、不做数据迁移：
// device_key 仍是一个字符串，只是值允许写成逗号分隔的多个 key，整串照旧加密成一个值。
// 老配置（没有逗号的单 key）就是这个规则的特例，升级后不需要任何处理。

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
	ErrBarkDeviceKeyRequiredWhenEnabled = infraerrors.BadRequest("BARK_DEVICE_KEY_REQUIRED", "device_key is required when Bark notification is enabled: provide one (or a comma-separated list) in the request or save it first")
	ErrBarkConfigCorrupt                = infraerrors.InternalServer("BARK_CONFIG_CORRUPT", "bark notification config data is corrupted")

	// ErrBarkDeviceKeyTooMany 多设备推送的防呆上限，见 barkMaxDeviceKeys。
	// 由 ParseBarkDeviceKeys 抛出，定义在这里是为了和其它 Bark 错误放在同一处。
	ErrBarkDeviceKeyTooMany = infraerrors.BadRequest(
		"BARK_DEVICE_KEY_TOO_MANY",
		fmt.Sprintf("too many device keys: at most %d comma-separated device keys are allowed", barkMaxDeviceKeys),
	)
	// ErrBarkNotEnabled 手动试发要求 Bark 已启用且配置完整；未启用时明确报错而不是静默跳过。
	ErrBarkNotEnabled = infraerrors.BadRequest("BARK_NOT_ENABLED", "Bark notification is not enabled or not fully configured")

	// ErrBarkEncryptionKeyNotConfigured 与备份 S3 密钥同一条护栏（#4524）：
	// 自动生成的加密密钥每次重启都变，落库的 device_key 密文会在重启后解不开。
	ErrBarkEncryptionKeyNotConfigured = infraerrors.BadRequest(
		"SECRET_ENCRYPTION_KEY_NOT_CONFIGURED",
		"cannot store the Bark device key: no fixed secret encryption key is configured, so the auto-generated key would change on every restart and make the stored device key undecryptable. Set a fixed TOTP_ENCRYPTION_KEY (e.g. generate one with `openssl rand -hex 32`) and try again",
	)
)

// BarkConfig 落库结构；DeviceKey 字段存的是密文，明文是一串逗号分隔的 device_key。
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

// BarkConfigView 管理端 GET / PUT 的返回结构：device_key 永远为空串，
// 用 has_device_key 表示已配置、device_key_count 表示配了几个设备。
type BarkConfigView struct {
	Enabled   bool   `json:"enabled"`
	ServerURL string `json:"server_url"`
	DeviceKey string `json:"device_key"`
	// HasDeviceKey 看的是"密文在不在"，DeviceKeyCount 看的是"解密后能切出几个"。
	// 两者一般同进同退；加密密钥被换过导致解不开时会出现 has=true / count=0，
	// 前端据此只显示"已配置"而不报设备数，比直接翻成"未配置"更贴近事实。
	HasDeviceKey    bool       `json:"has_device_key"`
	DeviceKeyCount  int        `json:"device_key_count"`
	Group           string     `json:"group"`
	Level           string     `json:"level"`
	Sound           string     `json:"sound"`
	ClickURL        string     `json:"click_url"`
	NotifyOnResolve bool       `json:"notify_on_resolve"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

// BarkConfigInput 管理端 PUT 的请求体。NotifyOnResolve 缺省（nil）按默认值 true 处理。
// DeviceKey 允许写成逗号分隔的多个设备 Key（也接受中文逗号与换行）。
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

// BarkPushOutcome 单个 device_key 的推送结果。
//
// Index 从 1 开始，按站长填写的顺序计数——"第几个设备失败了"就是靠它定位的；
// MaskedKey 只有前缀 + ***，接口返回与日志里都绝不出现完整 key。
type BarkPushOutcome struct {
	Index      int    `json:"index"`
	MaskedKey  string `json:"masked_key"`
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	LatencyMs  int64  `json:"latency_ms"`
}

// barkPushSummary 一次多设备推送的汇总。
type barkPushSummary struct {
	Total     int
	Succeeded int
	Failed    int
	Outcomes  []BarkPushOutcome
}

// BarkTestResult 测试推送的返回。
//
// OK / StatusCode / Message / LatencyMs 取第一个成功设备的回执，保持单设备时代的语义不变；
// 多设备的全貌看 DeviceCount / SuccessCount / FailureCount 与逐条的 Devices。
// 只要有一个设备收到就算 OK——全部失败才会走 502 错误分支。
type BarkTestResult struct {
	OK         bool   `json:"ok"`
	PingOK     bool   `json:"ping_ok"`
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	LatencyMs  int64  `json:"latency_ms"`

	DeviceCount  int               `json:"device_count"`
	SuccessCount int               `json:"success_count"`
	FailureCount int               `json:"failure_count"`
	Devices      []BarkPushOutcome `json:"devices,omitempty"`
}

// OpsAlertNotification 评估器交给推送通道的一条告警摘要（触发 / 恢复 / 手动试发共用）。
type OpsAlertNotification struct {
	RuleName   string
	Severity   string
	MetricType string
	// MetricLabel 非空时「指标：」行用它代替裸的 MetricType（账号用量类指标给人话名字）。
	MetricLabel string
	Operator    string
	Threshold   float64
	Value       float64
	// Unit 当前值 / 阈值的后缀，如 "%"、" CNY"；为空时格式与旧版完全一致。
	Unit  string
	Scope string
	// Details 插在「指标」与「当前值」之间的补充行（账号用量类规则放账号行）；为空时旧格式不变。
	Details    []string
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
		return toBarkConfigView(defaultBarkConfig(), 0), nil
	}
	return toBarkConfigView(stored, len(s.decryptDeviceKeys(stored.DeviceKey))), nil
}

// UpdateBarkConfig 保存配置：device_key 为空则保留已存的密文，非空则加密后覆盖。
// 请求里的 device_key 可以是逗号分隔的多个设备，规范化（trim / 去空 / 去重 / 查上限）后
// 拼回一串再整体加密，落库仍然只有一个值。
func (s *BarkNotificationService) UpdateBarkConfig(ctx context.Context, in BarkConfigInput) (*BarkConfigView, error) {
	next, err := normalizeBarkConfigInput(in, false)
	if err != nil {
		return nil, err
	}

	// 旧配置读不出来（含 JSON 损坏）时按"没有旧值"处理，保证坏数据能被这次保存覆盖掉。
	old, _ := s.load(ctx)

	// 只填了分隔符（例如 " , , "）解析后是空列表，等同于"这次没提供新 key"，走保留旧值分支；
	// 否则会把一串逗号当成有效密钥存进去。
	deviceKeys, err := ParseBarkDeviceKeys(in.DeviceKey)
	if err != nil {
		return nil, err
	}

	deviceKeyCount := 0
	switch {
	case len(deviceKeys) > 0:
		if !s.encryptionKeyConfigured {
			return nil, ErrBarkEncryptionKeyNotConfigured
		}
		encrypted, err := s.encryptor.Encrypt(JoinBarkDeviceKeys(deviceKeys))
		if err != nil {
			return nil, fmt.Errorf("encrypt bark device key: %w", err)
		}
		next.DeviceKey = encrypted
		deviceKeyCount = len(deviceKeys)
	case old != nil:
		next.DeviceKey = old.DeviceKey
		deviceKeyCount = len(s.decryptDeviceKeys(old.DeviceKey))
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
	return toBarkConfigView(next, deviceKeyCount), nil
}

// TestBark 用请求体里的配置直接发一条测试通知：先探活（失败不阻断），再逐个设备 push。
// 请求里没带 device_key、库里也没存时不算错误：只做探活，返回 ok=false + ping_ok，
// 让前端能在还没填 Key 的阶段就验证服务器地址是否可达。
//
// 配了多个设备时每个都会收到测试通知，逐条结果放在 Devices 里回给前端；
// 只要有一个成功就返回 200（部分失败靠 failure_count 体现），全部失败才是 502。
func (s *BarkNotificationService) TestBark(ctx context.Context, in BarkTestInput) (*BarkTestResult, error) {
	cfg, err := normalizeBarkConfigInput(in.BarkConfigInput, true)
	if err != nil {
		return nil, err
	}
	if s.sender == nil {
		return nil, errors.New("bark sender not initialized")
	}

	deviceKeys, err := s.resolveTestDeviceKeys(ctx, in.DeviceKey)
	if err != nil {
		return nil, err
	}
	if len(deviceKeys) == 0 {
		pingOK, pingLatency := s.ping(ctx, cfg.ServerURL)
		return &BarkTestResult{
			OK:         false,
			PingOK:     pingOK,
			StatusCode: 0,
			Message:    barkTestNoDeviceKeyMessage,
			LatencyMs:  pingLatency.Milliseconds(),
		}, nil
	}

	result := &BarkTestResult{DeviceCount: len(deviceKeys)}
	result.PingOK, _ = s.ping(ctx, cfg.ServerURL)

	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = barkTestDefaultTitle
	}
	body := strings.TrimSpace(in.Body)
	if body == "" {
		body = fmt.Sprintf("这是一条来自 Sub2API 的测试通知。\n时间：%s\n服务器：%s", formatBarkTime(s.now()), cfg.ServerURL)
	}

	summary := s.pushToDevices(ctx, cfg.ServerURL, deviceKeys, BarkMessage{
		Title: title,
		Body:  body,
		Group: cfg.Group,
		Level: cfg.Level,
		URL:   cfg.ClickURL,
		Sound: cfg.Sound,
	}, barkHTTPTimeout)
	result.SuccessCount = summary.Succeeded
	result.FailureCount = summary.Failed
	result.Devices = summary.Outcomes

	if summary.Succeeded == 0 {
		slog.Warn("bark_test_push_failed",
			"server_url", cfg.ServerURL,
			"devices", summary.Total,
			"failed_devices", barkFailedDeviceLabels(summary),
		)
		return nil, infraerrors.New(http.StatusBadGateway, "BARK_PUSH_FAILED", barkFailureMessage(summary))
	}
	if summary.Failed > 0 {
		slog.Warn("bark_test_push_partial",
			"server_url", cfg.ServerURL,
			"succeeded", summary.Succeeded,
			"failed", summary.Failed,
			"failed_devices", barkFailedDeviceLabels(summary),
		)
	}

	// OK / 状态码 / 延迟取第一个成功的设备：单设备配置下与旧版逐字一致。
	first := firstSuccessfulBarkOutcome(summary)
	result.OK = true
	result.StatusCode = first.StatusCode
	result.Message = first.Message
	result.LatencyMs = first.LatencyMs
	return result, nil
}

// resolveTestDeviceKeys 决定这次测试推给哪些设备：请求体里现填的优先（要过上限校验，
// 因为这就是站长正要保存的内容），没填就退回库里已存的（宽松解析，历史数据不该读不出来）。
func (s *BarkNotificationService) resolveTestDeviceKeys(ctx context.Context, requested string) ([]string, error) {
	if strings.TrimSpace(requested) != "" {
		keys, err := ParseBarkDeviceKeys(requested)
		if err != nil {
			return nil, err
		}
		if len(keys) > 0 {
			return keys, nil
		}
	}
	return s.storedDeviceKeys(ctx), nil
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

// IsEnabled Bark 是否已启用且配置完整（可直接推送）。
func (s *BarkNotificationService) IsEnabled(ctx context.Context) bool {
	_, ok := s.runtimeConfig(ctx)
	return ok
}

// NotifyOpsAlertManual 「立即试算」的手动试发：首行说明这不是真实告警，随后当前值、是否越阈、
// 涉及账号（Details）与作用域。Bark 未启用时返回 ErrBarkNotEnabled 而不是静默 nil。
func (s *BarkNotificationService) NotifyOpsAlertManual(ctx context.Context, n OpsAlertNotification, hasData bool, breached bool) error {
	cfg, ok := s.runtimeConfig(ctx)
	if !ok {
		return ErrBarkNotEnabled
	}
	title := fmt.Sprintf("[Sub2API] 手动试发 %s", strings.TrimSpace(n.RuleName))
	return s.push(ctx, cfg, strings.TrimSpace(title), buildOpsAlertManualBarkBody(n, hasData, breached))
}

// push 把一条告警推给配置里的每个设备。全部失败才向上返回错误：
// 只要有一个人收到，告警就算送达，把它判成失败只会让评估器记错状态、白白重试。
func (s *BarkNotificationService) push(ctx context.Context, cfg *BarkConfig, title, body string) error {
	if s == nil || s.sender == nil {
		return errors.New("bark sender not initialized")
	}
	// runtimeConfig 里的 DeviceKey 是解密后的整串，这里切回列表。
	deviceKeys := splitBarkDeviceKeys(cfg.DeviceKey)
	if len(deviceKeys) == 0 {
		return errors.New("bark device_key is not configured")
	}

	summary := s.pushToDevices(ctx, cfg.ServerURL, deviceKeys, BarkMessage{
		Title: title,
		Body:  body,
		Group: cfg.Group,
		Level: cfg.Level,
		URL:   cfg.ClickURL,
		Sound: cfg.Sound,
	}, barkNotifyTimeout)

	if summary.Succeeded == 0 {
		return errors.New(barkFailureMessage(summary))
	}
	if summary.Failed > 0 {
		// 部分失败只记一条 warn：站长得知道是"第几个设备"掉了（多半是 key 注销或 App 卸载），
		// 但日志里只放序号与打码前缀。
		slog.Warn("bark_push_partial_failure",
			"server_url", cfg.ServerURL,
			"succeeded", summary.Succeeded,
			"failed", summary.Failed,
			"failed_devices", barkFailedDeviceLabels(summary),
		)
	}
	return nil
}

// pushToDevices 把同一条消息逐个推给每个 device_key，并汇总逐条结果。
//
// 三条要求决定了这里的写法：
//   - 互不影响：一个设备失败（key 被注销、App 被卸载）不能让别人收不到，所以不 fail-fast，
//     每个结果都收下来再统一判断；
//   - 独立计时：每个设备一个超时预算，避免第一个设备卡满 10 秒后把后面的额度吃光；
//   - 全部失败才算整体失败：交给调用方按 Succeeded 判断。
//
// 设备数有 barkMaxDeviceKeys 上限，直接一个 key 起一个 goroutine 即可，不需要额外限流；
// 每个 goroutine 只写自己那一格 Outcomes，所以不用加锁。
func (s *BarkNotificationService) pushToDevices(
	ctx context.Context,
	serverURL string,
	deviceKeys []string,
	msg BarkMessage,
	timeout time.Duration,
) barkPushSummary {
	summary := barkPushSummary{Total: len(deviceKeys), Outcomes: make([]BarkPushOutcome, len(deviceKeys))}

	var wg sync.WaitGroup
	for i, key := range deviceKeys {
		wg.Add(1)
		go func(idx int, deviceKey string) {
			defer wg.Done()
			sendCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			outcome := BarkPushOutcome{Index: idx + 1, MaskedKey: MaskBarkDeviceKey(deviceKey)}
			sent, err := s.sender.Send(sendCtx, BarkTarget{ServerURL: serverURL, DeviceKey: deviceKey}, msg)
			switch {
			case err != nil:
				// 抹密钥时必须把整份列表都传进去，而不是只传当次这一个：
				// 上游回显 / 代理错误里可能带上别的设备的 key，漏出去任意一个都是事故。
				var sendErr *BarkSendError
				if errors.As(err, &sendErr) {
					outcome.StatusCode = sendErr.StatusCode
					outcome.Message = scrubBarkSecretText(sendErr.Error(), deviceKeys)
				} else {
					// 网络层错误自己不带"推送失败"这层语境，补上前缀，与旧版单设备的返回一致。
					outcome.Message = "bark push failed: " + scrubBarkSecretText(err.Error(), deviceKeys)
				}
			case sent != nil:
				outcome.OK = true
				outcome.StatusCode = sent.StatusCode
				outcome.Message = sent.Message
				outcome.LatencyMs = sent.Latency.Milliseconds()
			default:
				// 发送面既没报错也没给回执，按失败处理而不是静默当成功。
				outcome.Message = "bark sender returned no result"
			}
			summary.Outcomes[idx] = outcome
		}(i, key)
	}
	wg.Wait()

	for _, outcome := range summary.Outcomes {
		if outcome.OK {
			summary.Succeeded++
		} else {
			summary.Failed++
		}
	}
	return summary
}

// firstSuccessfulBarkOutcome 返回第一个成功的设备结果；没有成功的设备时返回零值。
func firstSuccessfulBarkOutcome(summary barkPushSummary) BarkPushOutcome {
	for _, outcome := range summary.Outcomes {
		if outcome.OK {
			return outcome
		}
	}
	return BarkPushOutcome{}
}

// barkFailureMessage 拼「所有设备都失败」时对外的错误信息。
//
// 单设备时逐字沿用旧版那条单独的错误信息，老的调用方与用例不受影响；
// 多设备时按「失败几个 / 共几个 + 逐条序号」展开，只带序号与打码前缀，不会带出完整 key。
func barkFailureMessage(summary barkPushSummary) string {
	failed := make([]BarkPushOutcome, 0, summary.Failed)
	for _, outcome := range summary.Outcomes {
		if !outcome.OK {
			failed = append(failed, outcome)
		}
	}
	if len(failed) == 0 {
		return "bark push failed"
	}
	if summary.Total == 1 {
		return failed[0].Message
	}
	parts := make([]string, 0, len(failed))
	for _, outcome := range failed {
		parts = append(parts, fmt.Sprintf("#%d (%s) %s", outcome.Index, outcome.MaskedKey, outcome.Message))
	}
	return fmt.Sprintf("%d of %d devices failed: %s", len(failed), summary.Total, strings.Join(parts, "; "))
}

// barkFailedDeviceLabels 失败设备的「#序号(打码前缀)」列表，只用于日志。
func barkFailedDeviceLabels(summary barkPushSummary) []string {
	labels := make([]string, 0, summary.Failed)
	for _, outcome := range summary.Outcomes {
		if !outcome.OK {
			labels = append(labels, fmt.Sprintf("#%d(%s)", outcome.Index, outcome.MaskedKey))
		}
	}
	return labels
}

func buildOpsAlertBarkBody(n OpsAlertNotification, resolved bool) string {
	lines := []string{opsAlertBarkMetricLine(n)}
	lines = append(lines, n.Details...)
	lines = append(lines,
		opsAlertBarkValueLine(n),
		"作用域："+opsAlertBarkScope(n),
		"触发时间："+formatBarkTime(n.FiredAt),
	)
	if resolved && n.ResolvedAt != nil {
		line := "恢复时间：" + formatBarkTime(*n.ResolvedAt)
		if d := n.ResolvedAt.Sub(n.FiredAt); d > 0 {
			line += "，持续 " + formatBarkDuration(d)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// buildOpsAlertManualBarkBody 手动试发正文：首行声明不是真实告警，随后指标、当前值、是否越阈、
// 涉及账号（调用方已裁到最多 5 行 +「另有 N 个」）、作用域、评估时间。
func buildOpsAlertManualBarkBody(n OpsAlertNotification, hasData bool, breached bool) string {
	lines := []string{
		"这是手动试发，不代表真实告警",
		opsAlertBarkMetricLine(n),
	}
	if hasData {
		lines = append(lines, opsAlertBarkValueLine(n))
		if breached {
			lines = append(lines, "是否越阈：是")
		} else {
			lines = append(lines, "是否越阈：否")
		}
	} else {
		lines = append(lines,
			fmt.Sprintf("当前值：无数据（阈值 %s %s%s）", strings.TrimSpace(n.Operator), formatBarkNumber(n.Threshold), n.Unit),
			"是否越阈：无数据",
		)
	}
	lines = append(lines, n.Details...)
	lines = append(lines,
		"作用域："+opsAlertBarkScope(n),
		"评估时间："+formatBarkTime(n.FiredAt),
	)
	return strings.Join(lines, "\n")
}

func opsAlertBarkMetricLine(n OpsAlertNotification) string {
	label := strings.TrimSpace(n.MetricLabel)
	if label == "" {
		label = strings.TrimSpace(n.MetricType)
	}
	return "指标：" + label
}

func opsAlertBarkValueLine(n OpsAlertNotification) string {
	return fmt.Sprintf("当前值：%s%s（阈值 %s %s%s）",
		formatBarkNumber(n.Value), n.Unit,
		strings.TrimSpace(n.Operator), formatBarkNumber(n.Threshold), n.Unit,
	)
}

func opsAlertBarkScope(n OpsAlertNotification) string {
	scope := strings.TrimSpace(n.Scope)
	if scope == "" {
		return "全局"
	}
	return scope
}

// FormatOpsAlertScope 把规则的 filters 压成一行作用域说明；无过滤时返回空串。
// 健康度指标只有 platform / group_id / region；账号用量类规则再带 window / dimension / provider
// 与指定账号数（accounts=N）。
func FormatOpsAlertScope(filters map[string]any) string {
	platform, groupID, region := parseOpsAlertRuleScope(filters)
	parts := make([]string, 0, 6)
	if platform != "" {
		parts = append(parts, "platform="+platform)
	}
	if groupID != nil && *groupID > 0 {
		parts = append(parts, fmt.Sprintf("group_id=%d", *groupID))
	}
	if region != nil && *region != "" {
		parts = append(parts, "region="+*region)
	}
	if filters != nil {
		for _, key := range []string{"window", "dimension", "provider"} {
			if v := strings.TrimSpace(stringValue(filters[key])); v != "" {
				parts = append(parts, key+"="+v)
			}
		}
		if ids := ParseOpsAlertAccountIDs(filters["account_ids"]); len(ids) > 0 {
			parts = append(parts, fmt.Sprintf("accounts=%d", len(ids)))
		}
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

// storedDeviceKeys 返回已存 device_key 的明文列表；没有或解不开时返回空切片。
func (s *BarkNotificationService) storedDeviceKeys(ctx context.Context) []string {
	stored, err := s.load(ctx)
	if err != nil || stored == nil {
		return nil
	}
	return s.decryptDeviceKeys(stored.DeviceKey)
}

// decryptDeviceKeys 解密整串 device_key 再切成列表。
// 解不开（典型场景：加密密钥被换过）时返回空切片，调用方一律按"未配置"处理。
func (s *BarkNotificationService) decryptDeviceKeys(ciphertext string) []string {
	if s == nil || s.encryptor == nil || ciphertext == "" {
		return nil
	}
	plain, err := s.encryptor.Decrypt(ciphertext)
	if err != nil {
		slog.Warn("bark_device_key_decrypt_failed", "error", err)
		return nil
	}
	return splitBarkDeviceKeys(plain)
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
	deviceKeys := s.decryptDeviceKeys(stored.DeviceKey)
	if len(deviceKeys) == 0 {
		slog.Warn("bark_config_device_key_unavailable", "server_url", serverURL)
		return nil
	}
	out := *stored
	out.ServerURL = serverURL
	// 运行时配置里 DeviceKey 放的是解密后的整串（已去重、逗号分隔），push 时再切回列表。
	out.DeviceKey = JoinBarkDeviceKeys(deviceKeys)
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

// toBarkConfigView 脱敏输出。deviceKeyCount 必须由调用方解密后数出来：
// cfg.DeviceKey 是密文，从密文本身看不出里面装了几个设备。
func toBarkConfigView(cfg *BarkConfig, deviceKeyCount int) *BarkConfigView {
	if cfg == nil {
		cfg = defaultBarkConfig()
	}
	view := &BarkConfigView{
		Enabled:         cfg.Enabled,
		ServerURL:       cfg.ServerURL,
		DeviceKey:       "",
		HasDeviceKey:    cfg.DeviceKey != "",
		DeviceKeyCount:  deviceKeyCount,
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

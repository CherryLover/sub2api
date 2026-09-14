package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/imroc/req/v3"
)

// ErrSparkShadowResetNotSupported is returned when ResetCredit is called on a
// spark shadow account. Shadow accounts do not hold credentials of their own;
// the caller must reset the parent account directly. It is a structured
// infraerrors value so the handler maps it to 409 Conflict (not a bare 500);
// errors.Is still matches it by identity since ResetCredit returns this var.
var ErrSparkShadowResetNotSupported = infraerrors.New(http.StatusConflict, "SPARK_SHADOW_RESET_NOT_SUPPORTED", "spark shadow account does not support credit reset; reset the parent account")

// Endpoints used by the OpenAI/ChatGPT/Codex quota query and reset feature.
const (
	chatGPTUsageURL             = "https://chatgpt.com/backend-api/wham/usage"
	chatGPTRateLimitCreditsURL  = "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits"
	chatGPTRateLimitResetURL    = "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits/consume"
	openaiQuotaUpstreamTimeout  = 20 * time.Second
	openaiQuotaCodexBeta        = "codex-1"
	openaiQuotaCodexOriginator  = "Codex Desktop"
	openaiQuotaCodexLanguageTag = "zh-CN"
	openaiQuotaSecFetchSite     = "none"
	openaiQuotaSecFetchMode     = "no-cors"
	openaiQuotaSecFetchDest     = "empty"
	openaiQuotaResetCreditsKey  = "codex_reset_credit_snapshot"
)

// OpenAIRateLimitWindow describes a single rate-limit window returned by
// /wham/usage. The upstream returns an explicit `null` window when the slot
// is unused, so consumers should treat a nil pointer as "no data".
type OpenAIRateLimitWindow struct {
	UsedPercent        float64 `json:"used_percent"`
	LimitWindowSeconds int64   `json:"limit_window_seconds"`
	ResetAfterSeconds  int64   `json:"reset_after_seconds"`
	ResetAt            int64   `json:"reset_at"`
}

// OpenAIRateLimit is a rate-limit envelope (primary + optional secondary window).
type OpenAIRateLimit struct {
	Allowed         bool                   `json:"allowed"`
	LimitReached    bool                   `json:"limit_reached"`
	PrimaryWindow   *OpenAIRateLimitWindow `json:"primary_window,omitempty"`
	SecondaryWindow *OpenAIRateLimitWindow `json:"secondary_window,omitempty"`
}

// OpenAIAdditionalRateLimit describes a per-feature rate limit (e.g. Codex Spark).
//
// NormalModelSlug 是上游给这个附加池标注的"对应的普通模型"，实测为 null。它是上游
// 目前唯一暴露出来的「池 → 模型」线索；一旦上游开始下发，就可以用它替换
// openAIDedicatedQuotaPoolMeteredFeatures 里手工维护的映射（见 model_rate_limit.go）。
// 现在只解析、不参与任何判定 —— 全量是 null 的字段没有资格决定调度行为。
type OpenAIAdditionalRateLimit struct {
	LimitName       string           `json:"limit_name"`
	MeteredFeature  string           `json:"metered_feature"`
	RateLimit       *OpenAIRateLimit `json:"rate_limit,omitempty"`
	NormalModelSlug string           `json:"normal_model_slug,omitempty"`
}

// OpenAIModelUsage 是 /wham/usage 顶层 model_usage 里单个模型的可用性描述。
//
// ⚠️ model_usage 是**稀疏**的：上游只列它想特别说明的模型。某个模型没出现在这张表里，
// 既不代表可用，也不代表不可用，而是"没有信息"。所以 Available 必须是指针：调用方
// 必须能区分「上游显式说 false」和「上游根本没提这个模型」。用值类型 bool 读，
// 会把"没信息"静默读成"不可用"，直接导致整批模型被误封。
type OpenAIModelUsage struct {
	Available *bool `json:"available,omitempty"`
	// AvailableAt 是上游给的"预计恢复时间"，实测为 null，真实取值的类型没有文档：
	// 既可能是 unix 秒（与 rate_limit 窗口里的 reset_at 同构），也可能是 RFC3339 字符串。
	// 猜错类型会让整个 /wham/usage 响应 Unmarshal 失败（管理后台的额度卡片会直接变成
	// 上游错误），所以这里原样收下裸 JSON，由 AvailableAtTime 兼容两种写法解析。
	AvailableAt        json.RawMessage `json:"available_at,omitempty"`
	CreditsWouldEnable *bool           `json:"credits_would_enable,omitempty"`
}

// AvailableAtTime 解析 available_at，兼容 unix 秒 / unix 毫秒 / RFC3339 字符串三种写法。
// ok=false 表示上游没给、给了 null，或者给了我们不认识的写法 —— 调用方必须按"没信息"
// 处理，绝不能当成"立即恢复"。
func (m OpenAIModelUsage) AvailableAtTime() (time.Time, bool) {
	raw := strings.TrimSpace(string(m.AvailableAt))
	if raw == "" || raw == "null" {
		return time.Time{}, false
	}
	if ts, err := strconv.ParseFloat(raw, 64); err == nil {
		return openAIEpochToTime(ts)
	}
	var text string
	if err := json.Unmarshal(m.AvailableAt, &text); err != nil {
		return time.Time{}, false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339, text); err == nil {
		return parsed, true
	}
	if ts, err := strconv.ParseFloat(text, 64); err == nil {
		return openAIEpochToTime(ts)
	}
	return time.Time{}, false
}

// openAIEpochToTime 把 unix 时间戳转成 time.Time。大于 1e12 的值按毫秒处理
// （2001 年之后的秒级时间戳都小于 1e10，不可能落进这个区间）。
func openAIEpochToTime(ts float64) (time.Time, bool) {
	if ts <= 0 {
		return time.Time{}, false
	}
	if ts > 1e12 {
		return time.UnixMilli(int64(ts)).UTC(), true
	}
	return time.Unix(int64(ts), 0).UTC(), true
}

// OpenAISpendControl 是上游的消费额度闸门（实测 {"reached": false, "individual_limit": null}）。
//
// IndividualLimit 实测恒为 null，类型未知（可能是数字，也可能是字符串金额）。这里只做
// 原样透传：猜一个具体类型，等于赌上整份 /wham/usage 的解析成功率。
type OpenAISpendControl struct {
	Reached         bool            `json:"reached"`
	IndividualLimit json.RawMessage `json:"individual_limit,omitempty"`
}

// OpenAICredits 是账号的信用点状态（实测 has_credits=false、balance 是字符串 "0"）。
//
// Balance 实测是**字符串**而不是数字（金额用字符串传是为了避免浮点误差），但上游没有
// 文档担保这一点，所以同样原样透传裸 JSON，前端拿到的字面量与上游完全一致。
type OpenAICredits struct {
	HasCredits          bool            `json:"has_credits"`
	Unlimited           bool            `json:"unlimited"`
	OverageLimitReached bool            `json:"overage_limit_reached"`
	Balance             json.RawMessage `json:"balance,omitempty"`
}

// OpenAIRateLimitResetCreditDetail is the sanitized metadata surfaced for one
// available reset credit. Do not add upstream ids or tokens here.
type OpenAIRateLimitResetCreditDetail struct {
	ExpiresAt string `json:"expires_at,omitempty"`
}

// OpenAIRateLimitResetCredits captures the "available_count" surfaced for the
// rate_limit_reset_credit grant type, which the reset action consumes.
type OpenAIRateLimitResetCredits struct {
	AvailableCount int                                `json:"available_count"`
	Credits        []OpenAIRateLimitResetCreditDetail `json:"credits,omitempty"`
}

// OpenAIQuotaUsage is the typed projection of /wham/usage we expose to the UI.
//
// 上游顶层一共 12 个键，历史上我们只解析了 5 个，剩下的一直被静默丢弃 —— 其中
// model_usage 正是"这个模型现在能不能用"的**唯一权威来源**，漏读它让模型级限流
// 只能靠 429 反推、且没有任何自愈（复盘见 openai_model_usage_selfheal.go 文件头）。
//
// 新增字段一律只做解析与透传，不改任何既有字段的语义：管理后台的额度卡片依赖
// rate_limit / additional_rate_limits / rate_limit_reset_credits 的现有含义。
// 目前仍然刻意不解析的只剩 promo（纯营销位，与调度无关）。
type OpenAIQuotaUsage struct {
	UserID                string                       `json:"user_id,omitempty"`
	AccountID             string                       `json:"account_id,omitempty"`
	Email                 string                       `json:"email,omitempty"`
	PlanType              string                       `json:"plan_type,omitempty"`
	RateLimit             *OpenAIRateLimit             `json:"rate_limit,omitempty"`
	CodeReviewRateLimit   *OpenAIRateLimit             `json:"code_review_rate_limit,omitempty"`
	AdditionalRateLimits  []OpenAIAdditionalRateLimit  `json:"additional_rate_limits,omitempty"`
	ModelUsage            map[string]OpenAIModelUsage  `json:"model_usage,omitempty"`
	Credits               *OpenAICredits               `json:"credits,omitempty"`
	SpendControl          *OpenAISpendControl          `json:"spend_control,omitempty"`
	RateLimitReachedType  string                       `json:"rate_limit_reached_type,omitempty"`
	RateLimitResetCredits *OpenAIRateLimitResetCredits `json:"rate_limit_reset_credits,omitempty"`
	FetchedAt             int64                        `json:"fetched_at"`
}

// OpenAIQuotaResetCredit captures the redeemed credit metadata returned by the
// reset endpoint.
type OpenAIQuotaResetCredit struct {
	ID              string `json:"id,omitempty"`
	ResetType       string `json:"reset_type,omitempty"`
	Status          string `json:"status,omitempty"`
	GrantedAt       string `json:"granted_at,omitempty"`
	ExpiresAt       string `json:"expires_at,omitempty"`
	RedeemStartedAt string `json:"redeem_started_at,omitempty"`
	RedeemedAt      string `json:"redeemed_at,omitempty"`
}

// OpenAIQuotaResetResult is the typed projection of /wham/rate-limit-reset-credits/consume.
// The inner Credit also carries `redeemed_at` (RFC3339 string); we deliberately do
// NOT add a top-level redeemed_at to avoid ambiguity with the nested field.
type OpenAIQuotaResetResult struct {
	Code         string                  `json:"code"`
	Credit       *OpenAIQuotaResetCredit `json:"credit,omitempty"`
	WindowsReset int                     `json:"windows_reset"`
}

// OpenAIQuotaService queries and consumes ChatGPT/Codex rate-limit reset credits
// for OpenAI OAuth accounts. It reuses the privacy client factory so all calls
// flow through the impersonated HTTP client (Cloudflare-friendly TLS fingerprint).
type OpenAIQuotaService struct {
	accountRepo          AccountRepository
	proxyRepo            ProxyRepository
	tokenProvider        *OpenAITokenProvider
	privacyClientFactory PrivacyClientFactory
	agentIdentityTaskMu  sync.Mutex
	agentIdentityWS      agentIdentityWSConnectionInvalidator
}

// NewOpenAIQuotaService constructs a quota service. token provider is required —
// it ensures we always invoke upstream with a valid (refreshed-if-needed)
// access_token, sharing the same refresh/locking machinery used by the gateway.
func NewOpenAIQuotaService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	tokenProvider *OpenAITokenProvider,
	privacyClientFactory PrivacyClientFactory,
) *OpenAIQuotaService {
	return &OpenAIQuotaService{
		accountRepo:          accountRepo,
		proxyRepo:            proxyRepo,
		tokenProvider:        tokenProvider,
		privacyClientFactory: privacyClientFactory,
	}
}

// QueryUsage fetches the latest rate-limit/usage snapshot for the given OpenAI
// OAuth account. Returns infraerrors so the handler layer can map them to
// stable error codes / HTTP statuses.
func (s *OpenAIQuotaService) QueryUsage(ctx context.Context, accountID int64) (*OpenAIQuotaUsage, error) {
	accessToken, chatGPTAccountID, proxyURL, fedRAMP, err := s.prepareUpstreamCall(ctx, accountID)
	if err != nil {
		return nil, err
	}

	client, err := s.privacyClientFactory(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_CLIENT_ERROR", "failed to build upstream client: %v", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, openaiQuotaUpstreamTimeout)
	defer cancel()
	agentIdentity := s.isAgentIdentityAccount(ctx, accountID)

	// 模型级限流自愈的"观测代际"：必须在上游往返【之前】读一次库。往返期间真实业务
	// 请求完全可能写入一条新的模型级限流，解除时只允许清掉这里看到的这一代。
	// 详见 openai_model_usage_selfheal.go。
	selfHealObservation := s.observeModelRateLimitsForSelfHeal(ctx, accountID, time.Now())

	var payload OpenAIQuotaUsage
	for recovered := false; ; {
		quotaHeaders, expectedTaskID, headerErr := s.buildCodexQuotaHeaders(callCtx, accountID, accessToken, chatGPTAccountID, fedRAMP)
		if headerErr != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_AUTH_FAILED", "failed to build upstream authentication: %v", headerErr)
		}
		resp, err := client.R().
			SetContext(callCtx).
			SetHeaders(quotaHeaders).
			SetSuccessResult(&payload).
			Get(chatGPTUsageURL)
		if err != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_REQUEST_FAILED", "upstream request failed: %v", err)
		}
		if !resp.IsSuccessState() {
			if agentIdentity && !recovered && isAgentIdentityTaskInvalidHTTPResponse(resp.StatusCode, []byte(resp.String())) {
				recovered = true
				if err := s.recoverAgentIdentityTask(ctx, accountID, expectedTaskID); err != nil {
					return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_AUTH_FAILED", "agent identity task recovery failed: %v", err)
				}
				continue
			}
			status := resp.StatusCode
			body := truncate(s.redactQuotaErrorBody(ctx, accountID, resp.String()), 240)
			slog.Warn("openai_quota_query_failed", "account_id", accountID, "status", status, "body", body)
			return nil, infraerrors.Newf(mapUpstreamStatus(status), "OPENAI_QUOTA_UPSTREAM_ERROR", "upstream returned %d: %s", status, body)
		}
		break
	}

	payload.FetchedAt = time.Now().Unix()
	details := s.queryResetCreditDetails(callCtx, client, accessToken, chatGPTAccountID, fedRAMP, accountID)
	if details != nil {
		hasDetailCount := details.AvailableCount != nil
		if payload.RateLimitResetCredits == nil {
			payload.RateLimitResetCredits = &OpenAIRateLimitResetCredits{}
		}
		if details.CreditListPresent {
			payload.RateLimitResetCredits.Credits = details.Credits
		}
		switch {
		case hasDetailCount:
			payload.RateLimitResetCredits.AvailableCount = *details.AvailableCount
		case details.CreditListPresent:
			payload.RateLimitResetCredits.AvailableCount = details.AvailableCreditCount
		}
	}

	// 顺手做模型级限流自愈：这是全仓库唯一能按「额度池」读到真实余量、并读到
	// model_usage 显式可用性的地方，错过这次就只能等那个可能已经过期的 reset_at。
	// 失败只记日志，绝不影响本次用量查询的返回值。
	s.applyOpenAIModelUsageSelfHeal(ctx, selfHealObservation, &payload, time.Now())
	return &payload, nil
}

// CacheResetCreditsSnapshot persists a complete reset-credit snapshot after an
// explicit UI refresh. The snapshot is written to the account that was queried
// (for a spark shadow that is the shadow row, even though the credits belong to
// its parent) because it is a per-row display cache: each row caches exactly
// what its own card renders, and shadows cannot consume credits anyway.
//
// Missing expiration details leave the old cache intact:
// a snapshot claiming N>0 available credits without their expiration timestamps
// cannot be aged out by readers, so it would keep showing (and offering to
// consume) credits that already expired. Callers must treat this rejection as a
// partial success — the upstream read itself is still valid.
func (s *OpenAIQuotaService) CacheResetCreditsSnapshot(ctx context.Context, accountID int64, credits *OpenAIRateLimitResetCredits) error {
	if credits == nil || (credits.AvailableCount > 0 && len(credits.Credits) == 0) {
		return infraerrors.New(
			http.StatusBadGateway,
			"OPENAI_QUOTA_RESET_CREDITS_REFRESH_FAILED",
			"failed to refresh reset-credit expiration details; cached data was preserved",
		)
	}
	if err := s.accountRepo.UpdateExtra(ctx, accountID, map[string]any{
		openaiQuotaResetCreditsKey: credits,
	}); err != nil {
		return infraerrors.New(
			http.StatusInternalServerError,
			"OPENAI_QUOTA_CACHE_WRITE_FAILED",
			"failed to cache reset-credit details",
		).WithCause(err)
	}
	return nil
}

func (s *OpenAIQuotaService) queryResetCreditDetails(ctx context.Context, client *req.Client, accessToken, chatGPTAccountID string, fedRAMP bool, accountID int64) *openAIRateLimitResetCreditDetails {
	quotaHeaders, _, headerErr := s.buildCodexQuotaHeaders(ctx, accountID, accessToken, chatGPTAccountID, fedRAMP)
	if headerErr != nil {
		slog.Warn("openai_quota_reset_credit_details_auth_failed", "account_id", accountID, "error", headerErr)
		return nil
	}
	resp, err := client.R().
		SetContext(ctx).
		SetHeaders(quotaHeaders).
		Get(chatGPTRateLimitCreditsURL)
	if err != nil {
		slog.Warn("openai_quota_reset_credit_details_failed", "account_id", accountID, "error", err)
		return nil
	}
	if !resp.IsSuccessState() {
		slog.Warn("openai_quota_reset_credit_details_failed", "account_id", accountID, "status", resp.StatusCode)
		return nil
	}

	details, err := parseOpenAIRateLimitResetCreditDetails(resp.Bytes())
	if err != nil {
		slog.Warn("openai_quota_reset_credit_details_parse_failed", "account_id", accountID, "error", err)
		if details.AvailableCount == nil {
			return nil
		}
	}
	if details.AvailableCount == nil && !details.CreditListPresent {
		return nil
	}
	return &details
}

// ResetCredit consumes one rate_limit_reset_credit for the given OpenAI account.
// The redeem_request_id is auto-generated (uuid-like) — upstream uses it for
// idempotency. Returns the consumed credit metadata so the UI can refresh.
func (s *OpenAIQuotaService) ResetCredit(ctx context.Context, accountID int64) (*OpenAIQuotaResetResult, error) {
	// Shadow guard: resetting credits via a shadow account would silently
	// operate on the parent's quota; that is surprising and unwanted. Callers
	// must reset the parent account directly.
	//
	// Fail-closed: if the account cannot be loaded (transient DB error), we
	// must NOT fall through to prepareUpstreamCall. That function resolves a
	// shadow to its parent and would perform a parent-level reset — exactly
	// what this guard must prevent. Return the load error instead.
	if s.accountRepo != nil {
		acc, loadErr := s.accountRepo.GetByID(ctx, accountID)
		if loadErr != nil {
			return nil, infraerrors.Newf(http.StatusNotFound, "OPENAI_QUOTA_ACCOUNT_NOT_FOUND", "account not found: %v", loadErr)
		}
		if acc.IsShadow() {
			return nil, ErrSparkShadowResetNotSupported
		}
	}

	accessToken, chatGPTAccountID, proxyURL, fedRAMP, err := s.prepareUpstreamCall(ctx, accountID)
	if err != nil {
		return nil, err
	}

	redeemRequestID, err := generateRedeemRequestID()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_QUOTA_REDEEM_ID_FAILED", "failed to generate redeem id: %v", err)
	}

	client, err := s.privacyClientFactory(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_CLIENT_ERROR", "failed to build upstream client: %v", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, openaiQuotaUpstreamTimeout)
	defer cancel()
	agentIdentity := s.isAgentIdentityAccount(ctx, accountID)

	var payload OpenAIQuotaResetResult
	for recovered := false; ; {
		headers, expectedTaskID, headerErr := s.buildCodexQuotaHeaders(callCtx, accountID, accessToken, chatGPTAccountID, fedRAMP)
		if headerErr != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_AUTH_FAILED", "failed to build upstream authentication: %v", headerErr)
		}
		headers["content-type"] = "application/json"
		resp, err := client.R().
			SetContext(callCtx).
			SetHeaders(headers).
			SetBody(map[string]string{"redeem_request_id": redeemRequestID}).
			SetSuccessResult(&payload).
			Post(chatGPTRateLimitResetURL)
		if err != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_RESET_REQUEST_FAILED", "upstream request failed: %v", err)
		}
		if !resp.IsSuccessState() {
			if agentIdentity && !recovered && isAgentIdentityTaskInvalidHTTPResponse(resp.StatusCode, []byte(resp.String())) {
				recovered = true
				if err := s.recoverAgentIdentityTask(ctx, accountID, expectedTaskID); err != nil {
					return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_AUTH_FAILED", "agent identity task recovery failed: %v", err)
				}
				continue
			}
			status := resp.StatusCode
			body := truncate(s.redactQuotaErrorBody(callCtx, accountID, resp.String()), 240)
			slog.Warn("openai_quota_reset_failed", "account_id", accountID, "status", status, "body", body)
			return nil, infraerrors.Newf(mapUpstreamStatus(status), "OPENAI_QUOTA_RESET_UPSTREAM_ERROR", "upstream returned %d: %s", status, body)
		}
		break
	}

	slog.Info("openai_quota_reset_success",
		"account_id", accountID,
		"code", payload.Code,
		"windows_reset", payload.WindowsReset,
	)
	return &payload, nil
}

// prepareUpstreamCall loads the account, validates it, obtains a fresh access
// token via the shared TokenProvider, and resolves the chatgpt-account-id and
// proxy URL. Centralized so QueryUsage / ResetCredit share validation.
func (s *OpenAIQuotaService) prepareUpstreamCall(ctx context.Context, accountID int64) (accessToken, chatGPTAccountID, proxyURL string, fedRAMP bool, err error) {
	if s == nil || s.accountRepo == nil || s.privacyClientFactory == nil {
		return "", "", "", false, infraerrors.New(http.StatusInternalServerError, "OPENAI_QUOTA_NOT_CONFIGURED", "openai quota service is not configured")
	}

	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return "", "", "", false, infraerrors.Newf(http.StatusNotFound, "OPENAI_QUOTA_ACCOUNT_NOT_FOUND", "account not found: %v", err)
	}
	if account == nil {
		return "", "", "", false, infraerrors.New(http.StatusNotFound, "OPENAI_QUOTA_ACCOUNT_NOT_FOUND", "account not found")
	}
	if account.Platform != PlatformOpenAI {
		return "", "", "", false, infraerrors.New(http.StatusBadRequest, "OPENAI_QUOTA_INVALID_PLATFORM", "account is not an OpenAI account")
	}
	if account.Type != AccountTypeOAuth {
		return "", "", "", false, infraerrors.New(http.StatusBadRequest, "OPENAI_QUOTA_INVALID_TYPE", "account is not an OAuth account")
	}

	// Spark shadow accounts do not hold their own credentials; resolve to the
	// parent account so that chatgpt_account_id / access_token / proxy all come
	// from the parent. This must happen BEFORE the chatgpt_account_id check.
	if account.IsShadow() {
		resolved, rerr := resolveCredentialAccount(ctx, s.accountRepo, account)
		if rerr != nil {
			return "", "", "", false, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_SHADOW_RESOLVE_FAILED", "failed to resolve shadow account: %v", rerr)
		}
		account = resolved
	}

	chatGPTAccountID = strings.TrimSpace(account.GetCredential("chatgpt_account_id"))
	if chatGPTAccountID == "" {
		// Fall back to organization_id — some legacy accounts only persisted poid.
		chatGPTAccountID = strings.TrimSpace(account.GetCredential("organization_id"))
	}
	if chatGPTAccountID == "" {
		return "", "", "", false, infraerrors.New(http.StatusBadRequest, "OPENAI_QUOTA_MISSING_ACCOUNT_ID", "chatgpt_account_id is missing; please re-authorize this account")
	}

	if !account.IsOpenAIAgentIdentity() {
		if s.tokenProvider == nil {
			return "", "", "", false, infraerrors.New(http.StatusInternalServerError, "OPENAI_QUOTA_NOT_CONFIGURED", "openai quota token provider is not configured")
		}
		accessToken, err = s.tokenProvider.GetAccessToken(ctx, account)
		if err != nil {
			return "", "", "", false, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_TOKEN_UNAVAILABLE", "failed to acquire access token: %v", err)
		}
		if strings.TrimSpace(accessToken) == "" {
			return "", "", "", false, infraerrors.New(http.StatusBadGateway, "OPENAI_QUOTA_TOKEN_UNAVAILABLE", "access token is empty")
		}
	}
	fedRAMP = account.IsChatGPTAccountFedRAMP()

	// account.Proxy is eager-loaded by accountRepo.GetByID (see
	// repository.accountsToService), so we can read the proxy URL directly
	// instead of round-tripping the DB again. Fall back to proxyRepo only
	// when Proxy isn't pre-populated (defensive — e.g. callers that built
	// the Account by hand).
	if account.ProxyID != nil {
		switch {
		case account.Proxy != nil:
			proxyURL = account.Proxy.URL()
		case s.proxyRepo != nil:
			if proxy, perr := s.proxyRepo.GetByID(ctx, *account.ProxyID); perr == nil && proxy != nil {
				proxyURL = proxy.URL()
			}
		}
	}

	return accessToken, chatGPTAccountID, proxyURL, fedRAMP, nil
}

func (s *OpenAIQuotaService) recoverAgentIdentityTask(ctx context.Context, accountID int64, expectedTaskID string) error {
	if s == nil || s.accountRepo == nil {
		return fmt.Errorf("account repository is unavailable")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return fmt.Errorf("account is unavailable")
	}
	if account.IsShadow() {
		account, err = resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil || account == nil {
			return fmt.Errorf("credential account is unavailable")
		}
	}
	if !account.IsOpenAIAgentIdentity() {
		return nil
	}
	return ensureAgentIdentityTaskForAccount(ctx, s.accountRepo, s.agentIdentityWS, &s.agentIdentityTaskMu, account, expectedTaskID)
}

func (s *OpenAIQuotaService) isAgentIdentityAccount(ctx context.Context, accountID int64) bool {
	if s == nil || s.accountRepo == nil {
		return false
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return false
	}
	if account.IsShadow() {
		account, err = resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil || account == nil {
			return false
		}
	}
	return account.IsOpenAIAgentIdentity()
}

func (s *OpenAIQuotaService) buildCodexQuotaHeaders(ctx context.Context, accountID int64, accessToken, chatGPTAccountID string, fedRAMP bool) (map[string]string, string, error) {
	headers := buildCodexCommonHeaders(accessToken, chatGPTAccountID, fedRAMP)
	if s == nil || s.accountRepo == nil {
		return headers, "", nil
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		if strings.TrimSpace(accessToken) == "" {
			return nil, "", fmt.Errorf("agent identity account credentials are unavailable")
		}
		return headers, "", nil
	}
	if account.IsShadow() {
		if resolved, resolveErr := resolveCredentialAccount(ctx, s.accountRepo, account); resolveErr == nil && resolved != nil {
			account = resolved
		} else if strings.TrimSpace(accessToken) == "" {
			return nil, "", fmt.Errorf("agent identity shadow credentials are unavailable")
		}
	}
	if !account.IsOpenAIAgentIdentity() {
		return headers, "", nil
	}
	if err := ensureAgentIdentityTaskForAccount(ctx, s.accountRepo, s.agentIdentityWS, &s.agentIdentityTaskMu, account, ""); err != nil {
		return nil, "", err
	}
	key, err := agentIdentityKeyFromAccount(account)
	if err != nil {
		return nil, "", err
	}
	assertion, err := buildAgentAssertion(key, time.Now())
	if err != nil {
		return nil, "", err
	}
	headers["authorization"] = assertion
	return headers, key.taskID, nil
}

func (s *OpenAIQuotaService) redactQuotaErrorBody(ctx context.Context, accountID int64, body string) string {
	if s == nil || s.accountRepo == nil {
		return body
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return body
	}
	return string(redactAgentIdentitySensitiveBodyForAccount(ctx, s.accountRepo, account, []byte(body)))
}

// buildCodexCommonHeaders sets the request headers expected by the chatgpt.com
// backend so calls succeed past Cloudflare/WASM checks.
func buildCodexCommonHeaders(accessToken, chatGPTAccountID string, fedRAMP bool) map[string]string {
	headers := map[string]string{
		"authorization":      "Bearer " + accessToken,
		"chatgpt-account-id": chatGPTAccountID,
		"openai-beta":        openaiQuotaCodexBeta,
		"oai-language":       openaiQuotaCodexLanguageTag,
		"originator":         openaiQuotaCodexOriginator,
		"accept":             "application/json",
		"sec-fetch-site":     openaiQuotaSecFetchSite,
		"sec-fetch-mode":     openaiQuotaSecFetchMode,
		"sec-fetch-dest":     openaiQuotaSecFetchDest,
		"priority":           "u=4, i",
	}
	if fedRAMP {
		headers["x-openai-fedramp"] = "true"
	}
	return headers
}

// generateRedeemRequestID produces a UUID-v4-shaped string without pulling in a
// new dependency. ChatGPT uses this as an idempotency key for the consume call.
func generateRedeemRequestID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// Set version (4) and variant (RFC 4122) bits.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	hexStr := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexStr[0:8], hexStr[8:12], hexStr[12:16], hexStr[16:20], hexStr[20:]), nil
}

// buildCodexSparkWindowExtraUpdates extracts Codex Spark usage windows from the
// /wham/usage response body's additional_rate_limits, matching the entry with
// MeteredFeature == "codex_bengalfox". It produces plain codex_* keys (NOT the
// Method-Z "codex_spark_" prefix) so that a spark shadow account's extra map
// is populated with the same key names used by the scheduling / frontend layers.
// Returns nil when no codex_bengalfox entry is present or when the RateLimit
// yields no window data.
//
// ⚠️ 只对 **spark 影子账号** 调用（见 account_usage_service.go 的调用点）：影子账号
// 整行只承接 spark，它的 codex_5h_*/codex_7d_* 天然就是 spark 池的数字，写进规范字段
// 不算污染。普通账号绝不能走这条路——普通账号的 codex_5h_*/codex_7d_* 必须是账号
// global 主池（rate_limit）的数字，混入 spark 池会造成整号误限流（2026-09-10 事故，
// 完整复盘见 model_rate_limit.go 的 openAIDedicatedQuotaPoolModels）。
//
// 这里也是「独立额度池」的权威数据源：/wham/usage 是唯一能按池读到真实余量的接口，
// 而 /responses 的 x-codex-* 响应头不带池名、无法自证属于哪个池，只能靠调用方按模型
// 推断（IsOpenAIDedicatedQuotaPoolModel）。
func buildCodexSparkWindowExtraUpdates(usage *OpenAIQuotaUsage, now time.Time) map[string]any {
	if usage == nil {
		return nil
	}
	var spark *OpenAIRateLimit
	for i := range usage.AdditionalRateLimits {
		a := usage.AdditionalRateLimits[i]
		if a.MeteredFeature == openAICodexSparkMeteredFeature {
			spark = a.RateLimit
			break
		}
	}
	if spark == nil {
		return nil
	}

	// Reuse OpenAICodexUsageSnapshot / Normalize to map primary/secondary windows
	// to canonical 5h/7d buckets (same logic as probeOpenAICodexSnapshot).
	snap := &OpenAICodexUsageSnapshot{}
	if w := spark.PrimaryWindow; w != nil {
		p := w.UsedPercent
		snap.PrimaryUsedPercent = &p
		ra := int(w.ResetAfterSeconds)
		snap.PrimaryResetAfterSeconds = &ra
		wm := int(w.LimitWindowSeconds / 60)
		snap.PrimaryWindowMinutes = &wm
	}
	if w := spark.SecondaryWindow; w != nil {
		p := w.UsedPercent
		snap.SecondaryUsedPercent = &p
		ra := int(w.ResetAfterSeconds)
		snap.SecondaryResetAfterSeconds = &ra
		wm := int(w.LimitWindowSeconds / 60)
		snap.SecondaryWindowMinutes = &wm
	}

	normalized := snap.Normalize()
	if normalized == nil {
		return nil
	}

	updates := make(map[string]any)
	if normalized.Used5hPercent != nil {
		updates["codex_5h_used_percent"] = *normalized.Used5hPercent
	}
	if normalized.Reset5hSeconds != nil {
		updates["codex_5h_reset_after_seconds"] = *normalized.Reset5hSeconds
	}
	if normalized.Window5hMinutes != nil {
		updates["codex_5h_window_minutes"] = *normalized.Window5hMinutes
	}
	if normalized.Used7dPercent != nil {
		updates["codex_7d_used_percent"] = *normalized.Used7dPercent
	}
	if normalized.Reset7dSeconds != nil {
		updates["codex_7d_reset_after_seconds"] = *normalized.Reset7dSeconds
	}
	if normalized.Window7dMinutes != nil {
		updates["codex_7d_window_minutes"] = *normalized.Window7dMinutes
	}
	if r := codexResetAtRFC3339(now, normalized.Reset5hSeconds); r != nil {
		updates["codex_5h_reset_at"] = *r
	}
	if r := codexResetAtRFC3339(now, normalized.Reset7dSeconds); r != nil {
		updates["codex_7d_reset_at"] = *r
	}
	if len(updates) == 0 {
		return nil
	}
	updates["codex_usage_updated_at"] = now.Format(time.RFC3339)
	return updates
}

// mapUpstreamStatus collapses upstream HTTP statuses into a stable set we
// surface from the admin handler. 4xx upstream errors are surfaced as 502
// (BadGateway) so callers can distinguish "your input is bad" (400) from
// "upstream said no" (502); 401/403 are bubbled directly to hint at re-auth.
func mapUpstreamStatus(status int) int {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return status
	case status == http.StatusTooManyRequests:
		return http.StatusTooManyRequests
	case status >= 400 && status < 500:
		return http.StatusBadGateway
	case status >= 500:
		return http.StatusBadGateway
	default:
		return http.StatusBadGateway
	}
}

//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type geminiLifecycleAccounts struct {
	service.AccountRepository
	account service.Account
}

func (r *geminiLifecycleAccounts) GetByID(context.Context, int64) (*service.Account, error) {
	a := r.account
	return &a, nil
}
func (r *geminiLifecycleAccounts) ListSchedulableByGroupIDAndPlatforms(context.Context, int64, []string) ([]service.Account, error) {
	return []service.Account{r.account}, nil
}
func (r *geminiLifecycleAccounts) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]service.Account, error) {
	return []service.Account{r.account}, nil
}
func (r *geminiLifecycleAccounts) ListSchedulableByPlatform(context.Context, string) ([]service.Account, error) {
	return []service.Account{r.account}, nil
}
func (r *geminiLifecycleAccounts) ListSchedulableByGroupID(context.Context, int64) ([]service.Account, error) {
	return []service.Account{r.account}, nil
}

type geminiLifecycleGroups struct {
	service.GroupRepository
	group *service.Group
}

func (r *geminiLifecycleGroups) GetByID(context.Context, int64) (*service.Group, error) {
	return r.group, nil
}

func (r *geminiLifecycleGroups) GetByIDLite(context.Context, int64) (*service.Group, error) {
	return r.group, nil
}

type geminiLifecycleStore struct {
	service.GeminiCachedContentRepository
	record                    *service.GeminiCachedContent
	lease                     string
	quotes                    []*service.GeminiCachedContentSettlement
	rejectCreate, rejectRenew bool
}

func (r *geminiLifecycleStore) GetForOwner(_ context.Context, keyID int64, id string) (*service.GeminiCachedContent, error) {
	if r.record == nil || r.record.APIKeyID != keyID || r.record.PublicID != id {
		return nil, service.ErrGeminiCachedContentNotFound
	}
	copy := *r.record
	return &copy, nil
}
func (r *geminiLifecycleStore) ListForOwner(_ context.Context, key, group int64, at time.Time, before int64, limit int) ([]*service.GeminiCachedContent, error) {
	if r.record != nil && r.record.APIKeyID == key && r.record.GroupID == group && r.record.ExpireTime.After(at) {
		return []*service.GeminiCachedContent{r.record}, nil
	}
	return nil, nil
}
func (r *geminiLifecycleStore) CreateWithSettlement(_ context.Context, record *service.GeminiCachedContent, q *service.GeminiCachedContentSettlement) (*service.UsageBillingApplyResult, error) {
	if r.rejectCreate {
		return nil, service.ErrAPIKeyQuotaExhausted
	}
	record.ID = 1
	record.CreatedAt = time.Now()
	record.UpdatedAt = record.CreatedAt
	copy := *record
	r.record = &copy
	r.quotes = append(r.quotes, q)
	return &service.UsageBillingApplyResult{Applied: true}, nil
}
func (r *geminiLifecycleStore) BeginMutation(_ context.Context, id int64, baseline time.Time, token string, lease time.Duration) (bool, error) {
	if r.record == nil {
		return false, service.ErrGeminiCachedContentNotFound
	}
	if r.lease != "" || !r.record.ExpireTime.Equal(baseline) {
		return false, nil
	}
	r.lease = token
	return true, nil
}
func (r *geminiLifecycleStore) TakeOverStaleMutation(context.Context, int64, string, time.Duration) (*service.GeminiCachedContent, error) {
	return nil, nil
}
func (r *geminiLifecycleStore) ReleaseMutation(_ context.Context, _ int64, token string) error {
	if r.lease == token {
		r.lease = ""
	}
	return nil
}
func (r *geminiLifecycleStore) CommitMutationWithSettlement(_ context.Context, record *service.GeminiCachedContent, token string, expiry time.Time, q *service.GeminiCachedContentSettlement) (*service.UsageBillingApplyResult, error) {
	if r.rejectRenew {
		return nil, service.ErrAPIKeyQuotaExhausted
	}
	if r.lease != token {
		return nil, service.ErrGeminiCachedContentMutationConflict
	}
	r.record.ExpireTime = expiry
	r.lease = ""
	r.quotes = append(r.quotes, q)
	return &service.UsageBillingApplyResult{Applied: true}, nil
}
func (r *geminiLifecycleStore) DeleteWithMutation(_ context.Context, _ *service.GeminiCachedContent, token string) error {
	if r.lease != token {
		return service.ErrGeminiCachedContentMutationConflict
	}
	r.record = nil
	r.lease = ""
	return nil
}
func (r *geminiLifecycleStore) SoftDelete(context.Context, int64) error {
	r.record = nil
	r.lease = ""
	return nil
}

type geminiLifecycleUpstream struct {
	service.HTTPUpstream
	methods        []string
	expiry         time.Time
	transportError error
}

func (u *geminiLifecycleUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.methods = append(u.methods, req.Method)
	if u.transportError != nil {
		return nil, u.transportError
	}
	if req.Method == http.MethodDelete {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	}
	body, _ := io.ReadAll(req.Body)
	ttl, err := time.ParseDuration(gjson.GetBytes(body, "ttl").String())
	if err != nil {
		return nil, err
	}
	u.expiry = time.Now().Add(ttl)
	out := fmt.Sprintf(`{"name":"cachedContents/provider1","usageMetadata":{"totalTokenCount":3000},"expireTime":%q}`, u.expiry.UTC().Format(time.RFC3339Nano))
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(out))}, nil
}

type geminiLifecycleFixture struct {
	h    *GatewayHandler
	key  *service.APIKey
	repo *geminiLifecycleStore
	up   *geminiLifecycleUpstream
}

func newGeminiLifecycleFixture(t *testing.T) *geminiLifecycleFixture {
	t.Helper()
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	group := &service.Group{ID: 42, Platform: service.PlatformGemini, Status: service.StatusActive, RateMultiplier: 1, Hydrated: true}
	inputPrice, outputPrice := 2e-6, 8e-6
	group.ModelPricing = []service.ChannelModelPricing{{Models: []string{"gemini-2.5-pro"}, BillingMode: service.BillingModeToken, InputPrice: &inputPrice, OutputPrice: &outputPrice}}
	key := &service.APIKey{ID: 43, UserID: 44, User: &service.User{ID: 44, Status: service.StatusActive}, Group: group, GroupID: &group.ID}
	accounts := &geminiLifecycleAccounts{account: service.Account{ID: 45, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, GroupIDs: []int64{group.ID}, Credentials: map[string]any{"api_key": "upstream-test-key"}}}
	groups := &geminiLifecycleGroups{group: group}
	concurrency := service.NewConcurrencyService(nil)
	billing := service.NewBillingCacheService(nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	price := service.NewBillingService(cfg, nil)
	up := &geminiLifecycleUpstream{}
	compat := service.NewGeminiMessagesCompatService(accounts, groups, nil, nil, nil, nil, up, nil, cfg)
	gateway := service.NewGatewayService(accounts, groups, nil, nil, nil, nil, cfg, nil, concurrency, price, nil, billing, nil, up, nil, nil, nil, nil, nil, nil, nil, nil, service.NewModelPricingResolver(nil, price), nil, nil)
	repo := &geminiLifecycleStore{}
	h := &GatewayHandler{gatewayService: gateway, geminiCompatService: compat, geminiCachedContentService: service.NewGeminiCachedContentService(repo, compat, cfg), concurrencyHelper: NewConcurrencyHelper(concurrency, SSEPingFormatNone, 0), billingCacheService: billing, apiKeyService: service.NewAPIKeyService(nil, nil, nil, nil, nil, cfg), cfg: cfg}
	return &geminiLifecycleFixture{h: h, key: key, repo: repo, up: up}
}
func (f *geminiLifecycleFixture) request(method, path, body string, handler func(*gin.Context)) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyAPIKey), f.key)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: f.key.UserID})
	if f.repo.record != nil {
		c.Params = gin.Params{{Key: "cacheID", Value: f.repo.record.PublicID}}
	}
	handler(c)
	return w
}

func TestGeminiCachedContentHTTPLifecycle(t *testing.T) {
	f := newGeminiLifecycleFixture(t)
	w := f.request("POST", "/v1beta/cachedContents", `{"model":"gemini-2.5-pro","ttl":"3600s","contents":[{"role":"user","parts":[{"text":"cached reference"}]}]}`, f.h.GeminiCachedContentsCreate)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NotNil(t, f.repo.record)
	require.Len(t, f.repo.quotes, 1)
	require.True(t, json.Valid(w.Body.Bytes()))
	require.NotContains(t, w.Body.String(), "provider1")
	initial := f.repo.record.ExpireTime
	w = f.request("GET", "/v1beta/cachedContents", ``, f.h.GeminiCachedContentsList)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Len(t, gjson.Get(w.Body.String(), "cachedContents").Array(), 1)
	w = f.request("GET", "/v1beta/cachedContents/x", ``, f.h.GeminiCachedContentsGet)
	require.Equal(t, 200, w.Code, w.Body.String())
	w = f.request("PATCH", "/v1beta/cachedContents/x?updateMask=ttl", `{"ttl":"7200s"}`, f.h.GeminiCachedContentsPatch)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.True(t, f.repo.record.ExpireTime.After(initial))
	require.Len(t, f.repo.quotes, 2)
	require.Zero(t, f.repo.quotes[1].Log.InputTokens)
	require.Positive(t, f.repo.quotes[1].Log.TotalCost)
	w = f.request("DELETE", "/v1beta/cachedContents/x", ``, f.h.GeminiCachedContentsDelete)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Nil(t, f.repo.record)
	require.Len(t, f.repo.quotes, 2)
	require.Equal(t, []string{"POST", "PATCH", "DELETE"}, f.up.methods)
}

func TestGeminiCachedContentHTTPCreateBillingFailureCleansUp(t *testing.T) {
	f := newGeminiLifecycleFixture(t)
	f.repo.rejectCreate = true
	w := f.request("POST", "/v1beta/cachedContents", `{"model":"gemini-2.5-pro"}`, f.h.GeminiCachedContentsCreate)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String()) // Preserve the existing post-admission billingErrorDetails contract.
	require.Nil(t, f.repo.record)
	require.Empty(t, f.repo.quotes)
	require.Equal(t, []string{"POST", "DELETE"}, f.up.methods)
}

func TestGeminiCachedContentHTTPMissingInputPriceDoesNotCreate(t *testing.T) {
	f := newGeminiLifecycleFixture(t)
	f.key.Group.ModelPricing = nil
	w := f.request("POST", "/v1beta/cachedContents", `{"model":"gemini-2.5-pro"}`, f.h.GeminiCachedContentsCreate)
	require.Equal(t, 400, w.Code, w.Body.String())
	require.Empty(t, f.up.methods)
	require.Nil(t, f.repo.record)
}

func TestGeminiCachedContentHTTPUnknownCreateOutcomeDoesNotRetry(t *testing.T) {
	f := newGeminiLifecycleFixture(t)
	f.up.transportError = errors.New("connection reset after request sent")
	w := f.request("POST", "/v1beta/cachedContents", `{"model":"gemini-2.5-pro"}`, f.h.GeminiCachedContentsCreate)
	require.Equal(t, 503, w.Code, w.Body.String())
	require.Equal(t, []string{"POST"}, f.up.methods)
	require.Nil(t, f.repo.record)
	require.Empty(t, f.repo.quotes)
}

func TestGeminiCachedContentHTTPRenewBillingFailureRollsBack(t *testing.T) {
	f := newGeminiLifecycleFixture(t)
	w := f.request("POST", "/v1beta/cachedContents", `{"model":"gemini-2.5-pro"}`, f.h.GeminiCachedContentsCreate)
	require.Equal(t, 200, w.Code, w.Body.String())
	baseline := f.repo.record.ExpireTime
	f.repo.rejectRenew = true
	w = f.request("PATCH", "/v1beta/cachedContents/x", `{"ttl":"7200s"}`, f.h.GeminiCachedContentsPatch)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String()) // The transaction is rejected, not reported as a successful renewal.
	require.True(t, f.repo.record.ExpireTime.Equal(baseline))
	require.Empty(t, f.repo.lease)
	require.WithinDuration(t, baseline, f.up.expiry, 2*time.Second)
	require.Len(t, f.repo.quotes, 1)
	require.Equal(t, []string{"POST", "PATCH", "PATCH"}, f.up.methods)
}

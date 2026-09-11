//go:build integration

package repository

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
)

// OpsRequestChainRepoSuite 跑在真库上：请求链路那条 SQL 的风险几乎全在 JSONB 上
// （jsonb_array_elements 碰到非数组会直接报错、字段类型不可信），
// sqlmock 验证不了这些，只有真 Postgres 能。
type OpsRequestChainRepoSuite struct {
	suite.Suite
	ctx  context.Context
	repo *opsRepository
}

func (s *OpsRequestChainRepoSuite) SetupTest() {
	s.ctx = context.Background()
	s.repo = &opsRepository{db: integrationDB}
}

func TestOpsRequestChainRepoSuite(t *testing.T) {
	suite.Run(t, new(OpsRequestChainRepoSuite))
}

// insertChainErrorLog 插一条 ops_error_logs；upstreamErrors 传 nil 表示该列为 SQL NULL。
func (s *OpsRequestChainRepoSuite) insertChainErrorLog(
	clientRequestID string,
	createdAt time.Time,
	statusCode int,
	stream bool,
	ttft *int64,
	upstreamErrors any,
) int64 {
	s.T().Helper()

	var id int64
	err := integrationDB.QueryRowContext(s.ctx, `
		INSERT INTO ops_error_logs (
			client_request_id, error_phase, error_type, severity, status_code,
			platform, model, requested_model, stream, time_to_first_token_ms,
			upstream_errors, created_at
		) VALUES ($1, 'upstream', 'upstream_error', 'error', $2,
			'openai', 'gpt-5.6-sol', 'gpt-5.6-sol', $3, $4,
			$5::jsonb, $6)
		RETURNING id`,
		clientRequestID, statusCode, stream, ttft, upstreamErrors, createdAt,
	).Scan(&id)
	s.Require().NoError(err)

	s.T().Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(),
			`DELETE FROM ops_error_logs WHERE id = $1`, id)
	})
	return id
}

// insertChainUsageLog 插一条 usage_logs，request_id 必须自带 "client:" 前缀。
func (s *OpsRequestChainRepoSuite) insertChainUsageLog(
	requestID string,
	account *service.Account,
	user *service.User,
	apiKey *service.APIKey,
	createdAt time.Time,
) {
	s.T().Helper()

	_, err := integrationDB.ExecContext(s.ctx, `
		INSERT INTO usage_logs (
			user_id, api_key_id, account_id, request_id, model,
			input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
			total_cost, actual_cost, stream, first_token_ms, created_at
		) VALUES ($1, $2, $3, $4, 'gpt-5.6-sol',
			1000, 800, 50, 33,
			0.0212, 0.0106, true, 1200, $5)`,
		user.ID, apiKey.ID, account.ID, requestID, createdAt,
	)
	s.Require().NoError(err)

	s.T().Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(),
			`DELETE FROM usage_logs WHERE request_id = $1`, requestID)
	})
}

func (s *OpsRequestChainRepoSuite) newChainAccount(name string) *service.Account {
	s.T().Helper()
	return mustCreateAccount(s.T(), testEntClient(s.T()), &service.Account{
		Name:     name + "-" + uuid.NewString()[:8],
		Platform: service.PlatformOpenAI,
	})
}

// 一次客户端请求：三次尝试全撞在同一个账号上、全是 502，最后仍由该账号成功。
// 这条用例同时钉住两件事：JSONB 数组能按 at_unix_ms 展开，以及 usage_logs 关联
// 必须补 "client:" 前缀（不补会一条都对不上）。
func (s *OpsRequestChainRepoSuite) TestGetRequestChainSource_RecoveredWithUsageLog() {
	clientRequestID := "chain-" + uuid.NewString()
	account := s.newChainAccount("chain-acct")
	user := mustCreateUser(s.T(), testEntClient(s.T()), &service.User{Email: "chain-" + uuid.NewString() + "@test.com"})
	apiKey := mustCreateApiKey(s.T(), testEntClient(s.T()), &service.APIKey{
		UserID: user.ID, Key: "sk-chain-" + uuid.NewString(), Name: "chain-key",
	})

	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	ttft := int64(18501)

	// 中途落的失败记录，只带前两次尝试。
	s.insertChainErrorLog(clientRequestID, base, 502, true, nil, `[
		{"at_unix_ms": `+unixMilliString(base)+`, "kind": "failover", "account_id": `+int64String(account.ID)+`,
		 "account_name": "anviz-7", "platform": "openai", "upstream_status_code": 502,
		 "message": "Our servers are currently overloaded."},
		{"at_unix_ms": `+unixMilliString(base.Add(2*time.Second))+`, "kind": "failover", "account_id": `+int64String(account.ID)+`,
		 "account_name": "anviz-7", "platform": "openai", "upstream_status_code": 502,
		 "message": "Our servers are currently overloaded."}
	]`)

	// 收尾记录：客户端最终拿到 200，upstream_errors 把三次尝试都抄了一遍。
	// 第三次故意不写 account_name，用来验证会回退到 accounts 表取名字。
	s.insertChainErrorLog(clientRequestID, base.Add(19*time.Second), 200, true, &ttft, `[
		{"at_unix_ms": `+unixMilliString(base)+`, "kind": "failover", "account_id": `+int64String(account.ID)+`,
		 "account_name": "anviz-7", "platform": "openai", "upstream_status_code": 502,
		 "message": "Our servers are currently overloaded."},
		{"at_unix_ms": `+unixMilliString(base.Add(2*time.Second))+`, "kind": "failover", "account_id": `+int64String(account.ID)+`,
		 "account_name": "anviz-7", "platform": "openai", "upstream_status_code": 502,
		 "message": "Our servers are currently overloaded."},
		{"at_unix_ms": `+unixMilliString(base.Add(5*time.Second))+`, "kind": "failover", "account_id": `+int64String(account.ID)+`,
		 "platform": "openai", "upstream_status_code": 502,
		 "message": "Our servers are currently overloaded."}
	]`)

	s.insertChainUsageLog(
		service.OpsUsageLogRequestIDForClientRequestID(clientRequestID),
		account, user, apiKey, base.Add(19*time.Second),
	)

	source, err := s.repo.GetRequestChainSource(s.ctx, clientRequestID)
	s.Require().NoError(err)
	s.Require().NotNil(source)

	s.Require().Len(source.Errors, 2, "同一个 client_request_id 的两条记录都要返回")
	s.Require().True(source.Errors[0].CreatedAt.Before(source.Errors[1].CreatedAt), "按 created_at 升序")
	s.Require().Equal(502, source.Errors[0].StatusCode)
	s.Require().Len(source.Errors[0].Attempts, 2)
	s.Require().Equal(200, source.Errors[1].StatusCode)
	s.Require().Len(source.Errors[1].Attempts, 3)
	s.Require().NotNil(source.Errors[1].TimeToFirstTokenMs)
	s.Require().EqualValues(18501, *source.Errors[1].TimeToFirstTokenMs)

	attempt := source.Errors[0].Attempts[0]
	s.Require().Equal(base.UnixMilli(), attempt.AtUnixMs)
	s.Require().Equal(account.ID, attempt.AccountID)
	s.Require().Equal("anviz-7", attempt.AccountName, "优先用事件里的账号名快照")
	s.Require().Equal("openai", attempt.Platform)
	s.Require().Equal(502, attempt.UpstreamStatusCode)
	s.Require().Equal("failover", attempt.Kind)
	s.Require().Equal("Our servers are currently overloaded.", attempt.Message)

	s.Require().Equal(account.Name, source.Errors[1].Attempts[2].AccountName, "事件没记账号名时回退 accounts 表")

	s.Require().NotNil(source.Usage, "usage_logs 必须靠 client: 前缀关联上")
	s.Require().Equal(account.ID, source.Usage.AccountID)
	s.Require().Equal(account.Name, source.Usage.AccountName)
	s.Require().EqualValues(1883, source.Usage.TotalTokens, "四类 token 现加：1000+800+50+33")
	s.Require().InDelta(0.0212, source.Usage.Cost, 1e-9)
	s.Require().NotNil(source.Usage.FirstTokenMs)
	s.Require().EqualValues(1200, *source.Usage.FirstTokenMs)

	// 装配后的业务结果：三次尝试合并去重、outcome=recovered。
	chain := serviceBuildChainForTest(clientRequestID, source)
	s.Require().Equal(service.OpsRequestChainOutcomeRecovered, chain.Outcome)
	s.Require().Len(chain.Attempts, 3)
	s.Require().Equal(1, chain.Attempts[0].Seq)
	s.Require().NotNil(chain.Final)
	s.Require().True(chain.Final.Succeeded)
}

// upstream_errors 可能是 SQL NULL、JSON null、空数组，甚至（脏数据）是个对象。
// jsonb_array_elements 碰到后三种会直接报错，所以查询里必须先 jsonb_typeof 判型。
func (s *OpsRequestChainRepoSuite) TestGetRequestChainSource_NonArrayUpstreamErrorsDoesNotError() {
	cases := map[string]any{
		"SQL NULL":  nil,
		"JSON null": `null`,
		"空数组":       `[]`,
		"对象而非数组":    `{"kind": "failover"}`,
		"标量":        `502`,
	}
	for name, value := range cases {
		s.Run(name, func() {
			clientRequestID := "chain-empty-" + uuid.NewString()
			s.insertChainErrorLog(clientRequestID, time.Now().UTC().Add(-time.Hour), 502, false, nil, value)

			source, err := s.repo.GetRequestChainSource(s.ctx, clientRequestID)
			s.Require().NoError(err)
			s.Require().Len(source.Errors, 1, "没有上游尝试的记录也必须留在结果里")
			s.Require().NotNil(source.Errors[0].Attempts)
			s.Require().Empty(source.Errors[0].Attempts)
			s.Require().Nil(source.Usage)

			chain := serviceBuildChainForTest(clientRequestID, source)
			s.Require().Equal(service.OpsRequestChainOutcomeFailed, chain.Outcome)
			s.Require().Empty(chain.Attempts)
		})
	}
}

// 数组元素里的字段类型同样不可信：写成字符串的 at_unix_ms / account_id 不该让整个接口 500。
func (s *OpsRequestChainRepoSuite) TestGetRequestChainSource_MalformedAttemptFieldsDegradeToZero() {
	clientRequestID := "chain-dirty-" + uuid.NewString()
	s.insertChainErrorLog(clientRequestID, time.Now().UTC().Add(-time.Hour), 502, false, nil, `[
		{"at_unix_ms": "not-a-number", "account_id": "seven", "upstream_status_code": "502",
		 "kind": "failover", "message": "overloaded"}
	]`)

	source, err := s.repo.GetRequestChainSource(s.ctx, clientRequestID)
	s.Require().NoError(err)
	s.Require().Len(source.Errors, 1)
	s.Require().Len(source.Errors[0].Attempts, 1)

	attempt := source.Errors[0].Attempts[0]
	s.Require().Zero(attempt.AtUnixMs)
	s.Require().Zero(attempt.AccountID)
	s.Require().Zero(attempt.UpstreamStatusCode)
	s.Require().Equal("failover", attempt.Kind)
	s.Require().Equal("overloaded", attempt.Message)

	// at_unix_ms 缺失时退回所属记录的 created_at，链路仍然成立。
	chain := serviceBuildChainForTest(clientRequestID, source)
	s.Require().Len(chain.Attempts, 1)
	s.Require().True(chain.Attempts[0].At.Equal(source.Errors[0].CreatedAt))
}

func (s *OpsRequestChainRepoSuite) TestGetRequestChainSource_UnknownIDReturnsNoRecords() {
	source, err := s.repo.GetRequestChainSource(s.ctx, "chain-missing-"+uuid.NewString())
	s.Require().NoError(err)
	s.Require().Empty(source.Errors)
	s.Require().Nil(source.Usage)
	s.Require().Nil(serviceBuildChainForTest("x", source))
}

// serviceBuildChainForTest 借服务层的入口把仓储结果装配成链路；
// 服务层的装配分支另有单测覆盖，这里只验证「真库出来的数据能走通」。
func serviceBuildChainForTest(clientRequestID string, source *service.OpsRequestChainSource) *service.OpsRequestChain {
	svc := service.NewOpsService(&opsRequestChainSourceStub{source: source}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	chain, err := svc.GetRequestChain(context.Background(), clientRequestID)
	if err != nil {
		return nil
	}
	return chain
}

type opsRequestChainSourceStub struct {
	service.OpsRepository
	source *service.OpsRequestChainSource
}

func (s *opsRequestChainSourceStub) GetRequestChainSource(_ context.Context, _ string) (*service.OpsRequestChainSource, error) {
	return s.source, nil
}

func unixMilliString(t time.Time) string {
	return int64String(t.UnixMilli())
}

func int64String(v int64) string {
	return strconv.FormatInt(v, 10)
}

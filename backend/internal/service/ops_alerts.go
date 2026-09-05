package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func (s *OpsService) ListAlertRules(ctx context.Context) ([]*OpsAlertRule, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return []*OpsAlertRule{}, nil
	}
	return s.opsRepo.ListAlertRules(ctx)
}

func (s *OpsService) CreateAlertRule(ctx context.Context, rule *OpsAlertRule) (*OpsAlertRule, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if rule == nil {
		return nil, infraerrors.BadRequest("INVALID_RULE", "invalid rule")
	}

	created, err := s.opsRepo.CreateAlertRule(ctx, rule)
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *OpsService) UpdateAlertRule(ctx context.Context, rule *OpsAlertRule) (*OpsAlertRule, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if rule == nil || rule.ID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RULE", "invalid rule")
	}

	updated, err := s.opsRepo.UpdateAlertRule(ctx, rule)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, infraerrors.NotFound("OPS_ALERT_RULE_NOT_FOUND", "alert rule not found")
		}
		return nil, err
	}
	return updated, nil
}

func (s *OpsService) DeleteAlertRule(ctx context.Context, id int64) error {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return err
	}
	if s.opsRepo == nil {
		return infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if id <= 0 {
		return infraerrors.BadRequest("INVALID_RULE_ID", "invalid rule id")
	}
	if err := s.opsRepo.DeleteAlertRule(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return infraerrors.NotFound("OPS_ALERT_RULE_NOT_FOUND", "alert rule not found")
		}
		return err
	}
	return nil
}

func (s *OpsService) ListAlertEvents(ctx context.Context, filter *OpsAlertEventFilter) ([]*OpsAlertEvent, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return []*OpsAlertEvent{}, nil
	}
	return s.opsRepo.ListAlertEvents(ctx, filter)
}

func (s *OpsService) GetAlertEventByID(ctx context.Context, eventID int64) (*OpsAlertEvent, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if eventID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_EVENT_ID", "invalid event id")
	}
	ev, err := s.opsRepo.GetAlertEventByID(ctx, eventID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, infraerrors.NotFound("OPS_ALERT_EVENT_NOT_FOUND", "alert event not found")
		}
		return nil, err
	}
	if ev == nil {
		return nil, infraerrors.NotFound("OPS_ALERT_EVENT_NOT_FOUND", "alert event not found")
	}
	return ev, nil
}

func (s *OpsService) GetActiveAlertEvent(ctx context.Context, ruleID int64) (*OpsAlertEvent, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if ruleID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RULE_ID", "invalid rule id")
	}
	return s.opsRepo.GetActiveAlertEvent(ctx, ruleID)
}

func (s *OpsService) CreateAlertSilence(ctx context.Context, input *OpsAlertSilence) (*OpsAlertSilence, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if input == nil {
		return nil, infraerrors.BadRequest("INVALID_SILENCE", "invalid silence")
	}
	if input.RuleID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RULE_ID", "invalid rule id")
	}
	if strings.TrimSpace(input.Platform) == "" {
		return nil, infraerrors.BadRequest("INVALID_PLATFORM", "invalid platform")
	}
	if input.Until.IsZero() {
		return nil, infraerrors.BadRequest("INVALID_UNTIL", "invalid until")
	}

	created, err := s.opsRepo.CreateAlertSilence(ctx, input)
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *OpsService) IsAlertSilenced(ctx context.Context, ruleID int64, platform string, groupID *int64, region *string, now time.Time) (bool, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return false, err
	}
	if s.opsRepo == nil {
		return false, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if ruleID <= 0 {
		return false, infraerrors.BadRequest("INVALID_RULE_ID", "invalid rule id")
	}
	if strings.TrimSpace(platform) == "" {
		return false, nil
	}
	return s.opsRepo.IsAlertSilenced(ctx, ruleID, platform, groupID, region, now)
}

func (s *OpsService) GetLatestAlertEvent(ctx context.Context, ruleID int64) (*OpsAlertEvent, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if ruleID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RULE_ID", "invalid rule id")
	}
	return s.opsRepo.GetLatestAlertEvent(ctx, ruleID)
}

func (s *OpsService) CreateAlertEvent(ctx context.Context, event *OpsAlertEvent) (*OpsAlertEvent, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if event == nil {
		return nil, infraerrors.BadRequest("INVALID_EVENT", "invalid event")
	}

	created, err := s.opsRepo.CreateAlertEvent(ctx, event)
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *OpsService) UpdateAlertEventStatus(ctx context.Context, eventID int64, status string, resolvedAt *time.Time) error {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return err
	}
	if s.opsRepo == nil {
		return infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if eventID <= 0 {
		return infraerrors.BadRequest("INVALID_EVENT_ID", "invalid event id")
	}
	status = strings.TrimSpace(status)
	if status == "" {
		return infraerrors.BadRequest("INVALID_STATUS", "invalid status")
	}
	if status != OpsAlertStatusResolved && status != OpsAlertStatusManualResolved {
		return infraerrors.BadRequest("INVALID_STATUS", "invalid status")
	}
	return s.opsRepo.UpdateAlertEventStatus(ctx, eventID, status, resolvedAt)
}

// EvaluateAlertRuleNow 「立即试算」一条规则，透传给评估器；未接线时返回 503。
func (s *OpsService) EvaluateAlertRuleNow(ctx context.Context, ruleID int64, send bool) (*OpsAlertRuleEvaluation, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if ruleID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RULE_ID", "invalid rule id")
	}
	if s.alertRuleEvaluator == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_ALERT_EVALUATOR_UNAVAILABLE", "Ops alert evaluator not available")
	}
	return s.alertRuleEvaluator.EvaluateRuleNow(ctx, ruleID, send)
}

// ListAccountsForAlerts 账号用量类告警规则取账号：整实体（含 extra 快照）。
//
// 必须走 ListAllWithFilters / GetByIDs，不能用 ListOpsAccountsForStats —— 后者只选 11 列、
// 没有 extra，所有用量都会读成 0，规则"正常运行"但永远不触发。
// accountIDs 非空时按 ID 取，再用 platform / groupID 在内存里过滤一遍。
func (s *OpsService) ListAccountsForAlerts(ctx context.Context, platform string, groupID *int64, accountIDs []int64) ([]*Account, error) {
	if s == nil {
		return nil, errors.New("ops service is nil")
	}
	if s.listAccountsForAlerts != nil {
		return s.listAccountsForAlerts(ctx, platform, groupID, accountIDs)
	}
	if s.accountRepo == nil {
		return nil, errors.New("account repository not available")
	}
	platform = strings.ToLower(strings.TrimSpace(platform))

	if len(accountIDs) > 0 {
		accounts, err := s.accountRepo.GetByIDs(ctx, accountIDs)
		if err != nil {
			return nil, err
		}
		out := make([]*Account, 0, len(accounts))
		for _, account := range accounts {
			if account == nil {
				continue
			}
			if platform != "" && strings.ToLower(strings.TrimSpace(account.Platform)) != platform {
				continue
			}
			if groupID != nil && *groupID > 0 && !accountInGroup(account, *groupID) {
				continue
			}
			out = append(out, account)
		}
		return out, nil
	}

	var gid int64
	if groupID != nil && *groupID > 0 {
		gid = *groupID
	}
	accounts, err := s.accountRepo.ListAllWithFilters(ctx, platform, "", "", "", gid, "")
	if err != nil {
		return nil, err
	}
	out := make([]*Account, 0, len(accounts))
	for i := range accounts {
		out = append(out, &accounts[i])
	}
	return out, nil
}

func accountInGroup(account *Account, groupID int64) bool {
	if account == nil {
		return false
	}
	for _, id := range account.GroupIDs {
		if id == groupID {
			return true
		}
	}
	return false
}

func (s *OpsService) UpdateAlertEventEmailSent(ctx context.Context, eventID int64, emailSent bool) error {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return err
	}
	if s.opsRepo == nil {
		return infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if eventID <= 0 {
		return infraerrors.BadRequest("INVALID_EVENT_ID", "invalid event id")
	}
	return s.opsRepo.UpdateAlertEventEmailSent(ctx, eventID, emailSent)
}

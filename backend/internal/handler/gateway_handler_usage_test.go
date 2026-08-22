package handler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageUnrestrictedIncludesWeeklyWindowStart(t *testing.T) {
	weeklyWindowStart := time.Date(2026, time.July, 13, 0, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))

	handler := &GatewayHandler{}
	payload, err := handler.usageUnrestricted(
		context.Background(),
		&service.APIKey{Group: &service.Group{
			Name:             "Weekly plan",
			SubscriptionType: service.SubscriptionTypeSubscription,
		}},
		middleware.AuthSubject{},
		&service.UserSubscription{WeeklyWindowStart: &weeklyWindowStart},
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)

	encoded, err := json.Marshal(payload)
	require.NoError(t, err)

	var response struct {
		Subscription struct {
			WeeklyWindowStart *time.Time `json:"weekly_window_start"`
		} `json:"subscription"`
	}
	require.NoError(t, json.Unmarshal(encoded, &response))
	require.NotNil(t, response.Subscription.WeeklyWindowStart)
	require.True(t, weeklyWindowStart.Equal(*response.Subscription.WeeklyWindowStart))
}

package repository

import (
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestBuildOpsAlertEventsWhere_ObserveOverlapWindow(t *testing.T) {
	start := time.Date(2026, 9, 29, 2, 0, 0, 0, time.UTC)
	end := start.Add(6 * time.Hour)
	where, args := buildOpsAlertEventsWhere(&service.OpsAlertEventFilter{
		Urgency:       service.OpsAlertUrgencyObserve,
		StartTime:     &start,
		EndTime:       &end,
		OverlapWindow: true,
	})

	if !strings.Contains(where, "urgency = $") {
		t.Fatalf("missing urgency filter: %s", where)
	}
	if !strings.Contains(where, "fired_at < $") || !strings.Contains(where, "(resolved_at IS NULL OR resolved_at >= $") {
		t.Fatalf("missing overlap window predicate: %s", where)
	}
	if strings.Contains(where, "fired_at >= $") {
		t.Fatalf("overlap query must include events fired before the window: %s", where)
	}
	if len(args) != 3 {
		t.Fatalf("args len = %d, want 3: %#v", len(args), args)
	}
}

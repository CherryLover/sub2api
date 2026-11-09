//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

func TestOpenAIPrivacySchedulingGroupOnlyUsesMatchingAuthenticatedSnapshot(t *testing.T) {
	groupID := int64(51)
	matching := &Group{ID: groupID, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, RequirePrivacySet: true}
	ctx := context.WithValue(context.Background(), ctxkey.Group, matching)
	svc := &OpenAIGatewayService{}
	if got := svc.loadOpenAIPrivacySchedulingGroup(ctx, &groupID); got != matching {
		t.Fatalf("matching hydrated group snapshot not reused: %+v", got)
	}
	otherID := int64(52)
	if got := svc.loadOpenAIPrivacySchedulingGroup(ctx, &otherID); got != nil {
		t.Fatalf("wrong group snapshot was reused: %+v", got)
	}
	unhydrated := &Group{ID: groupID, Platform: PlatformOpenAI, Status: StatusActive, RequirePrivacySet: true}
	ctx = context.WithValue(context.Background(), ctxkey.Group, unhydrated)
	if got := svc.loadOpenAIPrivacySchedulingGroup(ctx, &groupID); got != nil {
		t.Fatalf("unhydrated snapshot used for privacy gate: %+v", got)
	}
}

package service

import (
	"context"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// The lite gateway already switches Gemini accounts on transport errors.
// Backport only the missing persistent-network-fault cooldown, without
// replacing the existing cached-content and countTokens exception paths.
func (s *GeminiMessagesCompatService) tempUnscheduleGeminiTransportError(ctx context.Context, account *Account, transportErr error) {
	if s == nil || account == nil || transportErr == nil ||
		errors.Is(transportErr, context.Canceled) || ctx.Err() != nil ||
		!classifyOpenAITransportError(transportErr).Persistent {
		return
	}

	until := time.Now().Add(openAITransportErrorTempUnschedDuration)
	reason := "upstream transport error (proxy/network): " + sanitizeUpstreamErrorMessage(transportErr.Error())
	// Keep the selected account's in-process cooldown even if the database
	// write fails; never shorten a previously longer cooldown.
	if account.TempUnschedulableUntil == nil || until.After(*account.TempUnschedulableUntil) {
		account.TempUnschedulableUntil = &until
		account.TempUnschedulableReason = reason
	}
	if s.accountRepo == nil {
		return
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIAccountStateUpdateTimeout)
	defer cancel()
	if err := s.accountRepo.SetTempUnschedulable(writeCtx, account.ID, until, reason); err != nil {
		logger.LegacyPrintf("service.gemini_transport", "temp unschedule failed account=%d: %v", account.ID, err)
	}
}

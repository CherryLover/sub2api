package service

import (
	"strconv"
	"strings"
)

const (
	OpsNoAccountReasonModelNotSupported = "model_not_supported"
	OpsNoAccountReasonModelRateLimited  = "model_rate_limited"
	OpsNoAccountReasonExcluded          = "excluded"
	OpsNoAccountReasonNotSchedulable    = "not_schedulable"
	OpsNoAccountReasonEmptyPool         = "empty_pool"
	OpsNoAccountReasonUnknown           = "unknown"
)

// ParseNoAccountReason extracts the dominant account-selection filter reason
// from the scheduler's diagnostic text. Selection details stay out of storage;
// only the normalized reason code is retained.
func ParseNoAccountReason(errText string) string {
	lower := strings.ToLower(strings.TrimSpace(errText))
	if lower == "" {
		return OpsNoAccountReasonUnknown
	}

	if filteredAt := strings.Index(lower, "filtered:"); filteredAt >= 0 {
		bestReason := ""
		bestCount := -1
		for _, rawField := range strings.Fields(lower[filteredAt+len("filtered:"):]) {
			field := strings.Trim(rawField, " ,()")
			reason, countText, ok := strings.Cut(field, "=")
			if !ok || reason == "" {
				continue
			}
			count, err := strconv.Atoi(countText)
			if err != nil {
				continue
			}
			if count > bestCount || (count == bestCount && (bestReason == "" || reason < bestReason)) {
				bestReason = reason
				bestCount = count
			}
		}
		if bestReason != "" {
			return bestReason
		}
	}

	for _, rawField := range strings.Fields(lower) {
		field := strings.Trim(rawField, " ,()")
		if field == "pool=0" {
			return OpsNoAccountReasonEmptyPool
		}
	}
	return OpsNoAccountReasonUnknown
}

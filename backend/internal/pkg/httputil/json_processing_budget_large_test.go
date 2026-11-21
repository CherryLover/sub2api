package httputil

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

// Models admission on a 2 GiB host with an explicitly configured 512 MiB
// budget. This is a logical reservation test, not a process RSS/OOM guarantee.
func TestJSONProcessingBudgetThreeLargeImagesOnExplicitSmallHostBudget(t *testing.T) {
	const bodyBytes = 46 << 20
	body := make([]byte, bodyBytes)
	for i := range body {
		body[i] = 'x'
	}
	copy(body, `{"image":"`)
	copy(body[len(body)-2:], `"}`)
	budget := NewBodyMemoryBudget(512 << 20)
	var contexts []context.Context
	var releases []func()
	for i := 0; i < 3; i++ {
		ctx, release := WithRequestBodyMemoryBudget(context.Background(), budget)
		contexts = append(contexts, ctx)
		releases = append(releases, release)
		require.NoError(t, ReserveRequestBodyMemory(ctx, int64(len(body))))
	}
	t.Cleanup(func() {
		for _, release := range releases {
			release()
		}
	})
	admitted := 0
	for _, ctx := range contexts {
		err := ReserveJSONProcessingMemory(ctx, body)
		if err == nil {
			admitted++
		} else {
			require.True(t, errors.Is(err, ErrRequestBodyMemoryBudget))
			require.False(t, errors.Is(err, ErrRequestBodyMemoryTooLarge))
		}
	}
	require.Equal(t, 1, admitted, "transform peaks, not just ingress bodies, must fit")
	budget.mu.Lock()
	used := budget.used
	budget.mu.Unlock()
	require.LessOrEqual(t, used, int64(512<<20))
	for _, release := range releases {
		release()
	}
	budget.mu.Lock()
	defer budget.mu.Unlock()
	require.Zero(t, budget.used)
}

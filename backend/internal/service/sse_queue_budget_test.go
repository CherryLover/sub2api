package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSSEQueueBudgetEnforcesBufferedBytesAndReleases(t *testing.T) {
	budget := newSSEQueueBudget(&config.Config{Gateway: config.GatewayConfig{SSEQueueMaxBytes: 64}})
	done := make(chan struct{})
	require.True(t, budget.acquire(40, done))
	require.EqualValues(t, 40, budget.used.Load())
	ready := make(chan bool, 1)
	go func() {
		ready <- budget.acquire(40, done)
	}()
	select {
	case <-ready:
		t.Fatal("queued bytes exceeded configured budget")
	case <-time.After(20 * time.Millisecond):
	}
	budget.release(40)
	select {
	case accepted := <-ready:
		require.True(t, accepted)
	case <-time.After(time.Second):
		t.Fatal("budget never released blocked sender")
	}
	budget.release(40)
	require.Zero(t, budget.used.Load())
}

func TestSSEQueueBudgetCanceledSenderDoesNotLeak(t *testing.T) {
	budget := newSSEQueueBudget(&config.Config{Gateway: config.GatewayConfig{SSEQueueMaxBytes: 16}})
	done := make(chan struct{})
	require.True(t, budget.acquire(16, done))
	ready := make(chan bool, 1)
	go func() { ready <- budget.acquire(16, done) }()
	close(done)
	select {
	case accepted := <-ready:
		require.False(t, accepted)
	case <-time.After(time.Second):
		t.Fatal("canceled sender stayed blocked")
	}
	budget.release(16)
	require.Zero(t, budget.used.Load())
}

func TestSSEQueueBudgetOversizedSingleLineCanPassAlone(t *testing.T) {
	budget := newSSEQueueBudget(&config.Config{Gateway: config.GatewayConfig{SSEQueueMaxBytes: 8}})
	done := make(chan struct{})
	require.True(t, budget.acquire(200, done))
	require.EqualValues(t, 8, budget.used.Load())
	budget.release(200)
	require.Zero(t, budget.used.Load())
}

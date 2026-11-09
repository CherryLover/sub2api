package httputil

import (
	"context"
	"errors"
	"sync"
)

// ErrRequestBodyMemoryBudget means this instance cannot buffer another request
// body yet. It is transient overload, not a malformed or oversized request.
var ErrRequestBodyMemoryBudget = errors.New("request body memory budget exhausted")

// BodyMemoryBudget bounds buffered ingress allocations across requests. It
// counts chunks, their contiguous copy, decoded output and the decoder window;
// it is not a bound on the application's total heap or protocol conversion.
type BodyMemoryBudget struct {
	mu    sync.Mutex
	limit int64
	used  int64
}

func NewBodyMemoryBudget(limit int64) *BodyMemoryBudget {
	return &BodyMemoryBudget{limit: limit}
}

type bodyMemoryReservation struct {
	mu     sync.Mutex
	budget *BodyMemoryBudget
	bytes  int64
	owners int
	closed bool
}

type bodyMemoryReservationKey struct{}

// WithRequestBodyMemoryBudget retains final body reservations until the HTTP
// handler returns, or until the last explicitly retained async task releases.
// Read/decode/normalize helpers release temporary allocations when no longer
// owned. No goroutines or wait queue are created on exhaustion.
func WithRequestBodyMemoryBudget(ctx context.Context, budget *BodyMemoryBudget) (context.Context, func()) {
	if budget == nil || budget.limit <= 0 || ctx.Value(bodyMemoryReservationKey{}) != nil {
		return ctx, func() {}
	}
	r := &bodyMemoryReservation{budget: budget, owners: 1}
	return context.WithValue(ctx, bodyMemoryReservationKey{}, r), requestBodyMemoryReleaseCallback(r)
}

// RetainRequestBodyMemoryBudget keeps the input body's reservation alive when
// an accepted asynchronous task continues using the bytes after its HTTP
// submission handler returns. The returned release callback is idempotent.
func RetainRequestBodyMemoryBudget(ctx context.Context) func() {
	if ctx == nil {
		return func() {}
	}
	r, _ := ctx.Value(bodyMemoryReservationKey{}).(*bodyMemoryReservation)
	if r == nil {
		return func() {}
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return func() {}
	}
	r.owners++
	r.mu.Unlock()
	return requestBodyMemoryReleaseCallback(r)
}

func requestBodyMemoryReleaseCallback(r *bodyMemoryReservation) func() {
	var once sync.Once
	return func() {
		once.Do(r.releaseOwner)
	}
}

func (r *bodyMemoryReservation) releaseOwner() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.owners <= 0 {
		return
	}
	r.owners--
	if r.owners > 0 {
		return
	}
	r.closed = true
	r.budget.mu.Lock()
	r.budget.used -= r.bytes
	r.budget.mu.Unlock()
	r.bytes = 0
}

func reserveRequestBodyMemory(ctx context.Context, n int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r, _ := ctx.Value(bodyMemoryReservationKey{}).(*bodyMemoryReservation)
	if r == nil || n <= 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrRequestBodyMemoryBudget
	}
	r.budget.mu.Lock()
	defer r.budget.mu.Unlock()
	if n > r.budget.limit-r.budget.used {
		return ErrRequestBodyMemoryBudget
	}
	r.budget.used += n
	r.bytes += n
	return nil
}

// releaseRequestBodyMemory refunds storage that a helper no longer owns. Each
// allocation must be released once, or left for the handler's final cleanup.
// Cleanup may race with a cancelled read, so a closed reservation is a no-op.
func releaseRequestBodyMemory(ctx context.Context, n int64) {
	r, _ := ctx.Value(bodyMemoryReservationKey{}).(*bodyMemoryReservation)
	if r == nil || n <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.budget.mu.Lock()
	r.budget.used -= n
	r.budget.mu.Unlock()
	r.bytes -= n
}

package httputil

import (
	"context"
	"errors"
	"math"
)

// ErrRequestBodyMemoryTooLarge is non-transient: the transformation could not
// fit even in an otherwise idle configured budget. It is distinct from 503.
var ErrRequestBodyMemoryTooLarge = errors.New("request body processing exceeds configured memory budget")

// ReserveJSONProcessingMemory admits conservative transformation headroom before
// decoding or rewriting JSON. An explicit zero/absent ingress budget remains a
// no-op. This estimates live representations, not the total process RSS.
// Repeated calls raise one high-water reservation; retries do not accumulate it.
func ReserveJSONProcessingMemory(ctx context.Context, body []byte) error {
	if ctx == nil {
		return nil
	}
	r, _ := ctx.Value(bodyMemoryReservationKey{}).(*bodyMemoryReservation)
	if r == nil || len(body) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	const copies, perStructure = int64(6), int64(128)
	if int64(len(body)) > math.MaxInt64/copies {
		return ErrRequestBodyMemoryTooLarge
	}
	n := int64(len(body)) * copies
	quoted := false
	for i := 0; i < len(body); i++ {
		b := body[i]
		if quoted {
			switch b {
			case '\\':
				i++
			case '"':
				quoted = false
			}
			continue
		}
		if b == '"' {
			quoted = true
			continue
		}
		switch b {
		case '{', '[', ':', ',':
			if n > math.MaxInt64-perStructure {
				return ErrRequestBodyMemoryTooLarge
			}
			n += perStructure
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrRequestBodyMemoryBudget
	}
	settled := int64(len(body))
	if n <= r.processingBytes {
		r.processingSettled = max(r.processingSettled, settled)
		return nil
	}
	delta := n - r.processingBytes
	r.budget.mu.Lock()
	defer r.budget.mu.Unlock()
	held := max(r.bytes-r.processingBytes, 0)
	if n > r.budget.limit-held {
		return ErrRequestBodyMemoryTooLarge
	}
	if delta > r.budget.limit-r.budget.used {
		return ErrRequestBodyMemoryBudget
	}
	r.budget.used += delta
	r.bytes += delta
	r.processingBytes = n
	r.processingSettled = max(r.processingSettled, settled)
	return nil
}

// SettleJSONProcessingMemory releases only transformation headroom after a
// successful upstream handoff. Ingress plus one outgoing representation remain
// charged until their owners release. A later retry must reserve its peak again.
func SettleJSONProcessingMemory(ctx context.Context) {
	if ctx == nil {
		return
	}
	r, _ := ctx.Value(bodyMemoryReservationKey{}).(*bodyMemoryReservation)
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.processingBytes <= r.processingSettled {
		return
	}
	delta := r.processingBytes - r.processingSettled
	r.budget.mu.Lock()
	r.budget.used -= delta
	r.budget.mu.Unlock()
	r.bytes -= delta
	r.processingBytes = r.processingSettled
}

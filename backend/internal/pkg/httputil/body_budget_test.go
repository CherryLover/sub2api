package httputil

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestRequestBodyMemoryBudgetSharedUntilRelease(t *testing.T) {
	budget := NewBodyMemoryBudget(1024)
	ctx, release := WithRequestBodyMemoryBudget(context.Background(), budget)
	first := httptest.NewRequest("POST", "/", bytes.NewReader(bytes.Repeat([]byte{'x'}, 700))).WithContext(ctx)
	data, err := ReadRequestBodyWithPrealloc(first)
	if err != nil || len(data) != 700 {
		t.Fatalf("first request len=%d err=%v", len(data), err)
	}
	ctx2, release2 := WithRequestBodyMemoryBudget(context.Background(), budget)
	defer release2()
	second := httptest.NewRequest("POST", "/", bytes.NewReader(bytes.Repeat([]byte{'x'}, 700))).WithContext(ctx2)
	if _, err := ReadRequestBodyWithPrealloc(second); !errors.Is(err, ErrRequestBodyMemoryBudget) {
		t.Fatalf("want budget exhaustion, got %v", err)
	}
	release()
	release() // Idempotent: cannot refund a second request's reservation.
	if _, err := ReadRequestBodyWithPrealloc(second); err != nil {
		t.Fatal(err)
	}
}

func TestRequestBodyMemoryBudgetConcurrentReservationsRefunded(t *testing.T) {
	budget := NewBodyMemoryBudget(4096)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, release := WithRequestBodyMemoryBudget(context.Background(), budget)
			defer release()
			_ = reserveRequestBodyMemory(ctx, 1024)
		}()
	}
	wg.Wait()
	if budget.used != 0 {
		t.Fatalf("leaked reservations: %d", budget.used)
	}
}

func TestCompressedAndNormalizedRequestBodiesRespectBudget(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(bytes.Repeat([]byte{'x'}, 4096)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	budget := NewBodyMemoryBudget(2048)
	ctx, release := WithRequestBodyMemoryBudget(context.Background(), budget)
	req := httptest.NewRequest("POST", "/", bytes.NewReader(compressed.Bytes())).WithContext(ctx)
	req.Header.Set("Content-Encoding", "gzip")
	if _, err := ReadRequestBodyWithPrealloc(req); !errors.Is(err, ErrRequestBodyMemoryBudget) {
		t.Fatalf("decoded expansion bypassed memory budget: %v", err)
	}
	release()
	if budget.used != 0 {
		t.Fatalf("failed decompression retained %d bytes", budget.used)
	}

	payload := []byte(`{"input":"` + strings.Repeat("\x00", 100) + `"}`)
	ctx, release = WithRequestBodyMemoryBudget(context.Background(), NewBodyMemoryBudget(600))
	defer release()
	req = httptest.NewRequest("POST", "/", bytes.NewReader(payload)).WithContext(ctx)
	if _, err := ReadLenientJSONRequestBodyWithPrealloc(req, 4096); !errors.Is(err, ErrRequestBodyMemoryBudget) {
		t.Fatalf("normalized expansion bypassed memory budget: %v", err)
	}
}

func TestMemoryBudgetDisabledPreservesExistingBodyBehavior(t *testing.T) {
	budget := NewBodyMemoryBudget(0)
	ctx, release := WithRequestBodyMemoryBudget(context.Background(), budget)
	defer release()
	req := httptest.NewRequest("POST", "/", bytes.NewReader(bytes.Repeat([]byte{'x'}, 4096))).WithContext(ctx)
	data, err := ReadRequestBodyWithPrealloc(req)
	if err != nil || len(data) != 4096 {
		t.Fatalf("disabled budget changed legacy behavior: %d, %v", len(data), err)
	}
}

func TestRequestBodyMemoryBudgetRetainedByAsyncTask(t *testing.T) {
	budget := NewBodyMemoryBudget(1024)
	ctx, finishHTTP := WithRequestBodyMemoryBudget(context.Background(), budget)
	input := httptest.NewRequest("POST", "/", bytes.NewReader(bytes.Repeat([]byte{'x'}, 700))).WithContext(ctx)
	body, err := ReadRequestBodyWithPrealloc(input)
	if err != nil || len(body) != 700 {
		t.Fatalf("initial async input: len=%d err=%v", len(body), err)
	}
	finishAsync := RetainRequestBodyMemoryBudget(ctx)
	finishHTTP()
	finishHTTP()
	anotherCtx, finishOther := WithRequestBodyMemoryBudget(context.Background(), budget)
	defer finishOther()
	other := httptest.NewRequest("POST", "/", bytes.NewReader(bytes.Repeat([]byte{'y'}, 700))).WithContext(anotherCtx)
	if _, err := ReadRequestBodyWithPrealloc(other); !errors.Is(err, ErrRequestBodyMemoryBudget) {
		t.Fatalf("async input was prematurely unreserved: %v", err)
	}
	finishAsync()
	finishAsync()
	if _, err := ReadRequestBodyWithPrealloc(other); err != nil {
		t.Fatalf("async task did not release body allocation: %v", err)
	}
}

func TestRequestBodyMemoryScopeRefundsPerAttempt(t *testing.T) {
	budget := NewBodyMemoryBudget(1024)
	ctx, finishHTTP := WithRequestBodyMemoryBudget(context.Background(), budget)
	defer finishHTTP()
	for attempt := 0; attempt < 3; attempt++ {
		attemptCtx, finishAttempt := WithRequestBodyMemoryScope(ctx)
		if err := ReserveRequestBodyMemory(attemptCtx, 700); err != nil {
			t.Fatalf("attempt %d inherited previous budget: %v", attempt, err)
		}
		if err := ReserveRequestBodyMemory(attemptCtx, 500); !errors.Is(err, ErrRequestBodyMemoryBudget) {
			t.Fatalf("attempt %d exceeded budget: %v", attempt, err)
		}
		ReleaseRequestBodyMemory(attemptCtx, 100)
		ReleaseRequestBodyMemory(attemptCtx, 999) // cannot underflow or refund another request
		finishAttempt()
		finishAttempt()
	}
	if budget.used != 0 {
		t.Fatalf("attempt allocations leaked: %d", budget.used)
	}
	finishHTTP()
	closedScope, release := WithRequestBodyMemoryScope(ctx)
	defer release()
	if err := ReserveRequestBodyMemory(closedScope, 10); !errors.Is(err, ErrRequestBodyMemoryBudget) {
		t.Fatalf("closed request accepted an attempt allocation: %v", err)
	}
}

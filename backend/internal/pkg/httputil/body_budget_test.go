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

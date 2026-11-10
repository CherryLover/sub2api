//go:build unit

package middleware

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/gin-gonic/gin"
)

func TestGatewayBodyBudgetHeldUntilHandlerReturns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	budget := pkghttputil.NewBodyMemoryBudget(1024)
	router := gin.New()
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	router.POST("/body", RequestBodyLimit(4096, budget), func(c *gin.Context) {
		data, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
		if errors.Is(err, pkghttputil.ErrRequestBodyMemoryBudget) {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		if err != nil || len(data) != 700 {
			c.Status(http.StatusBadRequest)
			return
		}
		once.Do(func() { close(entered) })
		<-release
		c.Status(http.StatusOK)
	})
	makeRequest := func() *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/body", bytes.NewReader(bytes.Repeat([]byte{'x'}, 700)))
		router.ServeHTTP(r, request)
		return r
	}
	firstDone := make(chan int, 1)
	go func() { firstDone <- makeRequest().Code }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("initial body read did not complete under the memory budget")
	}
	if got := makeRequest().Code; got != http.StatusServiceUnavailable {
		close(release)
		t.Fatalf("concurrent request status=%d, want 503", got)
	}
	close(release)
	if got := <-firstDone; got != http.StatusOK {
		t.Fatalf("first status=%d, want 200", got)
	}
	if got := makeRequest().Code; got != http.StatusOK {
		t.Fatalf("budget not released for next request: %d", got)
	}
}

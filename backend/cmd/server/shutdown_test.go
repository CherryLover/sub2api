package main

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestShutdownHTTPServerClosesTimedOutStream(t *testing.T) {
	entered := make(chan struct{})
	canceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, ": ready\n\n")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("stream writer does not support flushing")
			return
		}
		flusher.Flush()
		close(entered)
		<-r.Context().Done()
		close(canceled)
	}))
	defer server.Close()
	resp, err := server.Client().Get(server.URL)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	<-entered
	err = shutdownHTTPServer(server.Config, 20*time.Millisecond)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Error("drain timeout returned while the stream was still live")
		server.CloseClientConnections()
	}
}
func TestShutdownHTTPServerLetsActiveRequestFinish(t *testing.T) {
	entered, finish := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-finish:
			_, _ = io.WriteString(w, "complete")
		case <-r.Context().Done():
			t.Error("request canceled before drain timeout")
		}
	}))
	defer server.Close()
	result := make(chan error, 1)
	go func() {
		resp, err := server.Client().Get(server.URL)
		if err != nil {
			result <- err
			return
		}
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		if string(b) != "complete" {
			err = errors.New("incomplete response")
		}
		result <- err
	}()
	<-entered
	stopped := make(chan error, 1)
	go func() { stopped <- shutdownHTTPServer(server.Config, time.Second) }()
	close(finish)
	require.NoError(t, <-result)
	require.NoError(t, <-stopped)
}
func TestShutdownHTTPServerNilIsSafe(t *testing.T) {
	require.NoError(t, shutdownHTTPServer(nil, time.Second))
}

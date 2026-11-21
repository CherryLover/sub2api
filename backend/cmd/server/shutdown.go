package main

import (
	"context"
	"errors"
	"net/http"
	"time"
)

func shutdownHTTPServer(server *http.Server, timeout time.Duration) error {
	if server == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		// Shutdown stops accepting but does not close active long streams after
		// the deadline. Close cancels their HTTP request contexts before cleanup.
		return errors.Join(err, server.Close())
	}
	return nil
}

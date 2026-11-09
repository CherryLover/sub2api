package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGrokVoiceBodyReadErrorClassification(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{"read timeout", os.ErrDeadlineExceeded, http.StatusRequestTimeout, "Timed out reading request body"},
		{"short body", io.ErrUnexpectedEOF, http.StatusBadRequest, "Failed to read request body"},
		{"disconnect", context.Canceled, 499, "Client closed the connection before the request body was received"},
		{"generic", errors.New("socket token=secret@10.0.0.1"), http.StatusBadRequest, "Failed to read request body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg := requestBodyReadFailure(nil, httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil), tt.err)
			if status != tt.status || msg != tt.message {
				t.Fatalf("got status %d, message %q, want %d %q", status, msg, tt.status, tt.message)
			}
		})
	}
}

func TestGrokVoiceMissingBodyRetainsValidationError(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	c.Request.Body = nil
	_, err := readGrokVoiceGatewayBody(c)
	if !errors.Is(err, errGrokVoiceBodyRequired) {
		t.Fatalf("missing body error = %v, want required body sentinel", err)
	}
}

package middleware

import (
	"net/http"
	"testing"
)

func TestAsyncVideoTaskReadDoesNotExemptMutations(t *testing.T) {
	for _, route := range []string{
		"/v1/videos/:request_id",
		"/v1/videos/:request_id/content",
		"/v1/videos/edits/:request_id",
		"/v1/videos/extensions/:request_id/content",
		"/api/v3/contents/generations/tasks/:task_id",
	} {
		if !isAsyncVideoTaskRead(http.MethodGet, route) {
			t.Errorf("GET route %q should be treated as a task read", route)
		}
		for _, method := range []string{http.MethodPost, http.MethodDelete, http.MethodPut, http.MethodPatch} {
			if isAsyncVideoTaskRead(method, route) {
				t.Errorf("%s route %q must still enforce billing eligibility", method, route)
			}
		}
	}
	for _, route := range []string{"/v1/videos", "/v1/videos/:request_id/cancel", "/v1/images/generations", "/v1/videos/:request_id/unknown"} {
		if isAsyncVideoTaskRead(http.MethodGet, route) {
			t.Errorf("non-read route %q must not be exempted", route)
		}
	}
}

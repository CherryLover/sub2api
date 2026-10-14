package service

import "testing"

func TestVertexSharedCapacity429ClassificationLite(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"transient", `{"error":{"status":"RESOURCE_EXHAUSTED","message":"Resource exhausted. Please try again later."}}`, true},
		{"array_wrapper", `[{"error":{"status":"RESOURCE_EXHAUSTED","message":"Resource exhausted, please try again later."}}]`, true},
		{"project_quota", `{"error":{"status":"RESOURCE_EXHAUSTED","message":"Resource exhausted. Project quota exceeded. Please try again later."}}`, false},
		{"throughput", `{"error":{"status":"RESOURCE_EXHAUSTED","message":"Resource exhausted. Provisioned Throughput. Please try again later."}}`, false},
		{"retry_info", `{"error":{"status":"RESOURCE_EXHAUSTED","message":"Resource exhausted. Please try again later.","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"4s"}]}}`, false},
		{"other", `{"error":{"status":"UNAVAILABLE","message":"Resource exhausted. Please try again later."}}`, false},
		{"malformed", `null`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isVertexSharedCapacity429([]byte(tc.body)); got != tc.want {
				t.Errorf("isVertexSharedCapacity429() = %v, want %v", got, tc.want)
			}
		})
	}
}

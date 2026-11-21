package service

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"net/http"
)

// A failed send may trigger response-dependent JSON repair on this same
// attempt, so keep its peak. A successful stream only retains owned bodies.
func (s *OpenAIGatewayService) doOpenAIProcessingUpstream(req *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		httputil.SettleJSONProcessingMemory(req.Context())
	}
	return resp, err
}

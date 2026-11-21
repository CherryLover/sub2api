package service

import (
	"context"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// compositeClientModel restores only an explicitly rewritten public route name.
// The upstream observation and billing model remain separate from this label.
func compositeClientModel(ctx context.Context, fallback string) string {
	publicModel, ok := RequestedPublicModelFromContext(ctx)
	if !ok {
		return fallback
	}
	if upstreamModel, ok := ResolvedUpstreamModelFromContext(ctx); !ok || upstreamModel == publicModel {
		return fallback
	}
	return publicModel
}
func restoreCompositeClientModel(ctx context.Context, body []byte) []byte {
	clientModel := compositeClientModel(ctx, "")
	if clientModel == "" || gjson.GetBytes(body, "model").Type != gjson.String {
		return body
	}
	if rewritten, err := sjson.SetBytes(body, "model", clientModel); err == nil {
		return rewritten
	}
	return body
}

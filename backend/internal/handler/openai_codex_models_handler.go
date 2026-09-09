package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// CodexModels serves the Codex models manifest for Codex clients.
//
// Codex CLI and the Codex desktop app refresh their model picker from
// GET {base_url}/models?client_version=... (custom provider mode) or
// GET /backend-api/codex/models (chatgpt_base_url mode). Both routes land
// here. Groups with explicit mappings are generated locally; otherwise ChatGPT
// manifests are proxied verbatim and custom API key manifests receive
// provider-compatibility normalization plus short-lived caching.
func (h *OpenAIGatewayHandler) CodexModels(c *gin.Context) {
	if c.Request.Context().Err() != nil {
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey.Group == nil {
		h.errorResponse(c, http.StatusUnauthorized, "invalid_request_error", "API key group is required")
		return
	}
	if apiKey.Group.Platform != service.PlatformOpenAI && apiKey.Group.Platform != service.PlatformComposite {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Codex models manifest is only available for OpenAI and Composite groups")
		return
	}

	ifNoneMatch := c.GetHeader("If-None-Match")
	// 固定账号分支：开启后只用选定账号拉取 manifest，不经过调度器；
	// 全部不可用/全部失败时按 FallbackToScheduler 决定回退调度器或返回错误。
	// 显式固定账号优先于分组映射本地生成的 catalog。
	if apiKey.Group.Platform == service.PlatformOpenAI &&
		apiKey.Group.CodexModelsManifestConfig.Enabled &&
		len(apiKey.Group.CodexModelsManifestConfig.AccountIDs) > 0 {
		pinnedManifest, pinnedAccount, pinnedErr := h.gatewayService.FetchPinnedCodexModelsManifest(
			c.Request.Context(),
			apiKey.Group,
			c.Query("client_version"),
		)
		if pinnedErr != nil {
			if c.Request.Context().Err() != nil {
				return
			}
			if !apiKey.Group.CodexModelsManifestConfig.FallbackToScheduler {
				if errors.Is(pinnedErr, service.ErrNoPinnedCodexModelsAccounts) {
					h.errorResponse(c, http.StatusServiceUnavailable, "upstream_error", "No available pinned OpenAI accounts")
					return
				}
				h.errorResponse(c, infraerrors.Code(pinnedErr), "upstream_error", infraerrors.Message(pinnedErr))
				return
			}
		} else {
			setOpsSelectedAccount(c, pinnedAccount.ID, pinnedAccount.Platform)
			if c.Request.Context().Err() != nil {
				return
			}
			writeCodexModelsManifest(c, pinnedManifest)
			return
		}
	}

	if apiKey.Group.Platform == service.PlatformOpenAI {
		configuredManifest, configured, err := h.gatewayService.BuildGroupConfiguredCodexModelsManifest(
			c.Request.Context(),
			apiKey.Group,
			ifNoneMatch,
		)
		if err != nil {
			if c.Request.Context().Err() != nil {
				return
			}
			h.errorResponse(c, http.StatusInternalServerError, "api_error", "Failed to build Codex models manifest")
			return
		}
		if configured {
			writeCodexModelsManifest(c, configuredManifest)
			return
		}
	}

	maxAccountSwitches := h.maxAccountSwitches
	if maxAccountSwitches <= 0 {
		maxAccountSwitches = 3
	}
	failedAccountIDs := make(map[int64]struct{})
	switchCount := 0
	var lastUpstreamErr error

	for {
		account, err := h.gatewayService.SelectAccountForModelWithExclusions(c.Request.Context(), apiKey.GroupID, "", "", failedAccountIDs)
		if err != nil {
			if c.Request.Context().Err() != nil {
				return
			}
			if lastUpstreamErr != nil {
				h.errorResponse(c, infraerrors.Code(lastUpstreamErr), "upstream_error", infraerrors.Message(lastUpstreamErr))
				return
			}
			h.errorResponse(c, http.StatusServiceUnavailable, "upstream_error", "No available OpenAI accounts")
			return
		}
		// 让 ops 错误日志携带实际选中的上游账号，便于定位失效账号（#4544）。
		setOpsSelectedAccount(c, account.ID, account.Platform)

		manifest, err := h.gatewayService.FetchCodexModelsManifest(c.Request.Context(), account, c.Query("client_version"), ifNoneMatch)
		if err != nil {
			if c.Request.Context().Err() != nil {
				return
			}
			if service.IsRetryableCodexModelsManifestError(err) && switchCount < maxAccountSwitches {
				failedAccountIDs[account.ID] = struct{}{}
				switchCount++
				lastUpstreamErr = err
				continue
			}
			h.errorResponse(c, infraerrors.Code(err), "upstream_error", infraerrors.Message(err))
			return
		}
		if c.Request.Context().Err() != nil {
			return
		}

		writeCodexModelsManifest(c, manifest)
		return
	}
}

func writeCodexModelsManifest(c *gin.Context, manifest *service.CodexModelsManifest) {
	if manifest == nil {
		return
	}
	if manifest.ETag != "" {
		c.Header("ETag", manifest.ETag)
	}
	if manifest.NotModified {
		c.Status(http.StatusNotModified)
		return
	}
	c.Data(http.StatusOK, "application/json", manifest.Body)
}

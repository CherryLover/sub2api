package securityaudit

import (
	"context"
	"errors"
	"net/http"
)

type PromptEngine interface {
	EffectiveMode() Mode
	Enqueue(ctx context.Context, req Request) error
	Evaluate(ctx context.Context, req Request) (*PromptDecision, error)
}

type Coordinator struct {
	prompt PromptEngine
}

func NewCoordinator(prompt PromptEngine) *Coordinator {
	return &Coordinator{prompt: prompt}
}

func (c *Coordinator) Check(ctx context.Context, req Request) Decision {
	if c == nil {
		return allowDecision(nil)
	}
	mode := ModeOff
	if c.prompt != nil {
		mode = c.prompt.EffectiveMode()
	}
	switch mode {
	case ModeAsync:
		// Enqueue is deliberately best-effort. The implementation owns a bounded
		// context and copies request memory before it can outlive the Handler.
		_ = c.prompt.Enqueue(ctx, req.Clone())
		return allowDecision(nil)
	case ModeBlocking:
		return c.checkBlocking(ctx, req)
	default:
		return allowDecision(nil)
	}
}

func (c *Coordinator) checkBlocking(ctx context.Context, req Request) Decision {
	if c.prompt == nil {
		return prioritize(unavailablePromptDecision(ErrorCodeUnavailable))
	}
	result, err := c.prompt.Evaluate(ctx, req.Clone())
	if err != nil {
		var guardErr *GuardError
		if errors.As(err, &guardErr) && guardErr.Code == ErrorCodeInvalidResponse {
			return prioritize(unavailablePromptDecision(ErrorCodeInvalidResponse))
		}
		return prioritize(unavailablePromptDecision(ErrorCodeUnavailable))
	}
	if result == nil {
		return prioritize(unavailablePromptDecision(ErrorCodeUnavailable))
	}
	return prioritize(result)
}

func prioritize(prompt *PromptDecision) Decision {
	if prompt == nil {
		return allowDecision(nil)
	}
	switch prompt.Kind {
	case DecisionBlock:
		return Decision{Kind: DecisionBlock, HTTPStatus: http.StatusForbidden, ErrorCode: ErrorCodeBlocked,
			ClientMessage: "提示词安全审计拒绝了该请求，请调整输入后重试", Prompt: prompt}
	case DecisionInvalid:
		return Decision{Kind: DecisionInvalid, HTTPStatus: http.StatusServiceUnavailable, ErrorCode: ErrorCodeInvalidResponse,
			ClientMessage: "提示词安全审计暂时不可用，请稍后重试", Prompt: prompt}
	case DecisionUnavailable:
		return Decision{Kind: DecisionUnavailable, HTTPStatus: http.StatusServiceUnavailable, ErrorCode: ErrorCodeUnavailable,
			ClientMessage: "提示词安全审计暂时不可用，请稍后重试", Prompt: prompt}
	case DecisionFlag:
		return Decision{Kind: DecisionFlag, HTTPStatus: http.StatusOK, Prompt: prompt, AllowNextStage: true}
	default:
		return allowDecision(prompt)
	}
}

func allowDecision(prompt *PromptDecision) Decision {
	return Decision{Kind: DecisionAllow, HTTPStatus: http.StatusOK, Prompt: prompt, AllowNextStage: true}
}

func unavailablePromptDecision(code string) *PromptDecision {
	kind := DecisionUnavailable
	if code == ErrorCodeInvalidResponse {
		kind = DecisionInvalid
	}
	return &PromptDecision{Kind: kind, ErrorCode: code, AllowNextStage: false}
}

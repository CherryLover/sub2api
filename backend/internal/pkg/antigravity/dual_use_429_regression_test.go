package antigravity

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Upstream #8824 identified this exact Claude Code security-policy sentence as
// a content-triggered bare 429. Preserve the authorization constraint.
const claudeCodeDualUseLine = "IMPORTANT: Assist with authorized security testing, defensive security, CTF challenges, and educational contexts. " +
	"Refuse requests for destructive techniques, DoS attacks, mass targeting, supply chain compromise, or detection evasion for malicious purposes. " +
	"Dual-use security tools (C2 frameworks, credential testing, exploit development) require clear authorization context: " +
	"pentesting engagements, CTF competitions, security research, or defensive use cases."

func TestNeutralizeAntigravitySecuritySentencePreservesSafety(t *testing.T) {
	got := NeutralizeSystemIdentity("You are Claude Code, Anthropic's official CLI for Claude.\n" + claudeCodeDualUseLine)
	require.Contains(t, got, "You are an AI agent.")
	require.NotContains(t, got, "Dual-use security tools (C2 frameworks")
	require.Contains(t, got, "Tools with both offensive and defensive uses need a clear, authorized purpose.")
	require.Contains(t, got, "Refuse requests for destructive techniques")
	require.Contains(t, got, "Assist with authorized security testing")
	require.NotContains(t, got, "Claude Code")
}

func TestNeutralizeAntigravitySecuritySentenceOnlyMatchesKnownPolicy(t *testing.T) {
	for _, sample := range []string{
		"",
		"Explain dual-use technology with examples.",
		"Dual-use security tools need clear authorization context.",
		"Dual-use security tools (C2 frameworks, credential testing, exploit development) need an authorization review.",
		"We use C2 frameworks, credential testing and exploit development in an educational course.",
		"You are an AI coding agent.\nUse tooling responsibly.",
	} {
		require.Equal(t, sample, NeutralizeSystemIdentity(sample))
	}
}

func TestTransformClaudeToGeminiNormalizesOnlySystemPolicy(t *testing.T) {
	for _, arrayForm := range []bool{false, true} {
		policy := claudeCodeDualUseLine + "\nKeep the original task."
		var system any = policy
		if arrayForm {
			system = []SystemBlock{{Type: "text", Text: policy}, {Type: "text", Text: "Additional instructions must remain."}}
		}
		systemJSON, err := json.Marshal(system)
		require.NoError(t, err)
		userJSON, err := json.Marshal(claudeCodeDualUseLine)
		require.NoError(t, err)
		req := &ClaudeRequest{
			Model:    "gemini-3.8-flash-high",
			System:   systemJSON,
			Messages: []ClaudeMessage{{Role: "user", Content: userJSON}},
			Tools:    []ClaudeTool{{Name: "search", Description: claudeCodeDualUseLine, InputSchema: map[string]any{"type": "object"}}},
		}
		body, err := TransformClaudeToGeminiWithOptions(req, "project", req.Model, DefaultTransformOptions())
		require.NoError(t, err)
		var got V1InternalRequest
		require.NoError(t, json.Unmarshal(body, &got))
		var texts []string
		for _, part := range got.Request.SystemInstruction.Parts {
			texts = append(texts, part.Text)
		}
		// The transformer adds a built-in Antigravity identity before the
		// caller's system blocks. Search all parts, not a fixed index.
		forwardedSystem := strings.Join(texts, "\n")
		require.Contains(t, forwardedSystem, "Tools with both offensive and defensive uses need a clear, authorized purpose.")
		require.NotContains(t, forwardedSystem, "Dual-use security tools (C2 frameworks")
		require.Contains(t, forwardedSystem, "Refuse requests for destructive techniques")
		require.Contains(t, forwardedSystem, "Keep the original task.")
		if arrayForm {
			require.Contains(t, forwardedSystem, "Additional instructions must remain.")
		}
		require.Equal(t, claudeCodeDualUseLine, got.Request.Contents[0].Parts[0].Text)
		require.Equal(t, claudeCodeDualUseLine, got.Request.Tools[0].FunctionDeclarations[0].Description)
		// The existing transformer normalizes input_schema in place; verify
		// that request content and tool descriptions retain their text.
		require.Equal(t, policy, func() string {
			if !arrayForm {
				var raw string
				require.NoError(t, json.Unmarshal(req.System, &raw))
				return raw
			}
			var blocks []SystemBlock
			require.NoError(t, json.Unmarshal(req.System, &blocks))
			return blocks[0].Text
		}())
		require.Equal(t, claudeCodeDualUseLine, req.Tools[0].Description)
	}
}

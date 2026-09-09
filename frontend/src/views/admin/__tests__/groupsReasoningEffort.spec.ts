import { describe, expect, it } from "vitest";

import {
  createReasoningEffortMappingRow,
  normalizeReasoningEffortForPlatform,
  normalizeReasoningEffortOverLimit,
  reasoningEffortMappingsToAPI,
  reasoningEffortMappingsToRows,
  reasoningEffortOptionsForPlatform,
  reasoningEffortOverLimitDeny,
  reasoningEffortOverLimitDowngrade,
  supportsReasoningEffortPolicyPlatform,
  validateReasoningEffortMappings,
} from "../groupsReasoningEffort";

describe("groupsReasoningEffort", () => {
  it("provides fixed OpenAI choices to OpenAI and Composite groups", () => {
    const expected = [
      "minimal",
      "low",
      "medium",
      "high",
      "xhigh",
      "max",
    ];
    for (const platform of ["openai", "composite"] as const) {
      expect(
        reasoningEffortOptionsForPlatform(platform).map(
          (option) => option.value,
        ),
      ).toEqual(expected);
      expect(supportsReasoningEffortPolicyPlatform(platform)).toBe(true);
    }
    for (const platform of [
      "anthropic",
      "gemini",
      "antigravity",
      "grok",
    ] as const) {
      expect(reasoningEffortOptionsForPlatform(platform)).toEqual([]);
      expect(supportsReasoningEffortPolicyPlatform(platform)).toBe(false);
    }
  });

  it("hydrates supported rows and drops stale custom values", () => {
    const rows = reasoningEffortMappingsToRows(
      [
        { from: " max ", to: " xhigh " },
        { from: "ultra", to: "high" },
        { from: "none", to: "low" },
        { from: "high", to: "deny", match_type: "prefix", model: "gpt-5" },
      ],
      "openai",
    );

    expect(reasoningEffortMappingsToAPI(rows)).toEqual([
      { from: "max", to: "xhigh" },
      { from: "none", to: "low" },
      { from: "high", to: "deny", match_type: "prefix", model: "gpt-5" },
    ]);
  });

  it("clears values unsupported by OpenAI or used on another platform", () => {
    expect(normalizeReasoningEffortForPlatform("openai", " MAX ")).toBe("max");
    expect(normalizeReasoningEffortForPlatform("composite", " MAX ")).toBe(
      "max",
    );
    expect(normalizeReasoningEffortForPlatform("grok", "max")).toBe("");
    expect(normalizeReasoningEffortForPlatform("openai", "none")).toBe("");
    expect(normalizeReasoningEffortOverLimit("deny")).toBe(
      reasoningEffortOverLimitDeny,
    );
    expect(normalizeReasoningEffortOverLimit("")).toBe(
      reasoningEffortOverLimitDowngrade,
    );
  });

  it("requires both sides of every mapping", () => {
    const row = createReasoningEffortMappingRow({ to: "low" });
    row.pairs.push({
      id: "pair-missing-to",
      from: "max",
      to: "",
    });

    expect(validateReasoningEffortMappings([row])).toEqual({
      [row.pairs[0].id]: { from: "fromRequired" },
      "pair-missing-to": { to: "toRequired" },
    });
  });

  it("rejects duplicate source values case insensitively", () => {
    const row = createReasoningEffortMappingRow({ from: "MAX", to: "xhigh" });
    row.pairs.push({
      id: "pair-dup",
      from: " max ",
      to: "high",
    });

    expect(validateReasoningEffortMappings([row])).toEqual({
      [row.pairs[0].id]: { from: "duplicateFrom" },
      "pair-dup": { from: "duplicateFrom" },
    });
  });

  it("rejects custom mappings", () => {
    const row = createReasoningEffortMappingRow({ from: "ultra", to: "high" });
    expect(validateReasoningEffortMappings([row], "openai")).toEqual({
      [row.pairs[0].id]: { from: "unsupportedFrom" },
    });
  });
});

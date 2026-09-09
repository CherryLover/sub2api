import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

import { describe, expect, it } from "vitest";

const currentDir = dirname(fileURLToPath(import.meta.url));
const groupsViewSource = readFileSync(
  resolve(currentDir, "../GroupsView.vue"),
  "utf8",
);

describe("groups models list layout", () => {
  it("keeps the toolbar outside of the scrolling list content", () => {
    expect(groupsViewSource).toContain("overflow-hidden rounded-lg border");
    expect(groupsViewSource).toContain("max-h-64 space-y-2 overflow-y-auto p-2");
    expect(groupsViewSource).not.toContain("sticky top-0");
  });

  it("uses the Gemini-native models endpoint in Gemini group copy", () => {
    expect(groupsViewSource).toContain(
      'platform === "gemini" ? "/v1beta/models" : "/v1/models"',
    );
    expect(groupsViewSource).toContain("modelAllowlistEndpoint(createForm.platform)");
    expect(groupsViewSource).toContain("modelAllowlistEndpoint(editForm.platform)");
  });

  it("uses a wide dialog and keeps model pricing controls responsive", () => {
    expect(groupsViewSource).toContain('width="wide"');
    expect(groupsViewSource).toContain(
      "btn btn-secondary shrink-0 whitespace-nowrap",
    );
  });

  it("shows standardized API messages when creating or updating a group fails", () => {
    expect(groupsViewSource).toContain(
      'extractApiErrorMessage(error, t("admin.groups.failedToCreate"))',
    );
    expect(groupsViewSource).toContain(
      'extractApiErrorMessage(error, t("admin.groups.failedToUpdate"))',
    );
  });
});

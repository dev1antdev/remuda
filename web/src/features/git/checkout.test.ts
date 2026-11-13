import { describe, expect, it } from "vitest";
import { gitLensTip, isCheckout } from "./checkout";

describe("isCheckout", () => {
  it("reads a branch the sweep found or a worktree herdr names as a checkout", () => {
    expect(isCheckout({ branch: "main" })).toBe(true);
    expect(isCheckout({ branch: "1a2b3c4" })).toBe(true);
    expect(isCheckout({ worktree: { repo_key: "k", repo_name: "app", linked: true, checkout_path: "/w" } })).toBe(true);
    expect(isCheckout({})).toBe(false);
    expect(gitLensTip(false)).toMatch(/not a git checkout/);
  });
});

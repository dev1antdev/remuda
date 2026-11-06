import { describe, expect, it } from "vitest";
import { ErrorBoundary } from "./ErrorBoundary";

describe("ErrorBoundary", () => {
  it("clears a caught error when its reset key changes, and keeps it while the key stands", () => {
    const first = { id: "turn-a" };
    const failed = { message: ErrorBoundary.getDerivedStateFromError(new Error("bad part")).message ?? null, resetKey: first as unknown };
    expect(failed.message).toBe("bad part");
    const props = (resetKey: unknown) => ({ children: null, fallback: () => null, resetKey });
    expect(ErrorBoundary.getDerivedStateFromProps(props(first), failed)).toBeNull();
    expect(ErrorBoundary.getDerivedStateFromProps(props({ id: "turn-a" }), failed)).toEqual({ message: null, resetKey: { id: "turn-a" } });
    expect(ErrorBoundary.getDerivedStateFromProps(props(undefined), { message: "x", resetKey: undefined })).toBeNull();
  });
});

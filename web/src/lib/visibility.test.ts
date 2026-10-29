import { describe, expect, it } from "vitest";

// a hidden document with no other of ours: the page is visible only inside the Mac app
describe("pageVisible", () => {
  it("counts a hidden page as visible inside the Mac app, hidden otherwise", async () => {
    const g = globalThis as unknown as { document?: unknown; window?: unknown };
    g.document = { visibilityState: "hidden", addEventListener() {}, removeEventListener() {} };
    g.window = {};
    const { pageVisible } = await import("./visibility");
    expect(pageVisible()).toBe(false);
    (g.window as { remudaShell?: { native: boolean } }).remudaShell = { native: true };
    expect(pageVisible()).toBe(true);
    delete g.document;
    delete g.window;
  });
});

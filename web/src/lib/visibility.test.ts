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

// what only the page's own document draws stops reading while it is hidden: inside the Mac app with its window shut,
// or in a tab behind the pop-out
describe("documentVisible", () => {
  it("is false for a hidden document, though the Mac app or a pop-out keeps the page counting as visible", async () => {
    const g = globalThis as unknown as { document?: unknown; window?: unknown };
    g.document = { visibilityState: "hidden", addEventListener() {}, removeEventListener() {} };
    g.window = { remudaShell: { native: true } };
    const { alsoVisible, documentVisible, pageVisible } = await import("./visibility");
    const popOut = { visibilityState: "visible", addEventListener() {}, removeEventListener() {} } as unknown as Document;
    const gone = alsoVisible(popOut);
    expect(pageVisible()).toBe(true);
    expect(documentVisible()).toBe(false);
    (g.document as { visibilityState: string }).visibilityState = "visible";
    expect(documentVisible()).toBe(true);
    gone();
    delete g.document;
    delete g.window;
  });
});

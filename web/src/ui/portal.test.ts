import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { PortalHost, usePortalHost } from "./portal";

// Where tells which body a menu or a tooltip of this piece would open in
function Where({ popOut }: { popOut: unknown }) {
  const host = usePortalHost();
  return createElement("i", null, host === undefined ? "the page's body" : host === popOut ? "the pop-out's body" : "elsewhere");
}

describe("usePortalHost", () => {
  it("is the pop-out's body for a piece drawn there, and leaves the page's own to Radix elsewhere", () => {
    const popOutBody = { tagName: "BODY" } as unknown as HTMLElement;
    expect(renderToStaticMarkup(createElement(Where, { popOut: popOutBody }))).toBe("<i>the page&#x27;s body</i>");
    expect(renderToStaticMarkup(createElement(PortalHost.Provider, { value: popOutBody }, createElement(Where, { popOut: popOutBody })))).toBe("<i>the pop-out&#x27;s body</i>");
  });
});

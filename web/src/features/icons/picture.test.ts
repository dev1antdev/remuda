import { describe, expect, it } from "vitest";
import { fitInside, pictureKind, siteOf } from "./picture";

describe("pictureKind", () => {
  it("keeps a small SVG, draws a raster, refuses the rest", () => {
    expect(pictureKind("image/svg+xml", 2000)).toBe("svg");
    expect(pictureKind("image/svg+xml", 200_000)).toEqual({ refused: "An SVG icon can be at most 64 KB" });
    expect(pictureKind("image/png", 5_000_000)).toBe("raster");
    expect(pictureKind("image/x-icon", 1000)).toBe("raster");
    expect(pictureKind("application/pdf", 1000)).toEqual({ refused: "Pick a PNG, JPEG, GIF, WebP, ICO or SVG picture" });
  });
});

describe("fitInside", () => {
  it("centres the whole picture in the square, its proportions kept", () => {
    expect(fitInside(256, 256, 128)).toEqual({ x: 0, y: 0, w: 128, h: 128 });
    expect(fitInside(400, 200, 128)).toEqual({ x: 0, y: 32, w: 128, h: 64 });
    expect(fitInside(50, 100, 128)).toEqual({ x: 32, y: 0, w: 64, h: 128 });
    expect(fitInside(0, 10, 128)).toEqual({ x: 0, y: 0, w: 128, h: 128 });
  });
});

describe("siteOf", () => {
  it("adds https when no scheme was typed", () => {
    expect(siteOf(" gog.com ")).toBe("https://gog.com");
    expect(siteOf("http://intranet.local/app")).toBe("http://intranet.local/app");
    expect(siteOf("")).toBe("");
  });
});

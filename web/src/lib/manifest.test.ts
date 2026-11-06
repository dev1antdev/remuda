import { describe, expect, it } from "vitest";
import manifest from "../../public/manifest.webmanifest?raw";
import page from "../../index.html?raw";
import { pageColor } from "./theme";

describe("the installed app's colours", () => {
  it("are the page's own before its first paint: Frappé's base, as index.html's theme-color and a bare root wear", () => {
    const { background_color, theme_color } = JSON.parse(manifest) as { background_color: string; theme_color: string };
    expect(background_color).toBe(pageColor("frappe"));
    expect(theme_color).toBe(pageColor("frappe"));
    expect(page).toContain(`<meta name="theme-color" content="${pageColor("frappe")}" />`);
  });
});

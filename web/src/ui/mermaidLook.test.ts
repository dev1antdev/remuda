import { describe, expect, it } from "vitest";
import { FLAVORS } from "@/lib/theme";
import { flavorOf, isMarkdownFile, isMermaid, isMermaidFile, lookOf, mermaidConfig, opensDrawn, previewOf, unsafeAttribute } from "./mermaidLook";

describe("which fences and files are diagrams", () => {
  it("reads a fence's first word, any case", () => {
    expect(isMermaid("mermaid")).toBe(true);
    expect(isMermaid(" Mermaid title=flow")).toBe(true);
    expect(isMermaid("mmd")).toBe(true);
    expect(isMermaid("mermaidjs")).toBe(false);
    expect(isMermaid("js")).toBe(false);
    expect(isMermaid("")).toBe(false);
  });

  it("knows a diagram file and a Markdown one, and how each is shown drawn", () => {
    expect(isMermaidFile("docs/flow.mmd")).toBe(true);
    expect(isMermaidFile("Flow.MERMAID")).toBe(true);
    expect(isMarkdownFile("README.md")).toBe(true);
    expect(isMarkdownFile("notes.markdown")).toBe(true);
    expect(previewOf("a.mmd")).toBe("mermaid");
    expect(previewOf("a.md")).toBe("markdown");
    expect(previewOf("a.ts")).toBeNull();
  });

  it("opens drawn only when no line and no changes were asked for", () => {
    expect(opensDrawn("README.md")).toBe(true);
    expect(opensDrawn("README.md", 12)).toBe(false);
    expect(opensDrawn("README.md", undefined, "changes")).toBe(false);
    expect(opensDrawn("main.go")).toBe(false);
  });
});

describe("the look a diagram wears", () => {
  it("reads the root's flavour and accent, Frappé and peach otherwise", () => {
    expect(flavorOf("mocha")).toBe("mocha");
    expect(flavorOf("tokyo-night-day")).toBe("tokyo-night-day");
    expect(flavorOf("sepia")).toBe("frappe");
    expect(flavorOf(undefined)).toBe("frappe");
    expect(lookOf("latte|blue")).toEqual({ flavor: "latte", accent: "blue" });
    expect(lookOf("|")).toEqual({ flavor: "frappe", accent: "peach" });
  });

  it("draws strict, with SVG labels, in the flavour's colours and the accent on the edges", () => {
    const config = mermaidConfig("latte", "blue", "Inter");
    expect(config.securityLevel).toBe("strict");
    expect(config.startOnLoad).toBe(false);
    expect(config.htmlLabels).toBe(false);
    expect(config.flowchart.htmlLabels).toBe(false);
    expect(config.themeVariables.darkMode).toBe(false);
    expect(config.themeVariables.background).toBe(FLAVORS.latte.base);
    expect(config.themeVariables.primaryTextColor).toBe(FLAVORS.latte.text);
    expect(config.themeVariables.nodeBorder).toBe(FLAVORS.latte.blue);
    expect(mermaidConfig("mocha", "peach", "Inter").themeVariables.darkMode).toBe(true);
    expect(mermaidConfig("github-light", "peach", "Inter").themeVariables.darkMode).toBe(false);
    expect(mermaidConfig("nord", "teal", "Inter").themeVariables.nodeBorder).toBe(FLAVORS.nord.teal);
    expect(config.themeVariables.pie1).toBe(FLAVORS.latte.pink);
  });
});

describe("unsafeAttribute", () => {
  it("is an event handler, or a value naming a script URL however it is spelled: the URL parser drops a tab or a newline in the scheme", () => {
    expect(unsafeAttribute("onclick", "x()")).toBe(true);
    expect(unsafeAttribute("ONLOAD", "x()")).toBe(true);
    expect(unsafeAttribute("href", "javascript:alert(1)")).toBe(true);
    expect(unsafeAttribute("href", "java\tscript:alert(1)")).toBe(true);
    expect(unsafeAttribute("xlink:href", " \nJaVa\nScRiPt:alert(1)")).toBe(true);
    expect(unsafeAttribute("href", "vbscript:msgbox")).toBe(true);
  });
  it("is what an animation would set a link to, which no href check sees", () => {
    expect(unsafeAttribute("to", "javascript:alert(1)")).toBe(true);
    expect(unsafeAttribute("values", "#a;javascript:alert(1)")).toBe(true);
  });
  it("leaves a diagram's own attributes and its links alone", () => {
    expect(unsafeAttribute("href", "https://example.com/a?b=c")).toBe(false);
    expect(unsafeAttribute("href", "#remuda-mermaid-1")).toBe(false);
    expect(unsafeAttribute("class", "node default flowchart-label")).toBe(false);
    expect(unsafeAttribute("d", "M0,0 L10,10")).toBe(false);
    expect(unsafeAttribute("style", "max-width: 640px;")).toBe(false);
  });
});

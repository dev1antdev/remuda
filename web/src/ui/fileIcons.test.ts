import { describe, expect, it } from "vitest";
import { FILE_ICON_TABLE } from "./fileIconTable";
import { fileIconName } from "./fileIcons";

const svgs = import.meta.glob("../../public/file-icons/*.svg", { query: "?raw", import: "default", eager: true }) as Record<string, string>;
const shipped = new Map(Object.entries(svgs).map(([path, text]) => [path.split("/").pop()!.replace(/\.svg$/, ""), text]));

// runsSomething says an SVG holds what could run or reach past itself: a script or a foreignObject under any namespace
// prefix, an event handler, a javascript: link, an href or a url() to anything but its own #id, an @import, a
// stylesheet instruction or a DOCTYPE with its entities
const RUNS = [/<([\w.-]+:)?(script|foreignObject)\b/i, /\son[a-z]+\s*=/i, /javascript:/i, /href\s*=\s*(?!["']\s*#)/i, /url\(\s*(?!["']?\s*#)/i, /@import/i, /<\?xml-stylesheet/i, /<!(DOCTYPE|ENTITY)/i];
const runsSomething = (text: string): boolean => RUNS.some((rule) => rule.test(text));

describe("a file's icon", () => {
  it("is picked as VS Code picks it: a path's end beats a name, a name an extension, the longest extension wins", () => {
    expect(fileIconName("app.ts")).toBe("typescript");
    expect(fileIconName("app.spec.ts")).toBe("test-ts");
    expect(fileIconName("types.d.ts")).toBe("typescript-def");
    expect(fileIconName("Button.tsx")).toBe("react_ts");
    expect(fileIconName("go.mod")).toBe("go-mod");
    expect(fileIconName("main.go")).toBe("go");
    expect(fileIconName("Makefile")).toBe("makefile");
    expect(fileIconName("package.json")).toBe("nodejs");
    expect(fileIconName("tsconfig.json")).toBe("tsconfig");
    expect(fileIconName("vite.config.ts")).toBe("vite");
    expect(fileIconName("README.md")).toBe("readme");
    expect(fileIconName("notes.md")).toBe("markdown");
    expect(fileIconName("CLAUDE.md")).toBe("claude");
    expect(fileIconName(".gitignore")).toBe("git");
    expect(fileIconName(".env")).toBe("tune");
    expect(fileIconName("Dockerfile")).toBe("docker");
    expect(fileIconName("LICENSE")).toBe("license");
    expect(fileIconName("web/.config/eslintrc.json")).toBe("eslint");
  });

  it("knows a Windows app's files", () => {
    expect(fileIconName("Form1.cs")).toBe("csharp");
    expect(fileIconName("App.xaml.cs")).toBe("csharp");
    expect(fileIconName("MainWindow.xaml")).toBe("xaml");
    expect(fileIconName("App.csproj")).toBe("visualstudio");
    expect(fileIconName("App.sln")).toBe("visualstudio");
    expect(fileIconName("Resources.resx")).toBe("xml");
    expect(fileIconName("build.ps1")).toBe("powershell");
    expect(fileIconName("C:\\Users\\me\\app\\Form1.cs")).toBe("csharp");
  });

  it("is the plain file for a name nobody has a rule for", () => {
    expect(fileIconName("notes.xyz")).toBe("file");
    expect(fileIconName("")).toBe("file");
  });

  it("is the light theme's own where it has one", () => {
    expect(fileIconName("config.toml")).toBe("toml");
    expect(fileIconName("config.toml", { light: true })).toBe("toml_light");
    expect(fileIconName("pnpm-lock.yaml", { light: true })).toBe("pnpm_light");
    expect(fileIconName("main.go", { light: true })).toBe("go");
  });
});

describe("a folder's icon", () => {
  it("goes by its name in every spelling Material reads, open or closed", () => {
    expect(fileIconName("src", { dir: true })).toBe("folder-src");
    expect(fileIconName("src", { dir: true, open: true })).toBe("folder-src-open");
    expect(fileIconName("_src", { dir: true })).toBe("folder-src");
    expect(fileIconName("__tests__", { dir: true })).toBe("folder-test");
    expect(fileIconName(".claude", { dir: true })).toBe("folder-claude");
    expect(fileIconName("node_modules", { dir: true })).toBe("folder-node");
    expect(fileIconName("assets", { dir: true })).toBe("folder-resource");
    expect(fileIconName("whatever", { dir: true })).toBe("folder");
    expect(fileIconName("whatever", { dir: true, open: true })).toBe("folder-open");
  });

  it("takes a rule for its path's end over its own name", () => {
    expect(fileIconName(".github/workflows", { dir: true })).toBe("folder-gh-workflows");
    expect(fileIconName("app/.github/workflows", { dir: true, open: true })).toBe("folder-gh-workflows-open");
    expect(fileIconName("app/.github", { dir: true })).toBe("folder-github");
  });
});

describe("the icons shipped", () => {
  const named = new Set<string>([FILE_ICON_TABLE.file, FILE_ICON_TABLE.folder]);
  for (const group of [FILE_ICON_TABLE.names, FILE_ICON_TABLE.extensions, FILE_ICON_TABLE.folders, FILE_ICON_TABLE.light.names, FILE_ICON_TABLE.light.extensions, FILE_ICON_TABLE.light.folders])
    for (const icon of Object.keys(group)) named.add(icon);
  for (const [closed, open] of Object.entries(FILE_ICON_TABLE.open)) named.add(closed).add(open);

  it("are every icon the rules name, and nothing else", () => {
    expect([...named].filter((icon) => !shipped.has(icon))).toEqual([]);
    expect([...shipped.keys()].filter((icon) => !named.has(icon))).toEqual([]);
  });

  it("run nothing: no script, no handler, no javascript: link, no outside reference", () => {
    const bad = [...shipped].filter(([, text]) => runsSomething(text)).map(([icon]) => icon);
    expect(bad).toEqual([]);
  });

  it("are read by a check that catches what would run or reach out", () => {
    const svg = (inner: string) => `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">${inner}</svg>`;
    for (const bad of [
      svg(`<script>alert(1)</script>`),
      svg(`<svg:script xmlns:svg="http://www.w3.org/2000/svg">alert(1)</svg:script>`),
      svg(`<path d="M0 0" onload="alert(1)"/>`),
      svg(`<a href="javascript:alert(1)"><path d="M0 0"/></a>`),
      svg(`<use xlink:href="https://example.invalid/i.svg#a"/>`),
      svg(`<image href='data:image/svg+xml;base64,AAAA'/>`),
      svg(`<path d="M0 0" fill="url(https://example.invalid/p.svg#g)"/>`),
      svg(`<path d="M0 0" style="fill: url( 'https://example.invalid/p.svg#g' )"/>`),
      svg(`<style>@import "https://example.invalid/x.css";</style>`),
      svg(`<foreignObject><div/></foreignObject>`),
      `<?xml-stylesheet href="https://example.invalid/x.css"?>${svg("")}`,
      `<!DOCTYPE svg [<!ENTITY x SYSTEM "file:///etc/passwd">]>${svg("&x;")}`,
    ])
      expect(runsSomething(bad), bad).toBe(true);
    for (const fine of [svg(`<path fill="url(#g)" d="M0 0"/><use href="#a"/>`), svg(`<path d="M1 1" fill="#4caf50" class="ion"/>`)]) expect(runsSomething(fine), fine).toBe(false);
  });

  it("weigh under 600 KB in all", () => {
    const bytes = [...shipped.values()].reduce((sum, text) => sum + new TextEncoder().encode(text).length, 0);
    expect(bytes).toBeLessThan(600 * 1024);
  });
});

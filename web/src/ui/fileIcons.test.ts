import { describe, expect, it } from "vitest";
import { FILE_ICON_TABLE } from "./fileIconTable";
import { fileIconName } from "./fileIcons";

const svgs = import.meta.glob("../../public/file-icons/*.svg", { query: "?raw", import: "default", eager: true }) as Record<string, string>;
const shipped = new Map(Object.entries(svgs).map(([path, text]) => [path.split("/").pop()!.replace(/\.svg$/, ""), text]));

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
    const bad = [...shipped].filter(([, text]) => /<script|\son[a-z]+\s*=|javascript:|<foreignObject|href\s*=\s*["'](?!#)/i.test(text)).map(([icon]) => icon);
    expect(bad).toEqual([]);
  });

  it("weigh under 600 KB in all", () => {
    const bytes = [...shipped.values()].reduce((sum, text) => sum + new TextEncoder().encode(text).length, 0);
    expect(bytes).toBeLessThan(600 * 1024);
  });
});

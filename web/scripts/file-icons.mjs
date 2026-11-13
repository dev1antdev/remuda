#!/usr/bin/env node
// file-icons.mjs: the file and folder icons of Material Icon Theme (MIT) the page draws — the subset below, for what
// the person works in — copied from a folder laid out as Better Icons keeps the theme (its icons.json table and an
// icons/ folder of SVGs), and the theme's rules for them written as ui/fileIconTable.ts. It runs no code of the
// theme's, reads only that folder and writes only web/public/file-icons/ and the table.
//
//   node scripts/file-icons.mjs <folder with icons.json and icons/>

import { copyFileSync, existsSync, mkdirSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const FILES = [
  "go", "go-mod", "typescript", "typescript-def", "react_ts", "react", "javascript", "test-ts", "test-js", "test-jsx",
  "json", "yaml", "toml", "markdown", "mdx", "python", "python-misc", "rust", "swift", "kotlin", "java", "javaclass",
  "jar", "c", "h", "cpp", "hpp", "csharp", "visualstudio", "xaml", "xml", "powershell", "ruby", "gemfile", "php",
  "console", "database", "html", "css", "sass", "less", "vue", "svelte", "docker", "makefile", "git", "tune", "nodejs",
  "npm", "yarn", "pnpm", "bun", "lock", "tsconfig", "jsconfig", "vite", "vitest", "eslint", "prettier", "editorconfig",
  "license", "readme", "changelog", "contributing", "claude", "agent", "gemini-ai", "todo", "image", "svg", "favicon",
  "font", "zip", "pdf", "document", "log", "table", "video", "audio", "gradle", "settings", "key", "certificate",
  "graphql", "proto", "webassembly", "exe", "dll", "lua", "dart", "diff", "tailwindcss", "postcss", "jest", "next",
  "nuget", "razor", "jupyter", "codeowners",
];
const FOLDERS = [
  "folder-src", "folder-lib", "folder-docs", "folder-test", "folder-github", "folder-gh-workflows", "folder-vscode",
  "folder-claude", "folder-public", "folder-resource", "folder-components", "folder-scripts", "folder-config",
  "folder-dist", "folder-node", "folder-command", "folder-api", "folder-app", "folder-packages", "folder-git",
  "folder-images", "folder-macos", "folder-windows", "folder-utils", "folder-server", "folder-tools", "folder-temp",
  "folder-coverage", "folder-typescript", "folder-database",
];

const pack = process.argv[2];
if (!pack || !existsSync(join(pack, "icons.json"))) {
  console.error("usage: node scripts/file-icons.mjs <folder with icons.json and icons/>");
  process.exit(2);
}
const theme = JSON.parse(readFileSync(join(pack, "icons.json"), "utf8"));
const web = new URL("..", import.meta.url).pathname;
const out = join(web, "public/file-icons");

const files = new Set([theme.file, ...FILES]);
const folders = new Set([theme.folder, ...FOLDERS]);
const kept = (table, wanted) => Object.fromEntries(Object.entries(table).filter(([, icon]) => wanted.has(icon)).sort(([a], [b]) => (a < b ? -1 : 1)));
const names = kept(theme.fileNames, files);
const extensions = kept(theme.fileExtensions, files);
const folderNames = kept(theme.folderNames, folders);
const open = Object.fromEntries([...folders].map((icon) => [icon, theme.open[icon]]).filter(([, icon]) => icon));
// a light theme's icon for a name the dark table gives one of the icons kept
const lightOf = (table, dark) => Object.fromEntries(Object.entries(table ?? {}).filter(([key]) => key in dark).sort(([a], [b]) => (a < b ? -1 : 1)));
const light = { names: lightOf(theme.light?.fileNames, names), extensions: lightOf(theme.light?.fileExtensions, extensions), folders: lightOf(theme.light?.folderNames, folderNames) };

const icons = new Set([...files, ...folders, ...Object.values(open), ...Object.values(light.names), ...Object.values(light.extensions), ...Object.values(light.folders)]);
rmSync(out, { recursive: true, force: true });
mkdirSync(out, { recursive: true });
let bytes = 0;
for (const icon of [...icons].sort()) {
  const from = join(pack, "icons", `${icon}.svg`);
  if (!existsSync(from)) throw new Error(`the theme has no ${icon}.svg`);
  copyFileSync(from, join(out, `${icon}.svg`));
  bytes += statSync(from).size;
}
const licence = readdirSync(pack).find((name) => /^LICENSE/i.test(name));
if (licence) copyFileSync(join(pack, licence), join(out, "LICENSE.txt"));

// every folder rule comes in five spellings — src, .src, _src, -src, __src__ — so a folder is kept by its plain one
const variants = (base) => [base, `.${base}`, `_${base}`, `-${base}`, `__${base}__`];
const plainFolders = Object.fromEntries(Object.entries(folderNames).filter(([key]) => !/^[._-]/.test(key)));
for (const [base, icon] of Object.entries(plainFolders))
  if (variants(base).some((key) => folderNames[key] !== icon)) throw new Error(`the folder rule ${base} lacks a spelling: keep the table whole`);
if (Object.keys(plainFolders).length * 5 !== Object.keys(folderNames).length) throw new Error("a folder rule has no plain spelling");
// byIcon turns key → icon into icon → its keys, the table at a fraction of its size
const byIcon = (table) => {
  const grouped = {};
  for (const [key, icon] of Object.entries(table)) (grouped[icon] ??= []).push(key);
  return Object.fromEntries(Object.entries(grouped).sort(([a], [b]) => (a < b ? -1 : 1)));
};
const table = {
  source: theme.source,
  file: theme.file,
  folder: theme.folder,
  names: byIcon(names),
  extensions: byIcon(extensions),
  folders: byIcon(plainFolders),
  open,
  light: { names: byIcon(light.names), extensions: byIcon(light.extensions), folders: byIcon(light.folders) },
};
writeFileSync(
  join(web, "src/ui/fileIconTable.ts"),
  `// Generated by scripts/file-icons.mjs from Material Icon Theme ${theme.source?.split(" ").pop() ?? ""} (MIT): the rules for the icons kept
// in public/file-icons/, each icon with the keys it is for. Do not edit by hand.

export type FileIconTable = {
  source: string;
  file: string;
  folder: string;
  names: Record<string, string[]>; // icon → the file names it is for, in lower case; a name with a slash is a path's end
  extensions: Record<string, string[]>; // icon → the extensions, without their first dot
  folders: Record<string, string[]>; // icon → the folder names, each read also after a . _ or - and inside __ __
  open: Record<string, string>; // a folder's icon → its open one
  light: { names: Record<string, string[]>; extensions: Record<string, string[]>; folders: Record<string, string[]> }; // a light theme's icon → the keys it takes
};

export const FILE_ICON_TABLE: FileIconTable = ${JSON.stringify(table)};
`,
);
console.log(`${icons.size} icons, ${Math.round(bytes / 1024)} KB, ${Object.keys(names).length} names, ${Object.keys(extensions).length} extensions, ${Object.keys(folderNames).length} folders`);

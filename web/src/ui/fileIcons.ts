// Which of Material Icon Theme's icons a file or a folder wears, picked the way VS Code picks them: a rule for the end of a
// path (.github/workflows) beats a name, a name beats an extension, and the longest extension wins, so app.spec.ts is a
// test and app.ts TypeScript; a light theme takes the theme's light icon where it has one; anything else is the plain
// file or folder. The icons are static files under /file-icons/, drawn as images.

import { FILE_ICON_TABLE, type FileIconTable } from "./fileIconTable";

export const FILE_ICON_BASE = "/file-icons/";
const PATH_PARTS = 2; // the longest path rule names this many parts

type Lookup = { names: Map<string, string>; extensions: Map<string, string>; folders: Map<string, string> };
type Rules = { dark: Lookup; light: Lookup; file: string; folder: string; open: Record<string, string> };

const flip = (byIcon: Record<string, string[]>): Map<string, string> => {
  const map = new Map<string, string>();
  for (const [icon, keys] of Object.entries(byIcon)) for (const key of keys) map.set(key, icon);
  return map;
};

// rulesOf reads a table once into the maps a lookup walks
export function rulesOf(table: FileIconTable): Rules {
  return {
    dark: { names: flip(table.names), extensions: flip(table.extensions), folders: flip(table.folders) },
    light: { names: flip(table.light.names), extensions: flip(table.light.extensions), folders: flip(table.light.folders) },
    file: table.file,
    folder: table.folder,
    open: table.open,
  };
}

let rules: Rules | null = null;
const shipped = (): Rules => (rules ??= rulesOf(FILE_ICON_TABLE));

// plainFolder is a folder's name without the spelling Material's rules read it in too: .src, _src, -src, __src__
const plainFolder = (name: string): string => {
  const wrapped = /^__(.+)__$/.exec(name);
  if (wrapped) return wrapped[1]!;
  return /^[._-]./.test(name) ? name.slice(1) : name;
};

export type IconAsk = { dir?: boolean; open?: boolean; light?: boolean };

// fileIconName is the icon for a path — a name, or a path ending in it with forward slashes, so a rule for the path's end
// matches whichever folder it sits in
export function fileIconName(path: string, { dir = false, open = false, light = false }: IconAsk = {}, table: Rules = shipped()): string {
  const parts = path.toLowerCase().split(/[\\/]/).filter(Boolean);
  const name = parts.at(-1) ?? "";
  const find = (kind: keyof Lookup, key: string): string | undefined => (light ? table.light[kind].get(key) : undefined) ?? table.dark[kind].get(key);
  const named = (key: string): string | undefined => (dir ? find("folders", key) ?? find("folders", plainFolder(key)) : find("names", key));
  let icon: string | undefined;
  for (let from = Math.max(0, parts.length - PATH_PARTS); from < parts.length - 1 && !icon; from++) icon = named(parts.slice(from).join("/"));
  icon ??= named(name);
  for (let dot = name.indexOf("."); !dir && !icon && dot !== -1; dot = name.indexOf(".", dot + 1)) icon = find("extensions", name.slice(dot + 1));
  icon ??= dir ? table.folder : table.file;
  return dir && open ? (table.open[icon] ?? icon) : icon;
}

// fileIconUrl is where the page reads an icon
export const fileIconUrl = (icon: string): string => `${FILE_ICON_BASE}${icon}.svg`;

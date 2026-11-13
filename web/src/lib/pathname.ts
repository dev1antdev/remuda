// A path's parts the way its machine writes them: a POSIX path by its slashes, a Windows one — herdr on Windows reports
// C:\Users\me\app — by either separator, under a drive letter or a share. Pure.

// windowsPath says a path is written the Windows way: a drive letter (C:\, C:/, C:), a \\share, or backslashes and no slash
export const windowsPath = (p: string): boolean => /^[A-Za-z]:([\\/]|$)/.test(p) || p.startsWith("\\\\") || (p.includes("\\") && !p.includes("/"));

const SEPARATORS = /[\\/]/;

// separatorOf is the separator a path's own machine joins with
export const separatorOf = (p: string): string => (windowsPath(p) && !p.includes("/") ? "\\" : "/");

const lastSeparator = (p: string) => (windowsPath(p) ? Math.max(p.lastIndexOf("/"), p.lastIndexOf("\\")) : p.lastIndexOf("/"));

// pathParts are the path's segments, the separators and the root left out
export const pathParts = (p: string): string[] => p.split(windowsPath(p) ? SEPARATORS : "/").filter(Boolean);

// basename is the last segment, a trailing separator ignored
export const basename = (p: string): string => pathParts(p).pop() ?? p;

// dirname is everything up to the last separator, the separator kept; "" for a bare name
export const dirname = (p: string): string => {
  const i = lastSeparator(p);
  return i < 0 ? "" : p.slice(0, i + 1);
};

// joinPath puts a name under a folder with the folder's own separator
export function joinPath(dir: string, name: string): string {
  if (!dir) return name;
  return /[\\/]$/.test(dir) ? dir + name : dir + separatorOf(dir) + name;
}

// underFolder is the path relative to the folder when it lies under it, else null; a Windows path compares case-blind
export function underFolder(path: string, dir: string): string | null {
  const base = dir.replace(/[\\/]+$/, "");
  if (!base) return null;
  const windows = windowsPath(base);
  const fold = (s: string) => (windows ? s.toLowerCase().replace(/\\/g, "/") : s);
  const head = fold(path.slice(0, base.length + 1));
  return head === fold(base) + "/" ? path.slice(base.length + 1) : null;
}

export type Crumb = { label: string; path: string };

// pathCrumbs is the path as a row of crumbs from its root, each with the path it stands for: "/" then the folders, or a
// drive (C:\) or a share (\\host\share) then the folders
export function pathCrumbs(path: string): Crumb[] {
  if (!path) return [];
  if (!windowsPath(path)) {
    const names = path.split("/").filter(Boolean);
    return [{ label: "/", path: "/" }, ...names.map((name, i) => ({ label: name, path: "/" + names.slice(0, i + 1).join("/") }))];
  }
  const sep = separatorOf(path);
  const share = /^\\\\[^\\/]+[\\/][^\\/]+/.exec(path);
  const drive = /^[A-Za-z]:/.exec(path);
  const root = share ? share[0] + sep : drive ? drive[0] + sep : "";
  const names = path.slice(root.length).split(SEPARATORS).filter(Boolean);
  const crumbs = root ? [{ label: root, path: root }] : [];
  return [...crumbs, ...names.map((name, i) => ({ label: name, path: root + names.slice(0, i + 1).join(sep) }))];
}

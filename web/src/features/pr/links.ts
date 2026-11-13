// A GitHub pull request's address read into its repository and number — the panel opens on one from a chip's menu or a
// link in an agent's answer — and the panel's way out to GitHub.

const PULL = /^https:\/\/github\.com\/([A-Za-z0-9_.-]+)\/([A-Za-z0-9_.-]+)\/pull\/([1-9][0-9]*)(?:[/?#].*)?$/;

// prFromURL is the pull request an address names, null for any other
export function prFromURL(url: string): { repo: string; number: number } | null {
  const m = PULL.exec(url.trim());
  if (!m || [m[1], m[2]].some((s) => s === "." || s === "..")) return null;
  return { repo: `${m[1]}/${m[2]}`, number: Number(m[3]) };
}

// isPullURL says whether a link opens the panel rather than the browser
export const isPullURL = (url: string): boolean => prFromURL(url) !== null;

// filesURL is the pull request's Files page on GitHub
export const filesURL = (url: string) => `${url.replace(/\/+$/, "")}/files`;

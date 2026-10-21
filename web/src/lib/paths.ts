// Finds file paths in a line of text the way an agent prints them (absolute, ~/…, ./…, relative with
// a slash, a bare name with a code-ish extension, a dotfile such as .env, optionally :line[:col]) and
// URLs, which are never paths. A link needs a name a file would have — a dotfile or a name with an
// extension — so a folder (internal/server) or a slashed phrase (Approve/Reject) is none. Shared by the
// terminal link provider and the chat linkifier.

export type PathMatch = {
  start: number;
  end: number; // exclusive
  text: string;
  path: string;
  line?: number;
};

const EXT =
  "(?:go|ts|tsx|js|jsx|mjs|cjs|py|rb|rs|java|kt|swift|c|h|cpp|hpp|cs|php|sh|zsh|bash|sql|md|txt|json|ya?ml|toml|ini|cfg|conf|env|css|scss|html?|xml|svg|png|jpe?g|gif|webp|pdf|mp4|webm|mov|lock|log|csv|proto|graphql|tf|vue|svelte|dart|ex|exs|hs|lua|pl|r|scala|zig|mod|sum)";
// the dotfiles agents name on their own; a bare ".md" in prose is an extension, not a file
const DOTFILE = String.raw`(?:env(?:\.[\w-]+)?|envrc|gitignore|gitattributes|gitmodules|dockerignore|npmrc|nvmrc|npmignore|editorconfig|prettierrc|eslintrc|babelrc|htaccess|tool-versions|python-version|ruby-version|node-version|gitconfig|zshrc|zprofile|bashrc|bash_profile|profile|vimrc)`;

const WORD = String.raw`\p{L}\p{N}_`;
const SEG = String.raw`[${WORD}.@%+=-]+`;
const NAME = String.raw`[${WORD}@%+-][${WORD}.@%+-]*`;
const PATH_RE = new RegExp(
  String.raw`(?:~|\.{1,2})?/(?:${SEG}/)*${SEG}` + // absolute, ~/, ./, ../
    String.raw`|(?:${SEG}/)+${SEG}` + // relative with a slash
    String.raw`|\.?${NAME}\.${EXT}\b` + // bare filename with a known extension, its leading dot kept (.mcp.json)
    String.raw`|(?<![${WORD}.@%+/-])\.${DOTFILE}\b`, // a dotfile
  "gu",
);
// a name a file would have: a dotfile, or an extension with a letter in it (v1.2.3 has none)
const FILE_NAME = /^\.[^/]+$|\.[A-Za-z0-9]*[A-Za-z][A-Za-z0-9]*$/;
const LINE_SUFFIX = /:(\d+)(?::\d+)?$/;
const TRAILING = /[.,;:)\]}'"`>]+$/;

export type UrlMatch = {
  start: number;
  end: number; // exclusive
  text: string;
  url: string; // with a scheme
};

// http(s) URLs, plus the scheme-less dev-server kind (localhost:5173/app,
// herdr.test/…) when a port or a path follows the host
const URL_TAIL = String.raw`[^\s<>"'\x60]*`;
const URL_RE = new RegExp(
  String.raw`https?://${URL_TAIL}` +
    String.raw`|(?<![\w.@/-])(?:localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1\]|[\w-]+\.(?:local|test|localhost))(?::\d+(?:/${URL_TAIL})?|/${URL_TAIL})`,
  "g",
);

export function findUrls(text: string): UrlMatch[] {
  const out: UrlMatch[] = [];
  for (const m of text.matchAll(URL_RE)) {
    const raw = m[0].replace(TRAILING, "");
    const at = m.index ?? 0;
    out.push({ start: at, end: at + raw.length, text: raw, url: /^https?:/.test(raw) ? raw : `http://${raw}` });
  }
  return out;
}

export function findPaths(text: string): PathMatch[] {
  const out: PathMatch[] = [];
  const urls = findUrls(text);
  for (const m of text.matchAll(PATH_RE)) {
    let raw = m[0];
    const at = m.index ?? 0;
    // not the path part of a URL
    const before = text.slice(Math.max(0, at - 3), at);
    if (before.endsWith("//") || raw.includes("://")) continue;
    if (urls.some((u) => at < u.end && at + raw.length > u.start)) continue;
    // keep a :line suffix that follows the match, then strip punctuation
    const tail = text.slice(at + raw.length).match(/^(?::\d+){1,2}/);
    if (tail) raw += tail[0];
    raw = raw.replace(TRAILING, "");
    if (raw.length < 3) continue;
    let path = raw;
    let line: number | undefined;
    const ln = raw.match(LINE_SUFFIX);
    if (ln) {
      line = Number(ln[1]);
      path = raw.slice(0, ln.index);
    }
    if (path === "/" || path === "./" || path === "../") continue;
    // a bare "x.y" that is really a version or a domain is not a file
    if (!path.includes("/") && /^\d+(\.\d+)+$/.test(path)) continue;
    if (!FILE_NAME.test(path.slice(path.lastIndexOf("/") + 1))) continue;
    out.push({ start: at, end: at + raw.length, text: raw, path, line });
  }
  return out;
}

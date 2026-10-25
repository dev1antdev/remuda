// A small highlighter of our own: one scanner driven by a table per language, hand-written so a line costs its
// length (a regex tried from every position inside a long token once froze the page on a base64 row). Nothing
// here knows React: the chat's code fences and the Changes lens's diff rows draw the tokens as spans.

export type TokenKind = "comment" | "string" | "number" | "keyword" | "builtin" | "constant" | "type" | "function" | "operator" | "punct" | "tag" | "attr" | "key" | "plain";
export type Token = { kind: TokenKind; text: string };

// MAX_HIGHLIGHT is the text size past which everything comes back plain: a dump pasted into a fence is read, not coloured
export const MAX_HIGHLIGHT = 200_000;

type StringRule = { open: string; close: string; escape: boolean; multiline: boolean };

type Lang = {
  line: string[]; // line comment openers
  block: [string, string][]; // block comment pairs
  strings: StringRule[];
  stringPrefixes?: string[]; // letters that may stand before a quote (python's f r b, rust's r b)
  keywords: Set<string>;
  builtins: Set<string>;
  constants: Set<string>;
  types: Set<string>;
  upperTypes: boolean; // an identifier starting with a capital letter is a type, an ALL_CAPS one a constant
  calls: boolean; // an identifier before ( is a function
  caseless?: boolean; // keywords match in any case (sql, dockerfile)
  identExtra?: string; // characters an identifier may hold beyond letters, digits and _
  hashAfterSpace?: boolean; // # opens a comment only at a token's start (shell: $# is a variable)
  sigils?: boolean; // $name and ${name} are variables (shell, make, dockerfile)
  decorators?: boolean; // @name is an attribute (python) or an at-rule (css)
  macros?: boolean; // name! is a function, #[…] an attribute, r#"…"# a raw string (rust)
  rustChars?: boolean; // 'a' is a character, 'a alone a lifetime
  regex?: boolean; // /…/ where a value may start is a string (javascript)
  dataKeys?: boolean; // a string before : is a key (json)
  commands?: boolean; // the first word of a command is a function, -flags are attributes (shell)
  lineKeywords?: boolean; // the first word of a line is a keyword when the table has it (dockerfile)
  makeTargets?: boolean; // name: at a line's start is a target, NAME = a variable (make)
  iniKeys?: boolean; // [section] and key = at a line's start (ini, toml)
  css?: boolean; // selectors outside braces, properties inside
};

const words = (list: string) => new Set(list.split(" ").filter(Boolean));
const none = new Set<string>();
const SQ: StringRule = { open: "'", close: "'", escape: true, multiline: false };
const DQ: StringRule = { open: '"', close: '"', escape: true, multiline: false };
const BT: StringRule = { open: "`", close: "`", escape: true, multiline: true };
const RAW_BT: StringRule = { open: "`", close: "`", escape: false, multiline: true };
const TRIPLE_D: StringRule = { open: '"""', close: '"""', escape: true, multiline: true };
const TRIPLE_S: StringRule = { open: "'''", close: "'''", escape: true, multiline: true };
const SLASH = ["//"];
const C_BLOCK: [string, string][] = [["/*", "*/"]];

const LANGS: Record<string, Lang> = {
  go: {
    line: SLASH,
    block: C_BLOCK,
    strings: [DQ, SQ, RAW_BT],
    keywords: words("break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var"),
    builtins: words("append cap clear close complex copy delete imag len make max min new panic print println real recover"),
    constants: words("true false nil iota"),
    types: words("any bool byte complex64 complex128 error float32 float64 int int8 int16 int32 int64 rune string uint uint8 uint16 uint32 uint64 uintptr comparable"),
    upperTypes: true,
    calls: true,
  },
  js: {
    line: SLASH,
    block: C_BLOCK,
    strings: [DQ, SQ, BT],
    keywords: words("abstract as async await break case catch class const continue debugger declare default delete do else enum export extends finally for from function get if implements import in instanceof interface is keyof let namespace new of override package private protected public readonly return satisfies set static switch throw try type typeof var void while with yield"),
    builtins: words("console window document globalThis require module exports process Math JSON Promise Array Object Number String Boolean Symbol Date Map Set WeakMap WeakSet RegExp Error parseInt parseFloat isNaN fetch setTimeout clearTimeout setInterval clearInterval"),
    constants: words("true false null undefined NaN Infinity this super arguments"),
    types: words("any bigint boolean never number object string symbol unknown void Record Partial Required Readonly Pick Omit ReturnType Parameters Awaited"),
    upperTypes: true,
    calls: true,
    identExtra: "$",
    decorators: true,
    regex: true,
  },
  python: {
    line: ["#"],
    block: [],
    strings: [TRIPLE_D, TRIPLE_S, DQ, SQ],
    stringPrefixes: ["rb", "br", "fr", "rf", "f", "r", "b", "u"],
    keywords: words("and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield match case"),
    builtins: words("abs all any ascii bin breakpoint callable chr compile delattr dir divmod enumerate eval exec filter format getattr globals hasattr hash help hex id input isinstance issubclass iter len locals map max min next oct open ord pow print property range repr reversed round setattr sorted staticmethod classmethod sum super vars zip __import__ self cls"),
    constants: words("True False None NotImplemented Ellipsis"),
    types: words("int float complex str bytes bytearray list tuple dict set frozenset bool object type Exception BaseException ValueError TypeError KeyError IndexError RuntimeError"),
    upperTypes: true,
    calls: true,
    decorators: true,
  },
  shell: {
    line: ["#"],
    block: [],
    strings: [
      { open: '"', close: '"', escape: true, multiline: true },
      { open: "'", close: "'", escape: false, multiline: true },
    ],
    keywords: words("if then else elif fi for while until do done case esac in function select time coproc"),
    builtins: words("echo cd export local return exit set unset source alias test printf read shift trap eval exec true false pwd type declare readonly let"),
    constants: none,
    types: none,
    upperTypes: false,
    calls: false,
    hashAfterSpace: true,
    sigils: true,
    commands: true,
  },
  json: {
    line: [],
    block: [],
    strings: [DQ],
    keywords: none,
    builtins: none,
    constants: words("true false null"),
    types: none,
    upperTypes: false,
    calls: false,
    dataKeys: true,
  },
  toml: {
    line: ["#"],
    block: [],
    strings: [TRIPLE_D, TRIPLE_S, DQ, { open: "'", close: "'", escape: false, multiline: false }],
    keywords: none,
    builtins: none,
    constants: words("true false inf nan"),
    types: none,
    upperTypes: false,
    calls: false,
    iniKeys: true,
    identExtra: "-.",
  },
  ini: {
    line: ["#", ";"],
    block: [],
    strings: [DQ, SQ],
    keywords: none,
    builtins: none,
    constants: words("true false yes no on off"),
    types: none,
    upperTypes: false,
    calls: false,
    iniKeys: true,
    identExtra: "-.",
  },
  css: {
    line: [],
    block: C_BLOCK,
    strings: [DQ, SQ],
    keywords: words("important and not only or from to"),
    builtins: words("var calc min max clamp rgb rgba hsl hsla color-mix url attr env light-dark linear-gradient radial-gradient"),
    constants: words("inherit initial unset revert none auto"),
    types: none,
    upperTypes: false,
    calls: true,
    identExtra: "-",
    decorators: true,
    css: true,
  },
  rust: {
    line: SLASH,
    block: C_BLOCK,
    strings: [DQ],
    stringPrefixes: ["br", "r", "b"],
    keywords: words("as async await break const continue crate dyn else enum extern fn for if impl in let loop match mod move mut pub ref return static struct super trait type unsafe use where while"),
    builtins: words("println print eprintln eprint format vec panic assert assert_eq assert_ne todo unimplemented unreachable dbg matches write writeln"),
    constants: words("true false self Self None Some Ok Err"),
    types: words("bool char str u8 u16 u32 u64 u128 usize i8 i16 i32 i64 i128 isize f32 f64 String Vec Option Result Box Rc Arc HashMap HashSet BTreeMap"),
    upperTypes: true,
    calls: true,
    macros: true,
    rustChars: true,
  },
  sql: {
    line: ["--"],
    block: C_BLOCK,
    strings: [{ open: "'", close: "'", escape: false, multiline: true }, DQ, RAW_BT],
    keywords: words("select from where and or not in is null as insert into values update set delete create table index view drop alter add column primary key foreign references unique default constraint join inner left right full outer on group by order asc desc having limit offset union all distinct case when then else end exists between like with returning begin commit rollback transaction if"),
    builtins: words("count sum avg min max coalesce nullif cast now date lower upper length substr trim round abs"),
    constants: words("true false"),
    types: words("int integer bigint smallint text varchar char boolean bool real float double decimal numeric date time timestamp timestamptz json jsonb uuid serial bytea blob"),
    upperTypes: false,
    calls: true,
    caseless: true,
  },
  make: {
    line: ["#"],
    block: [],
    strings: [DQ, SQ],
    keywords: words("ifeq ifneq ifdef ifndef else endif include define endef export unexport override vpath"),
    builtins: words("shell wildcard patsubst subst strip findstring filter filter-out sort word words dir notdir suffix basename addsuffix addprefix join foreach call eval value origin error warning info abspath realpath"),
    constants: none,
    types: none,
    upperTypes: false,
    calls: false,
    sigils: true,
    makeTargets: true,
    identExtra: "-.",
  },
  dockerfile: {
    line: ["#"],
    block: [],
    strings: [DQ, SQ],
    keywords: words("from run cmd label maintainer expose env add copy entrypoint volume user workdir arg onbuild stopsignal healthcheck shell as"),
    builtins: none,
    constants: none,
    types: none,
    upperTypes: false,
    calls: false,
    sigils: true,
    lineKeywords: true,
    caseless: true,
  },
};

// the languages with a scanner of their own rather than a table
const SPECIAL = new Set(["yaml", "markup", "diff", "markdown"]);

const NAMES: Record<string, string> = {
  go: "go",
  golang: "go",
  js: "js",
  javascript: "js",
  jsx: "js",
  mjs: "js",
  cjs: "js",
  ts: "js",
  typescript: "js",
  tsx: "js",
  mts: "js",
  cts: "js",
  node: "js",
  py: "python",
  python: "python",
  python3: "python",
  pyi: "python",
  sh: "shell",
  bash: "shell",
  zsh: "shell",
  shell: "shell",
  console: "shell",
  fish: "shell",
  json: "json",
  jsonc: "json",
  json5: "json",
  yaml: "yaml",
  yml: "yaml",
  toml: "toml",
  ini: "ini",
  cfg: "ini",
  conf: "ini",
  properties: "ini",
  gitconfig: "ini",
  editorconfig: "ini",
  css: "css",
  scss: "css",
  less: "css",
  html: "markup",
  htm: "markup",
  xml: "markup",
  svg: "markup",
  xhtml: "markup",
  vue: "markup",
  svelte: "markup",
  rust: "rust",
  rs: "rust",
  sql: "sql",
  psql: "sql",
  mysql: "sql",
  sqlite: "sql",
  diff: "diff",
  patch: "diff",
  md: "markdown",
  markdown: "markdown",
  mdx: "markdown",
  dockerfile: "dockerfile",
  docker: "dockerfile",
  containerfile: "dockerfile",
  make: "make",
  makefile: "make",
  mk: "make",
  gnumakefile: "make",
};

// langOf is the language of a fence's info string ("ts", "ts title=x") or a file's name (Makefile, a.tsx, src/x.go); "" when none is known
export function langOf(nameOrInfo: string): string {
  const first = nameOrInfo.trim().split(/\s+/)[0] ?? "";
  if (!first) return "";
  const base = first.slice(first.lastIndexOf("/") + 1).toLowerCase();
  if (NAMES[base]) return NAMES[base]!;
  const dot = base.lastIndexOf(".");
  if (dot > 0) {
    const ext = base.slice(dot + 1);
    if (NAMES[ext]) return NAMES[ext]!;
  } else if (dot === 0 && NAMES[base.slice(1)]) {
    return NAMES[base.slice(1)]!;
  }
  return "";
}

// tokenize is the text as tokens of one language; an unknown language or a text past the cap is one plain token
export function tokenize(text: string, lang: string): Token[] {
  if (text.length === 0) return [];
  if (text.length > MAX_HIGHLIGHT || (!LANGS[lang] && !SPECIAL.has(lang))) return [{ kind: "plain", text }];
  const out: Token[] = [];
  switch (lang) {
    case "yaml":
      scanYaml(text, out);
      break;
    case "markup":
      scanMarkup(text, out);
      break;
    case "diff":
      scanDiff(text, out);
      break;
    case "markdown":
      scanMarkdown(text, out);
      break;
    default:
      scanCode(text, LANGS[lang]!, out);
  }
  return out;
}

// highlightLines is the text tokenized once and cut at its newlines, one array per line, so a comment or a string
// spanning lines keeps its kind on every line it touches
export function highlightLines(text: string, lang: string): Token[][] {
  const lines: Token[][] = [[]];
  for (const token of tokenize(text, lang)) {
    const parts = token.text.split("\n");
    for (let i = 0; i < parts.length; i++) {
      if (i > 0) lines.push([]);
      if (parts[i]) lines[lines.length - 1]!.push({ kind: token.kind, text: parts[i]! });
    }
  }
  return lines;
}

// splitTokens cuts a line's tokens at two character offsets, so a mark over a range can wrap whole tokens and parts of others
export function splitTokens(tokens: Token[], from: number, to: number): { before: Token[]; inside: Token[]; after: Token[] } {
  const before: Token[] = [];
  const inside: Token[] = [];
  const after: Token[] = [];
  let at = 0;
  for (const token of tokens) {
    const start = at;
    const end = at + token.text.length;
    at = end;
    const piece = (a: number, b: number, into: Token[]) => {
      const lo = Math.max(a, start);
      const hi = Math.min(b, end);
      if (hi > lo) into.push({ kind: token.kind, text: token.text.slice(lo - start, hi - start) });
    };
    piece(0, from, before);
    piece(from, to, inside);
    piece(to, Infinity, after);
  }
  return { before, inside, after };
}

// ---- the scanners ----

const push = (out: Token[], kind: TokenKind, text: string) => {
  if (!text) return;
  const last = out[out.length - 1];
  if (last && last.kind === kind) last.text += text;
  else out.push({ kind, text });
};

const isDigit = (c: number) => c >= 48 && c <= 57;
const isUpper = (c: number) => c >= 65 && c <= 90;
const isLower = (c: number) => c >= 97 && c <= 122;
const isLetter = (c: number) => isUpper(c) || isLower(c) || c > 127;
const isWord = (c: number) => isLetter(c) || isDigit(c) || c === 95;
const isSpace = (c: number) => c === 32 || c === 9 || c === 13;
const OPERATORS = "+-*/%=<>!&|^~?:";
const PUNCT = "()[]{},;.";
const allCaps = (word: string) => word.length > 1 && word === word.toUpperCase() && /[A-Z]/.test(word);

function scanCode(t: string, lang: Lang, out: Token[]) {
  const n = t.length;
  const extra = lang.identExtra ?? "";
  const identChar = (c: number) => isWord(c) || (extra !== "" && extra.includes(String.fromCharCode(c)));
  let i = 0;
  let lineStart = true; // nothing but blanks since the line began
  let command = true; // shell: the next word names a command
  let depth = 0; // css: inside a rule's braces (an at-rule's braces do not count: selectors stand inside @media)
  const braces: boolean[] = []; // css: whether each open brace belongs to an at-rule
  let atRule = false; // css: an at-rule keyword was read on this statement
  let lastKind: TokenKind | "start" = "start"; // what the previous token was, for the regex rule
  let lastText = "";
  const emit = (kind: TokenKind, text: string) => {
    push(out, kind, text);
    lastKind = kind;
    lastText = text;
  };
  const lineEnd = (from: number) => {
    const nl = t.indexOf("\n", from);
    return nl < 0 ? n : nl;
  };
  const scanString = (rule: StringRule, from: number): number => {
    let j = from + rule.open.length;
    while (j < n) {
      const c = t.charCodeAt(j);
      if (rule.escape && c === 92) {
        j += 2;
        continue;
      }
      if (c === 10 && !rule.multiline) return j;
      if (t.startsWith(rule.close, j)) return j + rule.close.length;
      j++;
    }
    return n;
  };
  const scanNumber = (from: number): number => {
    let j = from;
    if (t[j] === "0" && "xXbBoO".includes(t[j + 1] ?? " ")) {
      j += 2;
      while (j < n && isWord(t.charCodeAt(j))) j++;
      return j;
    }
    while (j < n && (isDigit(t.charCodeAt(j)) || t[j] === "_")) j++;
    if (t[j] === "." && isDigit(t.charCodeAt(j + 1))) {
      j++;
      while (j < n && (isDigit(t.charCodeAt(j)) || t[j] === "_")) j++;
    }
    if ((t[j] === "e" || t[j] === "E") && (isDigit(t.charCodeAt(j + 1)) || ((t[j + 1] === "+" || t[j + 1] === "-") && isDigit(t.charCodeAt(j + 2))))) {
      j += 2;
      while (j < n && isDigit(t.charCodeAt(j))) j++;
    }
    while (j < n && (isWord(t.charCodeAt(j)) || (lang.css && t[j] === "%"))) j++; // a suffix: 10n, 1u32, 12px
    return j;
  };
  const nextVisible = (from: number): string => {
    let j = from;
    while (j < n && isSpace(t.charCodeAt(j))) j++;
    return t[j] ?? "";
  };
  const atWordStart = (j: number) => j === 0 || !identChar(t.charCodeAt(j - 1));
  const lineKeywordAt = (j: number): boolean => {
    if (!lang.lineKeywords) return false;
    let k = j;
    while (k < n && isWord(t.charCodeAt(k))) k++;
    return lang.keywords.has(t.slice(j, k).toLowerCase());
  };
  while (i < n) {
    const c = t.charCodeAt(i);
    if (c === 10) {
      emit("plain", "\n");
      i++;
      lineStart = true;
      command = true;
      continue;
    }
    if (isSpace(c)) {
      let j = i + 1;
      while (j < n && isSpace(t.charCodeAt(j))) j++;
      push(out, "plain", t.slice(i, j));
      i = j;
      continue;
    }
    const wasLineStart = lineStart;
    lineStart = false;
    // a make target or variable, an ini section or key: decided by the line's shape
    if (wasLineStart && lang.makeTargets && t[i] !== "\t") {
      const line = t.slice(i, lineEnd(i));
      const variable = line.match(/^([A-Za-z0-9_.-]+)\s*(:=|::=|\?=|\+=|!=|=)/);
      const target = line.match(/^([A-Za-z0-9_./%$()@<^*-][A-Za-z0-9_./%$()@<^* -]*?)\s*:(?![=])/);
      if (variable) {
        emit("key", variable[1]!);
        i += variable[1]!.length;
        continue;
      }
      if (target) {
        emit("function", target[1]!);
        i += target[1]!.length;
        continue;
      }
    }
    if (wasLineStart && lang.iniKeys) {
      const line = t.slice(i, lineEnd(i));
      if (line.startsWith("[")) {
        const close = line.indexOf("]");
        emit("punct", "[");
        emit("tag", close < 0 ? line.slice(1) : line.slice(1, close));
        if (close >= 0) emit("punct", "]");
        i += close < 0 ? line.length : close + 1;
        continue;
      }
      const key = line.match(/^([A-Za-z0-9_.\-"']+(?: +[A-Za-z0-9_.\-"']+)*) *=/);
      if (key) {
        emit("key", key[1]!);
        i += key[1]!.length;
        continue;
      }
    }
    // comments
    let matched = false;
    for (const [open, close] of lang.block) {
      if (t.startsWith(open, i)) {
        const j = t.indexOf(close, i + open.length);
        const end = j < 0 ? n : j + close.length;
        emit("comment", t.slice(i, end));
        i = end;
        matched = true;
        break;
      }
    }
    if (matched) continue;
    for (const marker of lang.line) {
      const hashOK = marker !== "#" || !lang.hashAfterSpace || i === 0 || isSpace(t.charCodeAt(i - 1)) || t[i - 1] === "\n" || t[i - 1] === ";";
      if (t.startsWith(marker, i) && hashOK) {
        const end = lineEnd(i);
        emit("comment", t.slice(i, end));
        i = end;
        matched = true;
        break;
      }
    }
    if (matched) continue;
    // rust: a character or a lifetime, before the string rules see the quote
    if (lang.rustChars && t[i] === "'") {
      if (t[i + 1] === "\\") {
        const close = t.indexOf("'", i + 2);
        if (close > 0 && close - i <= 12) {
          emit("string", t.slice(i, close + 1));
          i = close + 1;
          continue;
        }
      } else if (t[i + 2] === "'" && t[i + 1] !== "\n") {
        emit("string", t.slice(i, i + 3));
        i += 3;
        continue;
      }
      let j = i + 1;
      while (j < n && isWord(t.charCodeAt(j))) j++;
      emit("plain", t.slice(i, j));
      i = j;
      continue;
    }
    // strings, with a letter prefix where the language has them (f"…", r#"…"#)
    let prefix = 0;
    if (lang.stringPrefixes && isLetter(c) && atWordStart(i)) {
      for (const p of lang.stringPrefixes) {
        const after = t[i + p.length];
        if (t.startsWith(p, i) && (after === '"' || after === "'" || (lang.macros && after === "#"))) {
          prefix = p.length;
          break;
        }
      }
    }
    if (lang.macros && prefix > 0 && t[i + prefix] === "#") {
      let hashes = 0;
      while (t[i + prefix + hashes] === "#") hashes++;
      if (t[i + prefix + hashes] === '"') {
        const close = '"' + "#".repeat(hashes);
        const j = t.indexOf(close, i + prefix + hashes + 1);
        const end = j < 0 ? n : j + close.length;
        emit("string", t.slice(i, end));
        i = end;
        continue;
      }
      prefix = 0;
    }
    for (const rule of lang.strings) {
      if (t.startsWith(rule.open, i + prefix)) {
        const end = scanString(rule, i + prefix);
        emit(lang.dataKeys && nextVisible(end) === ":" ? "key" : "string", t.slice(i, end));
        i = end;
        matched = true;
        break;
      }
    }
    if (matched) continue;
    // a shell or make variable: $name, ${name}, $(name), $1, $@
    if (lang.sigils && t[i] === "$") {
      let j = i + 1;
      if (t[j] === "{") {
        const close = t.indexOf("}", j);
        j = close < 0 ? n : close + 1;
      } else if (t[j] === "(" && lang.makeTargets) {
        const close = t.indexOf(")", j);
        j = close < 0 ? n : close + 1;
      } else if (t[j] === "(") {
        j++; // a subshell: the command inside is read as usual
      } else if (j < n && "@#?!*$-<^0123456789".includes(t[j]!)) {
        j++;
      } else {
        while (j < n && isWord(t.charCodeAt(j))) j++;
      }
      emit("builtin", t.slice(i, j));
      i = j;
      continue;
    }
    // a decorator, an at-rule
    if (lang.decorators && t[i] === "@" && isLetter(t.charCodeAt(i + 1))) {
      let j = i + 1;
      while (j < n && (identChar(t.charCodeAt(j)) || t[j] === ".")) j++;
      emit(lang.css ? "keyword" : "attr", t.slice(i, j));
      i = j;
      if (lang.css) atRule = true;
      continue;
    }
    // rust attributes #[…] and #![…]
    if (lang.macros && t[i] === "#" && (t[i + 1] === "[" || (t[i + 1] === "!" && t[i + 2] === "["))) {
      const close = t.indexOf("]", i);
      const end = close < 0 ? lineEnd(i) : close + 1;
      emit("attr", t.slice(i, end));
      i = end;
      continue;
    }
    // css: a class, an id, a colour, a pseudo-class, a custom property
    if (lang.css) {
      if (t[i] === "#") {
        let j = i + 1;
        while (j < n && (depth === 0 ? identChar(t.charCodeAt(j)) : isWord(t.charCodeAt(j)))) j++;
        emit(depth === 0 ? "attr" : "number", t.slice(i, j));
        i = j;
        continue;
      }
      if (t[i] === "." && depth === 0 && isLetter(t.charCodeAt(i + 1))) {
        let j = i + 1;
        while (j < n && identChar(t.charCodeAt(j))) j++;
        emit("attr", t.slice(i, j));
        i = j;
        continue;
      }
      if (t[i] === ":" && depth === 0 && (isLetter(t.charCodeAt(i + 1)) || t[i + 1] === ":")) {
        let j = i + 1;
        if (t[j] === ":") j++;
        while (j < n && identChar(t.charCodeAt(j))) j++;
        emit("builtin", t.slice(i, j));
        i = j;
        continue;
      }
      if (t[i] === "-" && t[i + 1] === "-" && isLetter(t.charCodeAt(i + 2))) {
        let j = i + 2;
        while (j < n && identChar(t.charCodeAt(j))) j++;
        emit(depth > 0 && nextVisible(j) === ":" ? "key" : "builtin", t.slice(i, j));
        i = j;
        continue;
      }
      if (t[i] === "!" && t.startsWith("important", i + 1)) {
        emit("keyword", "!important");
        i += 10;
        continue;
      }
    }
    // a shell flag
    if (lang.commands && t[i] === "-" && (isLetter(t.charCodeAt(i + 1)) || t[i + 1] === "-") && (i === 0 || isSpace(t.charCodeAt(i - 1)))) {
      let j = i + 1;
      while (j < n && (isWord(t.charCodeAt(j)) || t[j] === "-")) j++;
      emit("attr", t.slice(i, j));
      i = j;
      command = false;
      continue;
    }
    // numbers
    if ((isDigit(c) || (t[i] === "." && isDigit(t.charCodeAt(i + 1)) && !identChar(t.charCodeAt(i - 1)))) && (!lang.commands || atWordStart(i))) {
      const end = scanNumber(i);
      emit("number", t.slice(i, end));
      i = end;
      continue;
    }
    // identifiers
    if (isLetter(c) || c === 95 || c === 36 && extra.includes("$")) {
      let j = i + 1;
      while (j < n && identChar(t.charCodeAt(j))) j++;
      const word = t.slice(i, j);
      const lookup = lang.caseless ? word.toLowerCase() : word;
      let kind: TokenKind = "plain";
      if (wasLineStart && lineKeywordAt(i)) kind = "keyword";
      else if (lang.css) {
        if (depth === 0) kind = "tag";
        else if (nextVisible(j) === ":") kind = "key";
        else if (lang.builtins.has(lookup) && t[j] === "(") kind = "function";
        else if (lang.keywords.has(lookup)) kind = "keyword";
        else if (lang.constants.has(lookup)) kind = "constant";
      } else if (!lang.lineKeywords && lang.keywords.has(lookup)) kind = "keyword";
      else if (lang.constants.has(word)) kind = "constant";
      else if (lang.types.has(lookup)) kind = "type";
      else if (lang.macros && t[j] === "!" && t[j + 1] !== "=") {
        kind = "function";
        j++;
      } else if (lang.builtins.has(lookup)) kind = "builtin";
      else if (lang.commands) kind = command ? "function" : "plain";
      else if (lang.calls && t[j] === "(") kind = "function";
      else if (lang.upperTypes && isUpper(c)) kind = allCaps(word) ? "constant" : "type";
      emit(kind, t.slice(i, j));
      i = j;
      if (lang.commands) command = kind === "keyword" && word !== "in";
      continue;
    }
    // a regex literal where a value may start
    if (lang.regex && t[i] === "/" && t[i + 1] !== "/" && t[i + 1] !== "*" && regexMayStart(lastKind, lastText)) {
      let j = i + 1;
      let inClass = false;
      let closed = false;
      while (j < n) {
        const d = t.charCodeAt(j);
        if (d === 10) break;
        if (d === 92) {
          j += 2;
          continue;
        }
        if (inClass) {
          if (d === 93) inClass = false;
        } else if (d === 91) inClass = true;
        else if (d === 47) {
          closed = true;
          j++;
          break;
        }
        j++;
      }
      if (closed) {
        while (j < n && isLower(t.charCodeAt(j))) j++;
        emit("string", t.slice(i, j));
        i = j;
        continue;
      }
    }
    if (OPERATORS.includes(t[i]!)) {
      let j = i + 1;
      while (j < n && OPERATORS.includes(t[j]!)) j++;
      const op = t.slice(i, j);
      emit("operator", op);
      i = j;
      if (lang.commands && (op === "|" || op === "||" || op === "&&" || op === "!")) command = true;
      continue;
    }
    if (PUNCT.includes(t[i]!)) {
      emit("punct", t[i]!);
      if (lang.css) {
        if (t[i] === "{") {
          braces.push(atRule);
          if (!atRule) depth++;
          atRule = false;
        } else if (t[i] === "}") {
          if (braces.pop() === false) depth = Math.max(0, depth - 1);
        } else if (t[i] === ";") atRule = false;
      }
      if (lang.commands && (t[i] === ";" || t[i] === "(")) command = true;
      i++;
      continue;
    }
    emit("plain", t[i]!);
    i++;
  }
}

// regexMayStart says whether a / here opens a regex rather than divides: after an operator, an opening bracket, a keyword or nothing
function regexMayStart(lastKind: TokenKind | "start", lastText: string): boolean {
  if (lastKind === "start" || lastKind === "operator" || lastKind === "keyword") return true;
  if (lastKind === "punct") return lastText !== ")" && lastText !== "]";
  if (lastKind === "plain") return lastText.endsWith("\n");
  return false;
}

// ---- yaml ----

const YAML_CONSTANTS = new Set(["true", "false", "null", "yes", "no", "on", "off", "~", "True", "False", "Null", "Yes", "No", "On", "Off", "TRUE", "FALSE", "NULL"]);
const YAML_KEY = /^("(?:[^"\\]|\\.)*"|'[^']*'|[^\s#"'{}[\],&*!|>%@`-][^#:]*?) *:(?= |$)/;
const YAML_NUMBER = /^[-+]?(\d[\d_]*(\.\d+)?([eE][-+]?\d+)?|0x[\da-fA-F]+|\.inf|\.nan)$/;

function scanYaml(t: string, out: Token[]) {
  let blockIndent = -1; // inside a | or > scalar: lines deeper than this are text
  for (const line of lines(t)) {
    if (line === "\n") {
      push(out, "plain", "\n");
      continue;
    }
    let j = 0;
    while (j < line.length && line[j] === " ") j++;
    if (blockIndent >= 0) {
      if (j > blockIndent || line.trim() === "") {
        push(out, "string", line);
        continue;
      }
      blockIndent = -1;
    }
    push(out, "plain", line.slice(0, j));
    let k = j;
    if (line.startsWith("- ", k) || line.slice(k) === "-") {
      push(out, "punct", "-");
      k++;
      let m = k;
      while (m < line.length && line[m] === " ") m++;
      push(out, "plain", line.slice(k, m));
      k = m;
    } else if (line.slice(k) === "---" || line.slice(k) === "..." || line.startsWith("--- ", k)) {
      push(out, "punct", line.slice(k, k + 3));
      k += 3;
    }
    const key = line.slice(k).match(YAML_KEY);
    if (key) {
      push(out, "key", key[1]!);
      k += key[1]!.length;
      const colon = line.indexOf(":", k);
      push(out, "plain", line.slice(k, colon));
      push(out, "punct", ":");
      k = colon + 1;
    }
    while (k < line.length) {
      const c = line[k]!;
      if (c === " ") {
        let m = k + 1;
        while (m < line.length && line[m] === " ") m++;
        push(out, "plain", line.slice(k, m));
        k = m;
        continue;
      }
      if (c === "#" && (k === 0 || line[k - 1] === " ")) {
        push(out, "comment", line.slice(k));
        break;
      }
      if (c === '"' || c === "'") {
        let m = k + 1;
        while (m < line.length && line[m] !== c) m += c === '"' && line[m] === "\\" ? 2 : 1;
        const end = Math.min(m + 1, line.length);
        push(out, "string", line.slice(k, end));
        k = end;
        continue;
      }
      if ((c === "|" || c === ">") && (k + 1 === line.length || " -+0123456789".includes(line[k + 1]!))) {
        push(out, "operator", line.slice(k));
        blockIndent = j;
        break;
      }
      if (c === "&" || c === "*" || c === "!") {
        let m = k + 1;
        while (m < line.length && line[m] !== " " && line[m] !== ",") m++;
        push(out, c === "!" ? "tag" : "attr", line.slice(k, m));
        k = m;
        continue;
      }
      if ("[]{},".includes(c)) {
        push(out, "punct", c);
        k++;
        continue;
      }
      let m = k;
      while (m < line.length && !" ,[]{}".includes(line[m]!) && !(line[m] === "#" && line[m - 1] === " ")) m++;
      const scalar = line.slice(k, m);
      const inner = line.slice(k).match(YAML_KEY); // a key inside a flow mapping, {a: 1}
      if (inner && inner[1]!.length < scalar.length + 1 && line[k + inner[1]!.length] !== undefined && line.slice(k + inner[1]!.length).trimStart().startsWith(":")) {
        push(out, "key", inner[1]!);
        k += inner[1]!.length;
        continue;
      }
      push(out, YAML_CONSTANTS.has(scalar) ? "constant" : YAML_NUMBER.test(scalar) ? "number" : "plain", scalar);
      k = m;
    }
  }
}

// ---- markup: html, xml, svg ----

function scanMarkup(t: string, out: Token[]) {
  const n = t.length;
  let i = 0;
  while (i < n) {
    if (t.startsWith("<!--", i)) {
      const close = t.indexOf("-->", i + 4);
      const end = close < 0 ? n : close + 3;
      push(out, "comment", t.slice(i, end));
      i = end;
      continue;
    }
    if (t.startsWith("<!", i) || t.startsWith("<?", i)) {
      const close = t.indexOf(">", i);
      const end = close < 0 ? n : close + 1;
      push(out, "keyword", t.slice(i, end));
      i = end;
      continue;
    }
    if (t[i] === "<" && (isLetter(t.charCodeAt(i + 1)) || (t[i + 1] === "/" && isLetter(t.charCodeAt(i + 2))))) {
      let j = i + 1;
      if (t[j] === "/") j++;
      push(out, "punct", t.slice(i, j));
      const nameStart = j;
      while (j < n && (isWord(t.charCodeAt(j)) || t[j] === "-" || t[j] === ":" || t[j] === ".")) j++;
      const name = t.slice(nameStart, j);
      push(out, "tag", name);
      i = j;
      let selfClosed = false;
      while (i < n) {
        const c = t.charCodeAt(i);
        if (c === 10 || isSpace(c)) {
          let m = i + 1;
          while (m < n && (t.charCodeAt(m) === 10 || isSpace(t.charCodeAt(m)))) m++;
          push(out, "plain", t.slice(i, m));
          i = m;
          continue;
        }
        if (t[i] === ">" || t.startsWith("/>", i)) {
          selfClosed = t[i] === "/";
          const len = selfClosed ? 2 : 1;
          push(out, "punct", t.slice(i, i + len));
          i += len;
          break;
        }
        if (t[i] === "=") {
          push(out, "operator", "=");
          i++;
          continue;
        }
        if (t[i] === '"' || t[i] === "'") {
          const close = t.indexOf(t[i]!, i + 1);
          const end = close < 0 ? n : close + 1;
          push(out, "string", t.slice(i, end));
          i = end;
          continue;
        }
        if (t[i] === "{") {
          // a jsx or svelte expression, to its matching brace, read as javascript
          let depth = 0;
          let m = i;
          while (m < n) {
            if (t[m] === "{") depth++;
            else if (t[m] === "}" && --depth === 0) {
              m++;
              break;
            }
            m++;
          }
          push(out, "punct", "{");
          for (const token of tokenize(t.slice(i + 1, Math.max(i + 1, m - 1)), "js")) push(out, token.kind, token.text);
          push(out, "punct", "}");
          i = m;
          continue;
        }
        let m = i;
        while (m < n && !isSpace(t.charCodeAt(m)) && t[m] !== "\n" && t[m] !== "=" && t[m] !== ">" && !t.startsWith("/>", m)) m++;
        if (m === i) m++;
        push(out, "attr", t.slice(i, m));
        i = m;
      }
      // the content of a script or style element is its own language
      const inner = name.toLowerCase() === "script" ? "js" : name.toLowerCase() === "style" ? "css" : "";
      if (inner && !selfClosed && t[nameStart - 1] !== "/") {
        const close = t.toLowerCase().indexOf("</" + name.toLowerCase(), i);
        const end = close < 0 ? n : close;
        for (const token of tokenize(t.slice(i, end), inner)) push(out, token.kind, token.text);
        i = end;
      }
      continue;
    }
    if (t[i] === "&") {
      let j = i + 1;
      while (j < n && (isWord(t.charCodeAt(j)) || t[j] === "#")) j++;
      if (t[j] === ";") {
        push(out, "constant", t.slice(i, j + 1));
        i = j + 1;
        continue;
      }
    }
    let j = i + 1;
    while (j < n && t[j] !== "<" && t[j] !== "&") j++;
    push(out, "plain", t.slice(i, j));
    i = j;
  }
}

// ---- diff ----

function scanDiff(t: string, out: Token[]) {
  for (const line of lines(t)) {
    if (line === "\n") {
      push(out, "plain", "\n");
      continue;
    }
    let kind: TokenKind = "plain";
    if (/^(\+\+\+|---|diff |index |new file|deleted file|similarity|rename |Binary)/.test(line)) kind = "comment";
    else if (line.startsWith("@@")) kind = "keyword";
    else if (line.startsWith("+")) kind = "string";
    else if (line.startsWith("-")) kind = "builtin";
    push(out, kind, line);
  }
}

// lines cuts a text into its lines and the newlines between them, both kept, so the pieces concatenate back to the text
function lines(t: string): string[] {
  const out: string[] = [];
  let i = 0;
  while (i < t.length) {
    const nl = t.indexOf("\n", i);
    if (nl < 0) {
      out.push(t.slice(i));
      break;
    }
    if (nl > i) out.push(t.slice(i, nl));
    out.push("\n");
    i = nl + 1;
  }
  return out;
}

// ---- markdown: headings, fences (their code in its own language), quotes, list marks, inline code, links ----

const FENCE_OPEN = /^\s*(`{3,}|~{3,})\s*(\S*)/;
const HEADING = /^\s{0,3}#{1,6}\s/;
const LIST_MARK = /^\s*([-*+]|\d+\.)\s/;
const RULE = /^\s*(---+|\*\*\*+|___+)\s*$/;

function scanMarkdown(t: string, out: Token[]) {
  let fence: { marker: string; lang: string; code: string[] } | null = null;
  const flushFence = () => {
    if (fence) for (const token of tokenize(fence.code.join(""), fence.lang)) push(out, token.kind, token.text);
  };
  for (const line of lines(t)) {
    if (fence) {
      if (line !== "\n" && line.trim().startsWith(fence.marker)) {
        flushFence();
        push(out, "punct", line);
        fence = null;
      } else fence.code.push(line);
      continue;
    }
    if (line === "\n") {
      push(out, "plain", "\n");
      continue;
    }
    const open = line.match(FENCE_OPEN);
    if (open) {
      push(out, "punct", line);
      fence = { marker: open[1]!, lang: langOf(open[2] ?? ""), code: [] };
      continue;
    }
    if (HEADING.test(line)) {
      push(out, "keyword", line);
      continue;
    }
    if (/^\s*>/.test(line)) {
      push(out, "comment", line);
      continue;
    }
    if (RULE.test(line)) {
      push(out, "punct", line);
      continue;
    }
    const mark = line.match(LIST_MARK);
    if (mark) {
      push(out, "punct", mark[0]);
      scanInline(line.slice(mark[0].length), out);
      continue;
    }
    scanInline(line, out);
  }
  flushFence();
}

function scanInline(line: string, out: Token[]) {
  const n = line.length;
  let i = 0;
  while (i < n) {
    if (line[i] === "`") {
      let ticks = 0;
      while (line[i + ticks] === "`") ticks++;
      const close = line.indexOf("`".repeat(ticks), i + ticks);
      if (close > 0) {
        push(out, "string", line.slice(i, close + ticks));
        i = close + ticks;
        continue;
      }
      push(out, "plain", line.slice(i, i + ticks));
      i += ticks;
      continue;
    }
    if (line[i] === "[") {
      const close = line.indexOf("](", i);
      const end = close > 0 ? line.indexOf(")", close) : -1;
      if (close > 0 && end > 0) {
        push(out, "plain", line.slice(i, close + 2));
        push(out, "attr", line.slice(close + 2, end));
        push(out, "plain", ")");
        i = end + 1;
        continue;
      }
    }
    let j = i + 1;
    while (j < n && line[j] !== "`" && line[j] !== "[") j++;
    push(out, "plain", line.slice(i, j));
    i = j;
  }
}

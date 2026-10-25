import { describe, expect, it } from "vitest";
import { highlightLines, langOf, MAX_HIGHLIGHT, splitTokens, tokenize, type Token, type TokenKind } from "./highlight";

// kinds lists the kind of every token whose text holds the given piece, so a test reads "this word is a keyword"
const kindOf = (tokens: Token[], text: string): TokenKind | undefined =>
  (tokens.find((t) => t.text === text) ?? tokens.find((t) => t.kind !== "plain" && t.text.includes(text)) ?? tokens.find((t) => t.text.includes(text)))?.kind;
const joined = (tokens: Token[]) => tokens.map((t) => t.text).join("");

describe("langOf", () => {
  it("reads a fence's info string, with or without more words", () => {
    expect(langOf("go")).toBe("go");
    expect(langOf("golang")).toBe("go");
    expect(langOf("ts title=\"x.ts\"")).toBe("js");
    expect(langOf("tsx")).toBe("js");
    expect(langOf("python3")).toBe("python");
    expect(langOf("bash")).toBe("shell");
    expect(langOf("yml")).toBe("yaml");
    expect(langOf("Dockerfile")).toBe("dockerfile");
    expect(langOf("")).toBe("");
    expect(langOf("brainfuck")).toBe("");
  });
  it("reads a file's name and its extension", () => {
    expect(langOf("internal/server/roster.go")).toBe("go");
    expect(langOf("web/src/app/Shell.tsx")).toBe("js");
    expect(langOf("Makefile")).toBe("make");
    expect(langOf("GNUmakefile")).toBe("make");
    expect(langOf(".github/workflows/ci.yml")).toBe("yaml");
    expect(langOf("web/package.json")).toBe("json");
    expect(langOf("styles.css")).toBe("css");
    expect(langOf("icon.svg")).toBe("markup");
    expect(langOf("README.md")).toBe("markdown");
    expect(langOf(".gitconfig")).toBe("ini");
    expect(langOf("go.mod")).toBe("");
    expect(langOf("go.sum")).toBe("");
    expect(langOf(".gitignore")).toBe("");
    expect(langOf("LICENSE")).toBe("");
  });
});

describe("tokenize", () => {
  it("gives the text back whole, whatever the language", () => {
    const samples = ['package main\nfunc main() { fmt.Println("hi") }', "const a = `x${1}`; // y\n/* z */", "a:\n  - b: 1 # c\n", "<a href=\"x\">&amp; y</a>", "# T\n\n- `c` [l](u)\n```go\nvar x = 1\n```\n", "@@ -1 +1 @@\n-a\n+b\n"];
    const langs = ["go", "js", "yaml", "markup", "markdown", "diff"];
    samples.forEach((text, i) => expect(joined(tokenize(text, langs[i]!))).toBe(text));
  });
  it("is plain for an unknown language and past the cap", () => {
    expect(tokenize("x = 1", "")).toEqual([{ kind: "plain", text: "x = 1" }]);
    expect(tokenize("x = 1", "cobol")).toEqual([{ kind: "plain", text: "x = 1" }]);
    const big = "a".repeat(MAX_HIGHLIGHT + 1);
    expect(tokenize(big, "go")).toEqual([{ kind: "plain", text: big }]);
    expect(tokenize("", "go")).toEqual([]);
  });

  it("go: comments, strings with escapes, a raw string, numbers, keywords, builtins, types, a call", () => {
    const code = 'package main // the entry\n/* a\nblock */\nfunc Run(n int) error {\n\ts := "a\\"b" + `raw`\n\tx := 0x1F + 2.5e3\n\tif n == 0 { return nil }\n\treturn fmt.Errorf("%d", len(s))\n}';
    const t = tokenize(code, "go");
    expect(kindOf(t, "// the entry")).toBe("comment");
    expect(kindOf(t, "/* a\nblock */")).toBe("comment");
    expect(kindOf(t, 'package')).toBe("keyword");
    expect(kindOf(t, "func")).toBe("keyword");
    expect(kindOf(t, '"a\\"b"')).toBe("string");
    expect(kindOf(t, "`raw`")).toBe("string");
    expect(kindOf(t, "0x1F")).toBe("number");
    expect(kindOf(t, "2.5e3")).toBe("number");
    expect(kindOf(t, "int")).toBe("type");
    expect(kindOf(t, "error")).toBe("type");
    expect(kindOf(t, "nil")).toBe("constant");
    expect(kindOf(t, "len")).toBe("builtin");
    expect(kindOf(t, "Run")).toBe("function");
    expect(kindOf(t, "Errorf")).toBe("function");
    expect(kindOf(t, ":=")).toBe("operator");
  });
  it("typescript: template strings across lines, a regex, decorators, types and constants", () => {
    const code = "// c\nconst re = /a\\/b[/]/gi;\nconst t = `one\n${x} two`;\nlet n: number = a / b;\nexport class Foo extends Bar { @log run(): Promise<void> { return null; } }\nconst MAX = 10n;";
    const t = tokenize(code, "js");
    expect(kindOf(t, "// c")).toBe("comment");
    expect(kindOf(t, "/a\\/b[/]/gi")).toBe("string");
    expect(kindOf(t, "`one\n${x} two`")).toBe("string");
    expect(kindOf(t, "number")).toBe("type");
    expect(kindOf(t, "Foo")).toBe("type");
    expect(kindOf(t, "@log")).toBe("attr");
    expect(kindOf(t, "run")).toBe("function");
    expect(kindOf(t, "null")).toBe("constant");
    expect(kindOf(t, "MAX")).toBe("constant");
    expect(kindOf(t, "10n")).toBe("number");
    const divide = tokenize("a / b / c", "js");
    expect(divide.filter((x) => x.kind === "string")).toEqual([]);
  });
  it("python: prefixed and triple-quoted strings, decorators, builtins, constants", () => {
    const code = '@dataclass\nclass Point:\n    """A point\n    in space"""\n    def norm(self) -> float:\n        return f"{self.x!r}" if self.x is not None else r"\\d"  # tail\n';
    const t = tokenize(code, "python");
    expect(kindOf(t, "@dataclass")).toBe("attr");
    expect(kindOf(t, "class")).toBe("keyword");
    expect(kindOf(t, "Point")).toBe("type");
    expect(kindOf(t, '"""A point\n    in space"""')).toBe("string");
    expect(kindOf(t, 'f"{self.x!r}"')).toBe("string");
    expect(kindOf(t, 'r"\\d"')).toBe("string");
    expect(kindOf(t, "None")).toBe("constant");
    expect(kindOf(t, "self")).toBe("builtin");
    expect(kindOf(t, "float")).toBe("type");
    expect(kindOf(t, "# tail")).toBe("comment");
  });
  it("shell: commands, flags, variables, strings and a comment only at a word's start", () => {
    const code = 'export FOO=1 # set\necho "$FOO ${BAR}" | grep -v --count x; cd /tmp && ls -la\nif [ $# -gt 0 ]; then echo yes; fi\nsha256sum file';
    const t = tokenize(code, "shell");
    expect(kindOf(t, "export")).toBe("builtin");
    expect(kindOf(t, "# set")).toBe("comment");
    expect(kindOf(t, '"$FOO ${BAR}"')).toBe("string");
    expect(kindOf(t, "grep")).toBe("function");
    expect(kindOf(t, "-v")).toBe("attr");
    expect(kindOf(t, "--count")).toBe("attr");
    expect(kindOf(t, "ls")).toBe("function");
    expect(kindOf(t, "$#")).toBe("builtin");
    expect(kindOf(t, "then")).toBe("keyword");
    expect(t.find((x) => x.text.includes("sha256sum"))?.kind).toBe("function");
    expect(t.some((x) => x.kind === "comment" && x.text.startsWith("# -gt"))).toBe(false);
  });
  it("json: keys before a colon, strings, numbers, constants", () => {
    const t = tokenize('{"name": "remuda", "n": 3.5, "ok": true, "none": null, "list": ["a"]}', "json");
    expect(kindOf(t, '"name"')).toBe("key");
    expect(kindOf(t, '"remuda"')).toBe("string");
    expect(kindOf(t, "3.5")).toBe("number");
    expect(kindOf(t, "true")).toBe("constant");
    expect(kindOf(t, "null")).toBe("constant");
    expect(kindOf(t, '"a"')).toBe("string");
  });
  it("yaml: keys, list marks, comments, anchors, block scalars and a flow mapping", () => {
    const code = "name: remuda # the app\nversion: 1.2\non:\n  push:\n    branches: [main, release]\n- item: &anchor yes\n- *anchor\nscript: |\n  echo hi\n  echo there\nnext: 'q'\n";
    const t = tokenize(code, "yaml");
    expect(kindOf(t, "name")).toBe("key");
    expect(kindOf(t, "# the app")).toBe("comment");
    expect(kindOf(t, "1.2")).toBe("number");
    expect(kindOf(t, "branches")).toBe("key");
    expect(kindOf(t, "&anchor")).toBe("attr");
    expect(kindOf(t, "*anchor")).toBe("attr");
    expect(kindOf(t, "yes")).toBe("constant");
    expect(kindOf(t, "  echo hi")).toBe("string");
    expect(kindOf(t, "  echo there")).toBe("string");
    expect(kindOf(t, "next")).toBe("key");
    expect(kindOf(t, "'q'")).toBe("string");
    expect(joined(t)).toBe(code);
  });
  it("markup: tags, attributes, strings, comments, entities, a style element in css", () => {
    const code = '<!doctype html>\n<!-- note -->\n<div class="a" data-x=1>&amp; text</div>\n<style>.a { color: red; }</style>\n<br/>';
    const t = tokenize(code, "markup");
    expect(kindOf(t, "<!doctype html>")).toBe("keyword");
    expect(kindOf(t, "<!-- note -->")).toBe("comment");
    expect(kindOf(t, "div")).toBe("tag");
    expect(kindOf(t, "class")).toBe("attr");
    expect(kindOf(t, '"a"')).toBe("string");
    expect(kindOf(t, "&amp;")).toBe("constant");
    expect(kindOf(t, "color")).toBe("key");
    expect(kindOf(t, ".a")).toBe("attr");
    expect(joined(t)).toBe(code);
  });
  it("css: selectors, classes, pseudo-classes, properties, numbers with units, colours, functions, at-rules", () => {
    const code = "@media (max-width: 920px) {\n  .rail:hover, a::after { color: #fff; width: calc(100% - 2px); --gap: 4px; margin: var(--gap) !important; }\n}";
    const t = tokenize(code, "css");
    expect(kindOf(t, "@media")).toBe("keyword");
    expect(kindOf(t, ".rail")).toBe("attr");
    expect(kindOf(t, ":hover")).toBe("builtin");
    expect(kindOf(t, "::after")).toBe("builtin");
    expect(kindOf(t, "a")).toBe("tag");
    expect(kindOf(t, "color")).toBe("key");
    expect(kindOf(t, "#fff")).toBe("number");
    expect(kindOf(t, "100%")).toBe("number");
    expect(kindOf(t, "calc")).toBe("function");
    expect(kindOf(t, "--gap")).toBe("key");
    expect(kindOf(t, "!important")).toBe("keyword");
  });
  it("rust: lifetimes stay plain, chars and raw strings are strings, macros are functions, attributes are attributes", () => {
    const code = "#[derive(Debug)]\nfn f<'a>(s: &'a str) -> Option<char> {\n    let c = 'x'; let n = '\\n'; let r = r#\"raw \"q\"\"#;\n    println!(\"{}\", s); Some(c)\n}";
    const t = tokenize(code, "rust");
    expect(kindOf(t, "#[derive(Debug)]")).toBe("attr");
    expect(kindOf(t, "fn")).toBe("keyword");
    expect(t.find((x) => x.text.includes("'a"))?.kind).toBe("plain");
    expect(kindOf(t, "'x'")).toBe("string");
    expect(kindOf(t, "'\\n'")).toBe("string");
    expect(kindOf(t, 'r#"raw "q""#')).toBe("string");
    expect(kindOf(t, "println!")).toBe("function");
    expect(kindOf(t, "Option")).toBe("type");
    expect(kindOf(t, "Some")).toBe("constant");
  });
  it("sql: keywords in any case, functions, strings, a comment", () => {
    const t = tokenize("SELECT count(*), my_func(id) AS n FROM tasks WHERE done = 1 and title like 'a%'; -- all", "sql");
    expect(kindOf(t, "SELECT")).toBe("keyword");
    expect(kindOf(t, "and")).toBe("keyword");
    expect(kindOf(t, "count")).toBe("builtin");
    expect(kindOf(t, "my_func")).toBe("function");
    expect(kindOf(t, "'a%'")).toBe("string");
    expect(kindOf(t, "-- all")).toBe("comment");
  });
  it("make: targets, variables, functions, keywords", () => {
    const code = "VERSION := 1.0\n.PHONY: build\nbuild: web\n\tgo build -o bin/x $(shell pwd) $(VERSION)\nifeq ($(OS),Darwin)\nendif\n";
    const t = tokenize(code, "make");
    expect(kindOf(t, "VERSION")).toBe("key");
    expect(kindOf(t, ".PHONY")).toBe("function");
    expect(kindOf(t, "build")).toBe("function");
    expect(kindOf(t, "$(shell pwd)")).toBe("builtin");
    expect(kindOf(t, "ifeq")).toBe("keyword");
    expect(joined(t)).toBe(code);
  });
  it("dockerfile: instructions, variables, a comment", () => {
    const t = tokenize("# base\nFROM golang:1.27 AS build\nRUN go build -o /out/remuda ./cmd/remuda && echo $HOME\nenv X=1", "dockerfile");
    expect(kindOf(t, "# base")).toBe("comment");
    expect(kindOf(t, "FROM")).toBe("keyword");
    expect(kindOf(t, "RUN")).toBe("keyword");
    expect(kindOf(t, "env")).toBe("keyword");
    expect(kindOf(t, "$HOME")).toBe("builtin");
    expect(kindOf(t, "AS")).toBe("plain");
  });
  it("ini and toml: sections, keys, strings, numbers, constants", () => {
    const ini = tokenize("[core]\n\tname = x\nauto crlf = true ; note\n", "ini");
    expect(kindOf(ini, "core")).toBe("tag");
    expect(kindOf(ini, "name")).toBe("key");
    expect(kindOf(ini, "auto crlf")).toBe("key");
    expect(kindOf(ini, "true")).toBe("constant");
    expect(kindOf(ini, "; note")).toBe("comment");
    const toml = tokenize('[package]\nname = "remuda"\nversion = 1 # c\n', "toml");
    expect(kindOf(toml, "package")).toBe("tag");
    expect(kindOf(toml, '"remuda"')).toBe("string");
    expect(kindOf(toml, "# c")).toBe("comment");
  });
  it("diff: added, removed, hunk heads and file heads", () => {
    const t = tokenize("--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@ func\n-old\n+new\n same", "diff");
    expect(kindOf(t, "--- a/x")).toBe("comment");
    expect(kindOf(t, "@@ -1,2 +1,2 @@ func")).toBe("keyword");
    expect(kindOf(t, "-old")).toBe("builtin");
    expect(kindOf(t, "+new")).toBe("string");
    expect(kindOf(t, " same")).toBe("plain");
  });
  it("markdown: headings, quotes, list marks, inline code, links, and a fence in its own language", () => {
    const code = "# Title\n\n> quoted\n\n- item with `code` and [a link](https://x.y)\n\n```go\nvar x = 1\n```\n\ntail";
    const t = tokenize(code, "markdown");
    expect(kindOf(t, "# Title")).toBe("keyword");
    expect(kindOf(t, "> quoted")).toBe("comment");
    expect(kindOf(t, "`code`")).toBe("string");
    expect(kindOf(t, "https://x.y")).toBe("attr");
    expect(kindOf(t, "var")).toBe("keyword");
    expect(kindOf(t, "1")).toBe("number");
    expect(joined(t)).toBe(code);
  });
  it("finishes a 200 KB single-line token in linear time", () => {
    const line = "x".repeat(199_000) + " = 1";
    const started = performance.now();
    for (const lang of ["go", "js", "shell", "yaml", "markup", "markdown", "css"]) tokenize(line, lang);
    expect(performance.now() - started).toBeLessThan(400);
  });
});

describe("highlightLines", () => {
  it("cuts the tokens at newlines, a block comment keeping its kind on every line", () => {
    const lines = highlightLines("a /* one\ntwo */ b\n", "go");
    expect(lines).toHaveLength(3);
    expect(lines[0]!.map((t) => t.kind)).toEqual(["plain", "comment"]);
    expect(lines[1]!.map((t) => t.kind)).toEqual(["comment", "plain"]);
    expect(lines[2]).toEqual([]);
    expect(lines.map((l) => l.map((t) => t.text).join("")).join("\n")).toBe("a /* one\ntwo */ b\n");
  });
  it("gives one plain line for a language it does not know", () => {
    expect(highlightLines("x\ny", "")).toEqual([[{ kind: "plain", text: "x" }], [{ kind: "plain", text: "y" }]]);
  });
});

describe("splitTokens", () => {
  const tokens: Token[] = [
    { kind: "keyword", text: "return" },
    { kind: "plain", text: " " },
    { kind: "function", text: "f" },
    { kind: "punct", text: "(" },
    { kind: "number", text: "42" },
    { kind: "punct", text: ")" },
  ];
  it("cuts whole tokens and parts of tokens at the offsets", () => {
    const { before, inside, after } = splitTokens(tokens, 3, 10);
    expect(before.map((t) => t.text).join("")).toBe("ret");
    expect(inside.map((t) => t.text).join("")).toBe("urn f(4");
    expect(after.map((t) => t.text).join("")).toBe("2)");
    expect(inside.map((t) => t.kind)).toEqual(["keyword", "plain", "function", "punct", "number"]);
  });
  it("leaves everything before or after an empty range", () => {
    expect(splitTokens(tokens, 0, 0).before).toEqual([]);
    expect(splitTokens(tokens, 0, 0).after).toEqual(tokens);
    expect(splitTokens(tokens, 12, 12).before).toEqual(tokens);
  });
});

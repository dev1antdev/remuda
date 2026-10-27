#!/usr/bin/env node
// strings.mjs: the page's own words outside t(…), for whoever translates. A scanner over the source, not a parser:
// JSX text (on a tag's line or on a line of its own), the attributes a person reads (aria-label, title, placeholder,
// alt, tip), the props that carry words (label, hint, title, text, what, …), a string in a JSX brace or a ternary, the
// first argument of notify / ask. It runs from web/ and needs nothing installed.
//
//   node scripts/strings.mjs            what is still outside t(): file:line: kind: text   (the count on stderr)
//   node scripts/strings.mjs --keys     every key inside t() / tn(), one per line
//   node scripts/strings.mjs --wrap     wrap the plain cases in t(…) in place and add the import; the mixed ones
//                                       (text beside an expression, a template with ${}) are listed for a hand
import { readdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const root = new URL("../src/", import.meta.url).pathname;
const mode = process.argv.includes("--keys") ? "keys" : process.argv.includes("--wrap") ? "wrap" : "list";
const where = process.argv.includes("--where"); // with --keys: the file before each key

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) yield* walk(path);
    else if (/\.tsx?$/.test(name) && !/\.test\.tsx?$/.test(name) && !/\/lib\/i18n/.test(path)) yield path;
  }
}

// words says whether a string is something a person reads: a run of letters with a space, a sentence mark or a capital
const words = (s) => {
  const v = s.trim();
  if (/&&|\|\||=>|[()[\]]|===|!==|\?\s*$|^:|\.[a-z]+\b|\breturn\b/.test(v)) return false; // code, not words
  if (/^[a-z][\w-]*(\s+[a-z][\w-]*)*$/.test(v) && /[-_]/.test(v)) return false; // class names
  return /[A-Za-z]{2,}/.test(v) && (/\s/.test(v) || /[.!?…:]$/.test(v) || /^[A-Z][a-z]+$/.test(v));
};
// inClass says the string sits in a className expression
const inClass = (line, at) => /className=\{[^}]*$/.test(line.slice(0, at));
const KEYWORDS = /^(return|const|let|var|export|import|if|else|for|while|case|default|break|continue|throw|type|interface|function|async|await|yield|new|delete|typeof|void|in|of|switch|try|catch|finally|do)\b/;
const ATTRS = "aria-label|title|placeholder|alt|tip|hint|submitLabel|text|label|empty|description|what";
const PROPS = "label|hint|title|text|what|placeholder|detail|description|body|ok|tip|sub|empty";
const CALLS = "notify|ask|setError|onError|fail";
const q = (s) => JSON.stringify(s);

// wrapText is a JSX text node's replacement: its words in t(), a space kept on the side that had one
function wrapText(raw) {
  const lead = /^\s/.test(raw) ? " " : "";
  const trail = /\s$/.test(raw) ? " " : "";
  return `${lead}{t(${q(decode(raw.trim()))})}${trail}`;
}
// typeish says a >…< run is a type's angle brackets, not a tag's: after an arrow, with a type in it, or a statement
const typeish = (line, at, text) => line[at - 1] === "=" || /;/.test(text) || /[:,]\s*[A-Z][\w$]*\s*$/.test(text) || /\b(ReturnType|Partial|Promise|Record|Omit|Pick|Readonly|Required|Parameters|Awaited|Map|Set|Array)\b/.test(text);
const decode = (s) => s.replace(/&amp;/g, "&").replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&nbsp;/g, "\u00a0").replace(/&quot;/g, '"').replace(/&#39;|&apos;/g, "'");

const keys = new Set();
const left = [];
let wrapped = 0;

for (const path of walk(root)) {
  const rel = path.slice(root.length);
  const tsx = path.endsWith(".tsx");
  const src = readFileSync(path, "utf8");
  for (const m of src.matchAll(/(?<![\w.$])tr?\(\s*(["'])((?:\\.|(?!\1)[^\\])*)\1/g)) keys.add(where ? `${rel}\t${m[2]}` : m[2]);
  for (const m of src.matchAll(/\btn\(\s*[^,]+,\s*"((?:\\.|[^"\\])*)",\s*"((?:\\.|[^"\\])*)"/g)) keys.add(where ? `${rel}\t${m[2]}` : m[2]);
  if (mode === "keys") continue;

  const lines = src.split("\n");
  let changed = false;
  let inComment = false;
  let inTemplate = false;
  const note = (i, kind, text) => left.push(`${rel}:${i + 1}: ${kind}: ${text.trim()}`);
  const prevCode = (i) => {
    for (let j = i - 1; j >= 0; j--) if (lines[j].trim()) return lines[j].trim();
    return "";
  };
  const nextCode = (i) => {
    for (let j = i + 1; j < lines.length; j++) if (lines[j].trim()) return lines[j].trim();
    return "";
  };
  for (let i = 0; i < lines.length; i++) {
    let line = lines[i];
    const trimmed = line.trim();
    if (inComment) {
      if (trimmed.includes("*/")) inComment = false;
      continue;
    }
    if (trimmed.startsWith("//") || trimmed.startsWith("*")) continue;
    if (trimmed.startsWith("/*")) {
      if (!trimmed.includes("*/")) inComment = true;
      continue;
    }
    const ticks = (line.match(/(?<!\\)`/g) || []).length;
    if (inTemplate) {
      if (ticks % 2 === 1) inTemplate = false;
      continue;
    }
    if (ticks % 2 === 1) inTemplate = true;
    // the mixed cases are for a hand: text beside an expression in one JSX node, a template with ${} and words
    if (tsx) for (const m of line.matchAll(/>([^<>]*?)</g)) if (/\{/.test(m[1]) && /[A-Za-z]{2,}/.test(m[1].replace(/\{[^}]*\}/g, "")) && !/\{t\(/.test(m[1])) note(i, "mixed", m[1]);
    for (const m of line.matchAll(/`((?:\\.|[^`\\])*)`/g)) if (/\$\{/.test(m[1]) && words(m[1].replace(/\$\{[^}]*\}/g, "x"))) note(i, "template", m[1]);

    const before = line;
    if (tsx) {
      // JSX text on a tag's line
      line = line.replace(/>([^<>{}]+)</g, (whole, text, at) => (words(text) && !typeish(line, at, text) ? (note(i, "text", text), `>${wrapText(text)}<`) : whole));
      // JSX text on a line of its own: between a tag line and the next tag or brace
      if (trimmed && !/[<>{}=;()[\]`"]/.test(trimmed.replace(/&\w+;/g, "&")) && words(trimmed) && !KEYWORDS.test(trimmed) && /[>}]$/.test(prevCode(i)) && /^[<{)]/.test(nextCode(i))) {
        note(i, "text", trimmed);
        line = line.replace(trimmed, `{t(${q(decode(trimmed))})}`);
      }
      // JSX text after the last tag on a line (`<Icon … /> Reject`), and before the first tag on a line that opens with text
      line = line.replace(/(\/>|<\/[\w.]+>)(\s*)([^<>{}=;\n]+)$/, (whole, tag, sp, text) => (words(text) && !KEYWORDS.test(text.trim()) ? (note(i, "text", text), `${tag}${sp}{t(${q(decode(text.trim()))})}`) : whole));
      line = line.replace(/^(\s*)([A-Za-z][^<>{}=;]*?)(\s*)(<[A-Za-z/])/, (whole, lead, text, sp, tag) => (words(text) && !KEYWORDS.test(text) && /[>}]$/.test(prevCode(i)) ? (note(i, "text", text), `${lead}{t(${q(decode(text.trim()))})}${sp}${tag}`) : whole));
      line = line.replace(new RegExp(`\\b(${ATTRS})="([^"]*)"`, "g"), (whole, attr, text) => (words(text) ? (note(i, attr, text), `${attr}={t(${q(decode(text))})}`) : whole));
      line = line.replace(/\{\s*"([^"]*)"\s*\}/g, (whole, text) => (words(text) ? (note(i, "brace", text), `{t(${q(text)})}`) : whole));
      line = line.replace(/(\?\?|\?|:)(\s*)"([^"]*)"(?=\s*[:})\],;]|\s*$)/g, (whole, op, sp, text, at) => (words(text) && !inClass(line, at) ? (note(i, "ternary", text), `${op}${sp}t(${q(text)})`) : whole));
    }
    line = line.replace(new RegExp(`\\b(${PROPS})(\\s*:\\s*)"([^"]*)"`, "g"), (whole, prop, sep, text) => (words(text) ? (note(i, prop, text), `${prop}${sep}t(${q(text)})`) : whole));
    line = line.replace(new RegExp(`\\b(${CALLS})\\((\\s*)"([^"]*)"`, "g"), (whole, call, sp, text) => (words(text) ? (note(i, call, text), `${call}(${sp}t(${q(text)})`) : whole));
    // the palette's command(id, title, …) and ask(text, label, …): the second argument is the words
    line = line.replace(/\b(command|ask)\(([^,()]+,\s*)"([^"]*)"/g, (whole, call, head, text) => (words(text) ? (note(i, call, text), `${call}(${head}t(${q(text)})`) : whole));
    if (line !== before) {
      changed = true;
      lines[i] = line;
    }
  }
  if (mode === "wrap" && changed) {
    let out = lines.join("\n");
    if (!/from "@\/lib\/i18n"/.test(out)) {
      const at = lastImportEnd(lines);
      lines.splice(at, 0, 'import { t } from "@/lib/i18n";');
      out = lines.join("\n");
    }
    writeFileSync(path, out);
    wrapped++;
  }
}

// lastImportEnd is the line after the file's imports, where one more import goes
function lastImportEnd(lines) {
  let at = 0;
  for (let i = 0; i < lines.length; i++) {
    if (/^import\b/.test(lines[i])) {
      let j = i;
      while (j < lines.length && !/;\s*$/.test(lines[j])) j++;
      at = j + 1;
      i = j;
    } else if (lines[i].trim() && !lines[i].trim().startsWith("//") && at > 0) break;
  }
  return at;
}

if (mode === "keys") {
  for (const k of [...keys].sort()) console.log(k);
} else {
  for (const l of left) console.log(l);
  const mixed = left.filter((l) => /: (mixed|template): /.test(l)).length;
  console.error(`${left.length - mixed} strings ${mode === "wrap" ? "wrapped" : "outside t()"}, ${mixed} mixed or templates for a hand, ${keys.size} keys inside t() before this run${mode === "wrap" ? `, ${wrapped} files written` : ""}`);
}

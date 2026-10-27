#!/usr/bin/env node
// strings.mjs: what the page still says in a component's own words — JSX text and the named attributes (aria-label,
// title, placeholder, label, hint, text) outside t(…) — so the person translating sees what is left.
// Usage: node scripts/strings.mjs [--keys]   (--keys lists the t("…") keys instead, one per line)
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

const root = new URL("../src/", import.meta.url).pathname;
const listKeys = process.argv.includes("--keys");

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) yield* walk(path);
    else if (/\.tsx?$/.test(name) && !/\.test\.tsx?$/.test(name)) yield path;
  }
}

// words says whether a string is something a person reads: letters with a space or a sentence mark, not an identifier
const words = (s) => /[A-Za-z]{2,}/.test(s) && (/\s/.test(s.trim()) || /[.!?…:]$/.test(s.trim()) || /^[A-Z][a-z]+$/.test(s.trim()));
const ATTRS = ["aria-label", "title", "placeholder"];
const PROPS = ["label", "hint", "text", "title", "placeholder", "detail", "description"];

const keys = new Set();
const left = [];
for (const path of walk(root)) {
  const src = readFileSync(path, "utf8");
  const rel = path.slice(root.length);
  for (const m of src.matchAll(/\bt\(\s*(["'`])((?:\\.|(?!\1)[^\\])*)\1/g)) keys.add(m[2]);
  // JSX text: between a closing > and an opening <, on one line, no braces
  for (const m of src.matchAll(/>([^<>{}\n]+)</g)) {
    const text = m[1].trim();
    if (text && words(text)) left.push([rel, lineOf(src, m.index), `text: ${text}`]);
  }
  for (const attr of ATTRS) {
    for (const m of src.matchAll(new RegExp(`\\b${attr}="([^"]+)"`, "g"))) if (words(m[1])) left.push([rel, lineOf(src, m.index), `${attr}: ${m[1]}`]);
  }
  for (const prop of PROPS) {
    for (const m of src.matchAll(new RegExp(`\\b${prop}:\\s*"([^"]+)"`, "g"))) if (words(m[1])) left.push([rel, lineOf(src, m.index), `${prop}: ${m[1]}`]);
  }
}

function lineOf(src, index) {
  return src.slice(0, index).split("\n").length;
}

if (listKeys) {
  for (const k of [...keys].sort()) console.log(k);
} else {
  for (const [rel, line, what] of left) console.log(`${rel}:${line}: ${what}`);
  console.error(`${left.length} strings outside t(), ${keys.size} keys inside`);
}

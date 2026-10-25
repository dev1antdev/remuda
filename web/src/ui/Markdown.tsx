import { Fragment, useEffect, useMemo, useRef, useState, type MouseEvent, type ReactNode } from "react";
import { findPaths } from "@/lib/paths";
import { highlightLines, langOf } from "@/lib/highlight";
import type { OpenFile } from "@/features/files/target";
import Icon from "@/ui/icons";
import { tokenNodes } from "@/ui/Tokens";
import { copyText, openUrl } from "@/lib/host";

// links open in a new tab — never by navigating this page
const link = (url: string) => (e: MouseEvent) => {
  e.preventDefault();
  openUrl(url);
};
const anchor = (url: string, children: ReactNode, key: string) => (
  <a key={key} href={url} target="_blank" rel="noopener noreferrer" onClick={link(url)}>
    {children}
  </a>
);

// A small Markdown renderer for agent answers: React nodes directly, no HTML
// string, nothing to sanitize. Headings, paragraphs, nested lists (task lists
// too), fenced code, blockquotes, tables, rules, inline marks, file paths as buttons.

type Block =
  | { t: "p"; lines: string[] }
  | { t: "h"; level: number; text: string }
  | { t: "code"; lang: string; code: string }
  | { t: "quote"; blocks: Block[] }
  | { t: "list"; ordered: boolean; start: number; items: ListItem[] }
  | { t: "table"; head: string[]; rows: string[][] }
  | { t: "hr" };

const FENCE = /^\s{0,3}(`{3,}|~{3,})\s*(\S*)\s*$/;
const HEADING = /^\s{0,3}(#{1,6})\s+(.*?)\s*#*\s*$/;
const HR = /^\s{0,3}([-*_])(\s*\1){2,}\s*$/;
const LIST = /^(\s*)([-*+]|\d{1,3}[.)])\s+(.*)$/;
const QUOTE = /^\s{0,3}>\s?(.*)$/;
const TABLE_SEP = /^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$/;

// A list item: its blocks, and for a task item ("[ ] …" / "[x] …") its state.
type ListItem = { blocks: Block[]; checked: boolean | null };

// Deeper than this, quotes and lists read as text: a line of three thousand ">" or "- " marks would otherwise recurse past the stack.
const MAX_NESTING = 16;
// A line this long is a paste, not prose: no marks, no links — the inline regex costs its square on a run of one character.
const LONG_LINE = 10_000;

export function parseBlocks(src: string, depth = 0): Block[] {
  const lines = src.replace(/\r\n?/g, "\n").split("\n");
  const out: Block[] = [];
  let i = 0;

  const para: string[] = [];
  const flushPara = () => {
    if (para.length) out.push({ t: "p", lines: para.splice(0) });
  };
  while (i < lines.length) {
    const line = lines[i]!;

    if (line.trim() === "") {
      flushPara();
      i++;
      continue;
    }

    const fence = line.match(FENCE);
    if (fence) {
      flushPara();
      const marker = fence[1]!;
      const lang = fence[2] ?? "";
      const code: string[] = [];
      i++;
      while (i < lines.length && !lines[i]!.trim().startsWith(marker)) code.push(lines[i++]!);
      i++; // closing fence (or EOF)
      out.push({ t: "code", lang, code: code.join("\n") });
      continue;
    }

    const h = line.match(HEADING);
    if (h) {
      flushPara();
      out.push({ t: "h", level: h[1]!.length, text: h[2]! });
      i++;
      continue;
    }

    if (HR.test(line)) {
      flushPara();
      out.push({ t: "hr" });
      i++;
      continue;
    }

    if (depth < MAX_NESTING && QUOTE.test(line)) {
      flushPara();
      const inner: string[] = [];
      while (i < lines.length && QUOTE.test(lines[i]!)) inner.push(lines[i++]!.match(QUOTE)![1]!);
      out.push({ t: "quote", blocks: parseBlocks(inner.join("\n"), depth + 1) });
      continue;
    }

    if (depth < MAX_NESTING && LIST.test(line)) {
      flushPara();
      const first = line.match(LIST)!;
      const indent = first[1]!.length;
      const ordered = /\d/.test(first[2]!);
      const start = ordered ? parseInt(first[2]!, 10) : 1;
      const items: ListItem[] = [];
      while (i < lines.length) {
        const m = lines[i]!.match(LIST);
        if (!m || m[1]!.length !== indent || /\d/.test(m[2]!) !== ordered) break;
        // the item's own text plus every following line indented deeper, blank lines before deeper lines included
        const body: string[] = [m[3]!];
        i++;
        while (i < lines.length) {
          const l = lines[i]!;
          if (l.trim() === "") {
            if (i + 1 < lines.length && leading(lines[i + 1]!) > indent) {
              body.push("");
              i++;
              continue;
            }
            break;
          }
          if (leading(l) > indent) {
            body.push(l.slice(Math.min(leading(l), indent + 2)));
            i++;
            continue;
          }
          break;
        }
        let checked: boolean | null = null;
        const task = body[0]!.match(/^\[([ xX])\]\s+/);
        if (task) {
          checked = task[1] !== " ";
          body[0] = body[0]!.slice(task[0].length);
        }
        items.push({ blocks: parseBlocks(body.join("\n"), depth + 1), checked });
      }
      out.push({ t: "list", ordered, start, items });
      continue;
    }

    if (line.includes("|") && i + 1 < lines.length && TABLE_SEP.test(lines[i + 1]!)) {
      flushPara();
      const head = cells(line);
      i += 2;
      const rows: string[][] = [];
      while (i < lines.length && lines[i]!.includes("|") && lines[i]!.trim() !== "") rows.push(cells(lines[i++]!));
      out.push({ t: "table", head, rows });
      continue;
    }

    para.push(line);
    i++;
  }
  flushPara();
  return out;
}

function leading(l: string): number {
  return l.length - l.trimStart().length;
}

function cells(line: string): string[] {
  let s = line.trim();
  if (s.startsWith("|")) s = s.slice(1);
  if (s.endsWith("|")) s = s.slice(0, -1);
  return s.split(/(?<!\\)\|/).map((c) => c.trim().replace(/\\\|/g, "|"));
}

// ---- inline ----

const INLINE = new RegExp(
  [
    "(`+)([\\s\\S]*?[^`])\\1(?!`)", // 1,2 code span
    "\\[([^\\]\\n]+)\\]\\((https?:\\/\\/[^\\s)]+)\\)", // 3,4 link
    "\\*\\*([^*\\n]+?)\\*\\*", // 5 bold
    "__([^_\\n]+?)__", // 6 bold
    "(?<![\\w*])\\*([^*\\n]+?)\\*(?![\\w*])", // 7 italic
    "(?<!\\w)_([^_\\n]+?)_(?!\\w)", // 8 italic
    "~~([^~\\n]+?)~~", // 9 strike
    "(https?:\\/\\/[^\\s<>\"'\\)\\]]+)", // 10 bare url
  ].join("|"),
  "g",
);

// renderInline; inside a link's text nothing else is clickable (onOpenFile null): an interactive element inside another is invalid markup
function renderInline(text: string, onOpenFile: OpenFile | null, key = "i"): ReactNode[] {
  if (text.length > LONG_LINE) return [<Fragment key={key}>{text}</Fragment>];
  const out: ReactNode[] = [];
  let at = 0;
  let n = 0;
  for (const m of text.matchAll(INLINE)) {
    const idx = m.index ?? 0;
    if (idx > at) out.push(...plain(text.slice(at, idx), onOpenFile, `${key}${n++}`));
    const k = `${key}${n++}`;
    if (m[2] !== undefined) out.push(<code key={k} className="md-code">{m[2]}</code>);
    else if (m[3] !== undefined) {
      if (onOpenFile) out.push(anchor(m[4]!, renderInline(m[3], null, k), k));
      else out.push(<Fragment key={k}>{m[0]}</Fragment>);
    } else if (m[5] !== undefined || m[6] !== undefined)
      out.push(<strong key={k}>{renderInline((m[5] ?? m[6])!, onOpenFile, k)}</strong>);
    else if (m[7] !== undefined || m[8] !== undefined)
      out.push(<em key={k}>{renderInline((m[7] ?? m[8])!, onOpenFile, k)}</em>);
    else if (m[9] !== undefined) out.push(<s key={k}>{renderInline(m[9], onOpenFile, k)}</s>);
    else if (m[10] !== undefined) {
      const url = m[10].replace(/[.,;:!?]+$/, "");
      out.push(onOpenFile ? anchor(url, url, k) : <Fragment key={k}>{url}</Fragment>);
      if (url.length < m[10].length) out.push(m[10].slice(url.length));
    }
    at = idx + m[0].length;
  }
  if (at < text.length) out.push(...plain(text.slice(at), onOpenFile, `${key}${n++}`));
  return out;
}

// plain text: file paths become buttons (none inside a link's text)
function plain(text: string, onOpenFile: OpenFile | null, key: string): ReactNode[] {
  const matches = onOpenFile ? findPaths(text) : [];
  if (matches.length === 0) return [<Fragment key={key}>{text}</Fragment>];
  const out: ReactNode[] = [];
  let at = 0;
  matches.forEach((m, i) => {
    if (m.start > at) out.push(<Fragment key={`${key}t${i}`}>{text.slice(at, m.start)}</Fragment>);
    out.push(
      <button key={`${key}p${i}`} type="button" className="path-link" onClick={() => onOpenFile!({ path: m.path, line: m.line })}>
        {m.text}
      </button>,
    );
    at = m.end;
  });
  if (at < text.length) out.push(<Fragment key={`${key}tail`}>{text.slice(at)}</Fragment>);
  return out;
}

// ---- blocks → React ----

export function Markdown({ text, onOpenFile }: { text: string; onOpenFile: OpenFile }) {
  const blocks = useMemo(() => parseBlocks(text), [text]);
  return <div className="md">{renderBlocks(blocks, onOpenFile, "b")}</div>;
}

function renderBlocks(blocks: Block[], onOpenFile: OpenFile, key: string): ReactNode[] {
  return blocks.map((b, i) => {
    const k = `${key}${i}`;
    switch (b.t) {
      case "p":
        return (
          <p key={k}>
            {b.lines.map((l, j) => (
              <Fragment key={j}>
                {j > 0 && <br />}
                {renderInline(l, onOpenFile, `${k}l${j}`)}
              </Fragment>
            ))}
          </p>
        );
      case "h": {
        const Tag = `h${Math.min(6, b.level + 2)}` as "h3" | "h4" | "h5" | "h6";
        return <Tag key={k} className={`md-h md-h${b.level}`}>{renderInline(b.text, onOpenFile, k)}</Tag>;
      }
      case "code":
        return <CodeBlock key={k} lang={b.lang} code={b.code} />;
      case "quote":
        return <blockquote key={k}>{renderBlocks(b.blocks, onOpenFile, k)}</blockquote>;
      case "hr":
        return <hr key={k} />;
      case "table":
        return (
          <div key={k} className="md-table-wrap">
            <table>
              <thead>
                <tr>
                  {b.head.map((c, j) => (
                    <th key={j}>{renderInline(c, onOpenFile, `${k}h${j}`)}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {b.rows.map((r, ri) => (
                  <tr key={ri}>
                    {b.head.map((_, j) => (
                      <td key={j}>{renderInline(r[j] ?? "", onOpenFile, `${k}r${ri}c${j}`)}</td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        );
      case "list": {
        const Tag = b.ordered ? "ol" : "ul";
        return (
          <Tag key={k} start={b.ordered ? b.start : undefined}>
            {b.items.map(({ blocks, checked }, j) => (
              <li key={j} className={checked === null ? undefined : checked ? "task done" : "task"}>
                {checked !== null && <input type="checkbox" checked={checked} disabled />}
                {renderBlocks(blocks, onOpenFile, `${k}i${j}`)}
              </li>
            ))}
          </Tag>
        );
      }
    }
  });
}

// CodeBlock is a fence: its language in the bar, a copy button, the first 40 lines until unfolded, the code coloured
// by lib/highlight when the language is known (a fence without one, or a dump past the cap, stays one plain string)
function CodeBlock({ lang, code }: { lang: string; code: string }) {
  const lines = code.split("\n");
  const long = lines.length > 40;
  const [open, setOpen] = useState(!long);
  const [copied, setCopied] = useState(false);
  const copiedTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(copiedTimer.current), []);
  const language = langOf(lang);
  const coloured = useMemo(() => (language ? highlightLines(code, language) : null), [code, language]);
  const shown = open ? code : lines.slice(0, 40).join("\n");
  const shownLines = coloured && (open ? coloured : coloured.slice(0, 40));
  return (
    <div className="md-pre">
      <div className="md-pre-bar">
        <span>{lang || "code"}</span>
        <button
          type="button"
          className="linkish"
          onClick={() => {
            void copyText(code).then((ok) => {
              if (!ok) return;
              setCopied(true);
              clearTimeout(copiedTimer.current);
              copiedTimer.current = setTimeout(() => setCopied(false), 1200);
            });
          }}
        >
          <Icon name={copied ? "check" : "copy"} size={12} /> {copied ? "copied" : "copy"}
        </button>
      </div>
      <pre>
        <code>
          {shownLines
            ? shownLines.map((tokens, i) => (
                <Fragment key={i}>
                  {i > 0 && "\n"}
                  {tokenNodes(tokens)}
                </Fragment>
              ))
            : shown}
        </code>
      </pre>
      {long && (
        <button type="button" className="linkish md-more" onClick={() => setOpen((o) => !o)}>
          {open ? "Show less" : `Show all ${lines.length} lines`}
        </button>
      )}
    </div>
  );
}

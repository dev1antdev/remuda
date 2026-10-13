// Descriptions are HTML (paragraphs; older ones may carry more): rendered through an allow-list into
// React nodes, never innerHTML, and edited as plain text that goes back as paragraphs.

import { createElement, useMemo, type ReactNode } from "react";
import { openUrl } from "@/lib/host";

const BLOCK = new Set(["p", "div", "li", "h1", "h2", "h3", "h4", "pre", "blockquote", "tr"]);

export function htmlToText(html: string): string {
  const doc = new DOMParser().parseFromString(html, "text/html");
  let out = "";
  const walk = (n: Node) => {
    if (n.nodeType === Node.TEXT_NODE) {
      out += n.textContent;
      return;
    }
    if (n.nodeType !== Node.ELEMENT_NODE) return;
    const tag = (n as Element).tagName.toLowerCase();
    if (tag === "br") {
      out += "\n";
      return;
    }
    if (tag === "li") out += "- ";
    n.childNodes.forEach(walk);
    if (BLOCK.has(tag)) out += "\n";
  };
  doc.body.childNodes.forEach(walk);
  return out.replace(/\n{3,}/g, "\n\n").trim();
}

export function textToHtml(text: string): string {
  const esc = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  return text
    .split(/\n{2,}/)
    .map((para) => `<p>${esc(para).replace(/\n/g, "<br>")}</p>`)
    .join("");
}

const ALLOWED = new Set(["p", "br", "strong", "b", "em", "i", "u", "s", "a", "ul", "ol", "li", "code", "pre", "h1", "h2", "h3", "h4", "blockquote", "hr", "input", "span", "div", "label"]);

export function Html({ html }: { html: string }) {
  const nodes = useMemo(() => {
    const doc = new DOMParser().parseFromString(html, "text/html");
    let k = 0;
    const render = (n: Node): ReactNode => {
      if (n.nodeType === Node.TEXT_NODE) return n.textContent;
      if (n.nodeType !== Node.ELEMENT_NODE) return null;
      const el = n as Element;
      const tag = el.tagName.toLowerCase();
      const kids = Array.from(el.childNodes).map(render);
      const key = k++;
      if (!ALLOWED.has(tag)) return <span key={key}>{kids}</span>;
      if (tag === "a") {
        const href = el.getAttribute("href") ?? "";
        if (!/^https?:\/\//.test(href)) return <span key={key}>{kids}</span>;
        return (
          <a
            key={key}
            href={href}
            target="_blank"
            rel="noopener noreferrer"
            onClick={(e) => {
              e.preventDefault();
              openUrl(href);
            }}
          >
            {kids}
          </a>
        );
      }
      if (tag === "input") return <input key={key} type="checkbox" checked={el.hasAttribute("checked")} readOnly />;
      if (tag === "br" || tag === "hr") return createElement(tag, { key });
      return createElement(tag, { key }, ...kids);
    };
    return Array.from(doc.body.childNodes).map(render);
  }, [html]);
  return <>{nodes}</>;
}

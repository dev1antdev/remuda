// Mermaid draws a Mermaid source as its diagram: the library loaded only when a diagram is on screen (its own chunks),
// strict security, the page's flavour and accent, drawn again when they change. The SVG mermaid answers is parsed as
// XML and its nodes adopted into a box React leaves empty — never innerHTML — after anything that could run is
// stripped. A source mermaid cannot read is handed back through onError, for the caller to show as code.

import { memo, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { errMsg } from "@/lib/text";
import { t } from "@/lib/i18n";
import { lookOf, mermaidConfig } from "./mermaidLook";

type MermaidAPI = (typeof import("mermaid"))["default"];

let library: Promise<MermaidAPI> | null = null;
const loadMermaid = () => (library ??= import("mermaid").then((m) => m.default));

let configured = "";
let drawn = 0;
let queue: Promise<unknown> = Promise.resolve();

// draw renders one source after the ones before it: mermaid keeps one configuration, so a drawing in another
// flavour must not start between another's initialize and render
function draw(source: string, look: string, font: string): Promise<{ svg: string } | { error: string }> {
  const run = async () => {
    const mermaid = await loadMermaid();
    const key = `${look}|${font}`;
    if (configured !== key) {
      const { flavor, accent } = lookOf(look);
      mermaid.initialize(mermaidConfig(flavor, accent, font));
      configured = key;
    }
    try {
      await mermaid.parse(source);
    } catch (e) {
      return { error: errMsg(e) };
    }
    const { svg } = await mermaid.render(`remuda-mermaid-${++drawn}`, source);
    return { svg };
  };
  const next = queue.then(run, run);
  queue = next.catch(() => undefined);
  return next;
}

// rootLook is "flavor|accent" off the page's root, which app/theme dresses
const rootLook = () => `${document.documentElement.dataset.flavor ?? ""}|${document.documentElement.dataset.accent ?? ""}`;
function watchRootLook(changed: () => void) {
  const watcher = new MutationObserver(changed);
  watcher.observe(document.documentElement, { attributes: true, attributeFilter: ["data-flavor", "data-accent"] });
  return () => watcher.disconnect();
}

const SVG_NS = "http://www.w3.org/2000/svg";

// adopt puts the drawn SVG into the box: parsed as XML, scripts, handlers and script links removed, at the size it was
// drawn (mermaid would shrink it to the column; a wide one scrolls instead), the nodes imported
function adopt(svg: string, box: HTMLElement): boolean {
  const doc = new DOMParser().parseFromString(svg, "image/svg+xml");
  const root = doc.documentElement;
  if (root.namespaceURI !== SVG_NS || doc.getElementsByTagName("parsererror").length > 0) return false;
  for (const script of Array.from(root.getElementsByTagName("script"))) script.remove();
  for (const el of [root, ...Array.from(root.getElementsByTagName("*"))]) {
    for (const attr of Array.from(el.attributes)) {
      const name = attr.name.toLowerCase();
      const value = attr.value.trim().toLowerCase();
      if (name.startsWith("on") || ((name === "href" || name === "xlink:href") && value.startsWith("javascript:"))) el.removeAttribute(attr.name);
    }
  }
  const view = (root.getAttribute("viewBox") ?? "").split(/[\s,]+/).map(Number);
  if (view.length === 4 && view[2]! > 0 && view[3]! > 0) {
    root.setAttribute("width", String(Math.ceil(view[2]!)));
    root.setAttribute("height", String(Math.ceil(view[3]!)));
    root.setAttribute("style", (root.getAttribute("style") ?? "").replace(/max-width:[^;]*;?/i, ""));
  }
  box.replaceChildren(box.ownerDocument.importNode(root, true));
  return true;
}

export const Mermaid = memo(function Mermaid({ source, onError }: { source: string; onError: (error: string) => void }) {
  const look = useSyncExternalStore(watchRootLook, rootLook);
  const box = useRef<HTMLDivElement>(null);
  const [drawing, setDrawing] = useState(true);
  const failed = useRef(onError);
  failed.current = onError;
  useEffect(() => {
    let live = true;
    setDrawing(true);
    const font = getComputedStyle(document.body).fontFamily;
    void draw(source, look, font).then(
      (out) => {
        if (!live || !box.current) return;
        if ("error" in out) return failed.current(out.error);
        if (!adopt(out.svg, box.current)) return failed.current(t("The diagram could not be drawn"));
        setDrawing(false);
      },
      (e) => live && failed.current(errMsg(e)),
    );
    return () => {
      live = false;
    };
  }, [source, look]);
  return (
    <div className="mermaid-diagram">
      {drawing && <div className="mermaid-drawing">{t("Drawing the diagram…")}</div>}
      <div ref={box} className="mermaid-svg" />
    </div>
  );
});

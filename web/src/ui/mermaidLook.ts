// What makes a fence or a file a Mermaid diagram, which view a file opens in, and the configuration a diagram is drawn
// with: strict security, labels as SVG text (no HTML in a foreignObject, so the answer parses as XML), the colours of
// the page's theme and accent. The pure part of ui/Mermaid.tsx.

import { ACCENTS, DEFAULT_DARK, FLAVORS, accentOf, isFlavor, themeOf, type Accent, type Flavor } from "@/lib/theme";

// isMermaid says whether a fence's info string names a Mermaid diagram: its first word mermaid (or mmd), any case
export const isMermaid = (info: string): boolean => /^(mermaid|mmd)$/i.test(info.trim().split(/\s+/)[0] ?? "");

// isMermaidFile and isMarkdownFile are the files the viewer can show drawn as well as as text
export const isMermaidFile = (path: string): boolean => /\.(mmd|mermaid)$/i.test(path);
export const isMarkdownFile = (path: string): boolean => /\.(md|markdown|mdown|mkd)$/i.test(path);

// previewOf is how a text file can be shown drawn: as Markdown, as a diagram, or not at all
export function previewOf(path: string): "markdown" | "mermaid" | null {
  if (isMermaidFile(path)) return "mermaid";
  if (isMarkdownFile(path)) return "markdown";
  return null;
}

// opensDrawn says whether the viewer starts on the drawn view: a file that has one, asked for neither its changes nor a line
export const opensDrawn = (path: string, line?: number, prefer?: "file" | "changes"): boolean => previewOf(path) !== null && !line && prefer !== "changes";

// what the URL parser drops from a URL before it reads the scheme, so "java\tscript:" runs as javascript: does
const URL_NOISE = /[\u0000-\u0020]/g;
const SCRIPT_URL = /(?:javascript|vbscript):/;

// unsafeAttribute says whether an attribute of a drawn diagram could run something: an event handler, or a value naming a
// script URL however it is spelled — a link's, or the one an animation would set a link to
export function unsafeAttribute(name: string, value: string): boolean {
  if (name.toLowerCase().startsWith("on")) return true;
  return SCRIPT_URL.test(value.replace(URL_NOISE, "").toLowerCase());
}

// flavorOf reads the root's data-flavor, Frappé when it names none of the twenty
export function flavorOf(value: string | undefined): Flavor {
  return isFlavor(value) ? value : DEFAULT_DARK;
}

// lookOf turns the root's "flavor|accent" into the two, each kept to a known value
export function lookOf(look: string): { flavor: Flavor; accent: Accent } {
  const [flavor, accent] = look.split("|");
  return { flavor: flavorOf(flavor), accent: accentOf(accent) };
}

// mermaidConfig is the configuration a diagram is drawn with in a theme, with the accent on the shapes' edges
export function mermaidConfig(flavor: Flavor, accent: Accent, fontFamily: string) {
  const c = FLAVORS[flavor];
  const pies = Object.fromEntries(ACCENTS.filter((a) => a !== "rosewater" && a !== "flamingo").map((a, i) => [`pie${i + 1}`, c[a]]));
  const themeVariables: Record<string, string | boolean> = {
    darkMode: themeOf(flavor) === "dark",
    fontFamily,
    fontSize: "14px",
    background: c.base,
    primaryColor: c.surface0,
    primaryTextColor: c.text,
    primaryBorderColor: c[accent],
    secondaryColor: c.mantle,
    secondaryTextColor: c.text,
    secondaryBorderColor: c.overlay0,
    tertiaryColor: c.surface1,
    tertiaryTextColor: c.text,
    tertiaryBorderColor: c.overlay0,
    mainBkg: c.surface0,
    nodeBorder: c[accent],
    nodeTextColor: c.text,
    textColor: c.text,
    titleColor: c.text,
    lineColor: c.overlay2,
    edgeLabelBackground: c.base,
    clusterBkg: c.mantle,
    clusterBorder: c.surface2,
    noteBkgColor: c.surface1,
    noteTextColor: c.text,
    noteBorderColor: c.overlay0,
    actorBkg: c.surface0,
    actorBorder: c[accent],
    actorTextColor: c.text,
    actorLineColor: c.overlay1,
    signalColor: c.text,
    signalTextColor: c.text,
    labelBoxBkgColor: c.surface0,
    labelBoxBorderColor: c.overlay0,
    labelTextColor: c.text,
    loopTextColor: c.text,
    activationBkgColor: c.surface1,
    activationBorderColor: c.overlay1,
    sequenceNumberColor: c.base,
    errorBkgColor: c.red,
    errorTextColor: c.base,
    pieStrokeColor: c.base,
    pieOuterStrokeColor: c.surface2,
    pieTitleTextColor: c.text,
    pieLegendTextColor: c.text,
    ...pies,
  };
  return { startOnLoad: false, securityLevel: "strict" as const, theme: "base" as const, htmlLabels: false, flowchart: { htmlLabels: false }, fontFamily, themeVariables };
}

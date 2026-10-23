// The placeholders a quick prompt may carry — {dir}, {branch}, {file} — and what they become when the prompt is picked.
// One the page cannot fill stays as written, so the person sees it rather than a hole.

export type QuickVars = { dir?: string; branch?: string; file?: string };
export type QuickVar = keyof QuickVars;

const PLACEHOLDER = /\{(dir|branch|file)\}/g;

// needs lists the placeholders a text carries, so the branch is asked for only when a prompt wants it
export function needs(text: string): Set<QuickVar> {
  const out = new Set<QuickVar>();
  for (const m of text.matchAll(PLACEHOLDER)) out.add(m[1] as QuickVar);
  return out;
}

export function expand(text: string, vars: QuickVars): string {
  return text.replace(PLACEHOLDER, (whole, name: QuickVar) => vars[name] || whole);
}

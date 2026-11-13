// The labels a row carries, after git-graph: a local branch with the remote of the same name riding on it (`main |
// origin`), a remote branch alone with its cloud, a tag, a detached HEAD; the one checked out first. A name is cut at 22
// characters with an ellipsis, a remote's name on a branch at 12.

import type { Commit, Ref } from "./api";

export const LABEL_MAX = 22;
const REMOTE_MAX = 12;

export type LabelPart = { kind: Ref["kind"]; name: string; text: string; icon: boolean };
export type Label = { kind: Ref["kind"]; head: boolean; parts: LabelPart[] };

// cut shortens a name to n characters, the last one an ellipsis
export function cut(name: string, n = LABEL_MAX): string {
  const chars = [...name];
  return chars.length > n ? chars.slice(0, n - 1).join("") + "…" : name;
}

// labelsOf is a commit's labels in the order they are drawn: what is checked out first, then branches, remote
// branches alone, tags
export function labelsOf(commit: Pick<Commit, "refs">, remotesShown = true): Label[] {
  const refs = commit.refs ?? [];
  const remotes = remotesShown ? refs.filter((r) => r.kind === "remote") : [];
  const paired = new Set<string>();
  const labels: Label[] = [];
  for (const ref of refs) {
    if (ref.kind === "head") labels.push({ kind: "head", head: true, parts: [{ kind: "head", name: "HEAD", text: "HEAD", icon: false }] });
    if (ref.kind !== "branch") continue;
    const parts: LabelPart[] = [{ kind: "branch", name: ref.name, text: cut(ref.name), icon: true }];
    for (const remote of remotes) {
      const slash = remote.name.indexOf("/");
      if (remote.name.slice(slash + 1) !== ref.name || paired.has(remote.name)) continue;
      paired.add(remote.name);
      parts.push({ kind: "remote", name: remote.name, text: cut(remote.name.slice(0, slash), REMOTE_MAX), icon: false });
    }
    labels.push({ kind: "branch", head: !!ref.head, parts });
  }
  for (const remote of remotes) if (!paired.has(remote.name)) labels.push({ kind: "remote", head: false, parts: [{ kind: "remote", name: remote.name, text: cut(remote.name), icon: true }] });
  for (const ref of refs) if (ref.kind === "tag") labels.push({ kind: "tag", head: false, parts: [{ kind: "tag", name: ref.name, text: cut(ref.name), icon: true }] });
  return labels.sort((a, b) => Number(b.head) - Number(a.head));
}

// isHead says the commit is where HEAD stands: a branch checked out on it, or a detached HEAD
export const isHead = (commit: Pick<Commit, "refs">): boolean => (commit.refs ?? []).some((r) => r.head || r.kind === "head");

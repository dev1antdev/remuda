// treeBatch coalesces the folders a tick reads into one request: a tree with a dozen folders open read them one at a
// time on the machine, each its own session of the ssh master. Everything asked in the same turn goes as one call, and
// each folder keeps its own answer.

import { fsTrees } from "../api";
import type { TreeListing } from "./tree";

type Waiting = { resolve: (listing: TreeListing) => void; reject: (why: unknown) => void };
type Group = { machineId: string; dir: string; rels: Map<string, Waiting[]> };

const pending = new Map<string, Group>();
let queued = false;

export function treeBatch(machineId: string, dir: string, rel: string): Promise<TreeListing> {
  const key = `${machineId}|${dir}`;
  return new Promise((resolve, reject) => {
    const group = pending.get(key) ?? { machineId, dir, rels: new Map<string, Waiting[]>() };
    pending.set(key, group);
    const waiting = group.rels.get(rel) ?? [];
    waiting.push({ resolve, reject });
    group.rels.set(rel, waiting);
    if (!queued) {
      queued = true;
      queueMicrotask(() => void flush());
    }
  });
}

async function flush(): Promise<void> {
  queued = false;
  const groups = [...pending.values()];
  pending.clear();
  for (const group of groups) {
    try {
      const listings = await fsTrees(
        group.machineId,
        group.dir,
        [...group.rels.keys()],
      );
      for (const [rel, waiting] of group.rels) {
        const listing = listings[rel];
        for (const w of waiting) {
          if (listing) w.resolve(listing);
          else w.reject(new Error("this folder's answer is missing"));
        }
      }
    } catch (why) {
      for (const waiting of group.rels.values()) for (const w of waiting) w.reject(why);
    }
  }
}

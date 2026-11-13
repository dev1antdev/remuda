// The Repositories place's actions as they run: the rows being fetched, pulled or pushed, Fetch all's progress, the
// last failed fetch of each row, and the push the secret check holds back. Each action's outcome is a toast, and the
// row it ran on is read again in its answer.

import { useCallback, useRef, useState } from "react";
import { actions } from "@/app/store";
import { errMsg } from "@/lib/text";
import type { Listed, Repo } from "./api";
import { outcomeWords, ranWords, type FetchRun } from "./outcome";
import { FETCH_AT_ONCE, pooled } from "./pool";
import { fetchable, keyOf, replaced } from "./repos";
import { fetchRepo, syncRepo, type Outcome, type SyncAction } from "./syncApi";

// HeldPush is a push the secret check holds back, waiting for Push anyway
export type HeldPush = { machineId: string; repo: Repo; outcome: Outcome };

export function useRepoSync(mutate: (fn: (data: { machines: Listed[] } | null) => { machines: Listed[] } | null) => void, reload: () => void) {
  const [busy, setBusy] = useState<Set<string>>(new Set());
  const [failed, setFailed] = useState<Map<string, string>>(new Map());
  const [run, setRun] = useState<FetchRun | null>(null);
  const [held, setHeld] = useState<HeldPush | null>(null);
  const running = useRef(false);

  const mark = useCallback((key: string, on: boolean) => {
    setBusy((before) => {
      const next = new Set(before);
      if (on) next.add(key);
      else next.delete(key);
      return next;
    });
  }, []);
  const noteFetch = useCallback((key: string, why: string) => {
    setFailed((before) => {
      const next = new Map(before);
      if (why) next.set(key, why);
      else next.delete(key);
      return next;
    });
  }, []);
  const landed = useCallback(
    (machineId: string, outcome: Outcome) => {
      const repo = outcome.repo;
      if (repo) mutate((data) => (data ? { machines: replaced(data.machines, machineId, repo) ?? data.machines } : data));
    },
    [mutate],
  );

  // act runs one action on one row; anyway lets a held push past the secret check, after the person's confirm
  const act = useCallback(
    async (machineId: string, repo: Repo, action: SyncAction, anyway = false) => {
      const key = keyOf(machineId, repo.path);
      mark(key, true);
      try {
        const outcome = await syncRepo(action, machineId, repo.path, anyway);
        landed(machineId, outcome);
        if (action === "fetch" || action === "pull") noteFetch(key, outcome.state === "failed" ? (outcome.output ?? "") : "");
        if (outcome.state === "held") setHeld({ machineId, repo, outcome });
        else {
          const said = outcomeWords(outcome, repo.name);
          actions.notify(said.text, { error: said.error });
        }
      } catch (e) {
        actions.notify(errMsg(e), { error: true });
      } finally {
        mark(key, false);
        reload();
      }
    },
    [mark, landed, noteFetch, reload],
  );

  // fetchAll fetches every repository with a remote on the connected machines, FETCH_AT_ONCE at a time, then says
  // what failed, each by its repository and git's line
  const fetchAll = useCallback(
    async (machines: Listed[]) => {
      if (running.current) return;
      const rows = machines.filter((m) => !m.away && !m.error).flatMap((m) => m.repos.filter(fetchable).map((repo) => ({ machineId: m.id, repo })));
      if (!rows.length) return;
      running.current = true;
      const progress: FetchRun = { done: 0, total: rows.length, failed: [] };
      setRun({ ...progress });
      await pooled(rows, FETCH_AT_ONCE, async ({ machineId, repo }) => {
        const key = keyOf(machineId, repo.path);
        mark(key, true);
        try {
          const outcome = await fetchRepo(machineId, repo.path);
          landed(machineId, outcome);
          const why = outcome.state === "done" ? "" : outcome.output || outcomeWords(outcome, repo.name).text;
          noteFetch(key, outcome.state === "failed" ? why : "");
          if (why) progress.failed.push({ name: repo.name, why });
        } catch (e) {
          progress.failed.push({ name: repo.name, why: errMsg(e) });
        } finally {
          mark(key, false);
          progress.done++;
          setRun({ ...progress, failed: [...progress.failed] });
        }
      });
      running.current = false;
      setRun(null);
      const said = ranWords(progress);
      actions.notify(said.text, { error: said.error });
      reload();
    },
    [mark, landed, noteFetch, reload],
  );

  const say = useCallback((text: string) => actions.notify(text, { error: true }), []);
  return { busy, failed, run, held, dropHeld: () => setHeld(null), act, fetchAll, say };
}

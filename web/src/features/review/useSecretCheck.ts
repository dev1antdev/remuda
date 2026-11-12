// The lens's secret check: asked when the verdict dialog opens on an approval with a commit or a pull request, read
// afresh whenever that ask changes and once a minute while the dialog stays, kept once it closes so the rows it found
// stay marked, and dropped once the approval has gone out.

import { useCallback, useMemo, useState } from "react";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { fetchSecrets } from "./api";
import type { VerdictAsk } from "./draft";
import { asksPush, checkState, secretsByFile, type CheckState, type SecretScan } from "./secrets";

const RECHECK = 60_000;

// Round is one ask of the check: a new id reads it afresh, open says the dialog is there to show it
type Round = { push: boolean; id: number; open: boolean };
// Answer is a scan, or why there is none, with the round it answers: the render that starts a round never shows the
// last one's findings or failure as its own
type Answer = { round: number; scan?: SecretScan; error?: string };

export function useSecretCheck(machineId: string, dir: string) {
  const [round, setRound] = useState<Round | null>(null);
  const asked = round?.id ?? 0;
  const read = () =>
    fetchSecrets(machineId, dir, round?.push ?? false).then(
      (scan): Answer => ({ round: asked, scan }),
      (failure: unknown): Answer => ({ round: asked, error: errMsg(failure) }),
    );
  const query = useQuery<Answer>(read, [machineId, dir, round?.push, asked], { enabled: !!round?.open, every: RECHECK });
  // ask follows the dialog: an approval with a commit or a pull request asks, anything else (or the dialog gone) lets go
  const ask = useCallback((what: VerdictAsk | null) => {
    setRound((before) => {
      if (what === null || what === "none") return before && { ...before, open: false };
      const push = asksPush(what);
      return before?.open && before.push === push ? before : { push, id: Date.now(), open: true };
    });
  }, []);
  const clear = useCallback(() => setRound(null), []);
  const answer = round && query.data?.round === round.id ? query.data : null;
  const scan = answer?.scan ?? null;
  const error = answer?.error ?? "";
  const state = checkState(!!round?.open, scan, error, query.fetching);
  const marks = useMemo(() => secretsByFile(scan), [scan]);
  return { state, scan, error, ask, retry: query.reload, clear, marks, push: round?.push ?? false, open: !!round?.open };
}

export type SecretCheck = ReturnType<typeof useSecretCheck>;

// stateFor is where the check stands for the dialog's choice right now: the render that changes the choice comes before
// the round it asks for, and must not show the last round's answer as this one's
export const stateFor = (check: SecretCheck, ask: VerdictAsk): CheckState =>
  ask === "none" ? "off" : check.open && check.push === asksPush(ask) ? check.state : "checking";

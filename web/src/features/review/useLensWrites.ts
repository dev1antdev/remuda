// The lens's git writes at a row's or a group's press: stage, unstage, discard behind a confirm that names the file and
// what cannot come back; one at a time, the lens read again after each, git's words kept on screen when it refuses.

import { useState } from "react";
import { actions } from "@/app/store";
import { errMsg } from "@/lib/text";
import { discardFile, stageFiles, unstageFiles } from "./api";
import { discardAsk, discardKind, wroteWords, type BulkAction, type ChangeRow, type GroupId, type RowAction } from "./groups";

export function useLensWrites({ machineId, dir, onDone, onError }: { machineId: string; dir: string; onDone: () => void; onError: (text: string) => void }) {
  const [writing, setWriting] = useState<string | null>(null);
  const run = async (what: string, write: () => Promise<string>) => {
    if (writing) return;
    setWriting(what);
    try {
      actions.notify(await write(), { brief: true });
      onDone();
    } catch (failure) {
      onError(errMsg(failure));
    } finally {
      setWriting(null);
    }
  };
  const index = (action: BulkAction, paths: string[], bulk: boolean) => (action === "stage" ? stageFiles(machineId, dir, paths, bulk) : unstageFiles(machineId, dir, paths, bulk)).then((done) => wroteWords(action, done));
  const onAction = (row: ChangeRow, action: RowAction) => {
    if (action !== "discard") {
      void run(row.key, () => index(action, [row.file.path], false));
      return;
    }
    const ask = discardAsk(row);
    actions.ask(ask.text, ask.button, () => void run(row.key, () => discardFile(machineId, dir, row.file.path, discardKind(row)).then(() => ask.done)));
  };
  const onBulk = (group: GroupId, action: BulkAction, paths: string[]) => {
    if (paths.length > 0) void run(group, () => index(action, paths, true));
  };
  return { writing, onAction, onBulk };
}

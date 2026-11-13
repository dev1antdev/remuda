// A checkout's note, after Orca's worktree notes: whatever the person wants to remember about it, kept by the server
// by machine and folder after the workspace closes. ⌘↵ (Ctrl+↵ elsewhere) saves, Esc leaves it as it was, and saving it
// empty removes it.

import { useState, type KeyboardEvent } from "react";
import { isMac } from "@/lib/prefs";
import { t } from "@/lib/i18n";
import { Dialog, Notice, useSubmit } from "@/ui";
import { saveNote } from "./cardApi";
import { NOTE_MAX, noteBytes } from "./cardFacts";
import { useRoster } from "./useRoster";

export default function NoteDialog({ machineId, dir, name, text, onClose }: { machineId: string; dir: string; name: string; text: string; onClose: () => void }) {
  const [draft, setDraft] = useState(text);
  const { refresh } = useRoster();
  const bytes = noteBytes(draft);
  const tooLong = bytes > NOTE_MAX;
  const submit = useSubmit(async () => {
    if (tooLong) return;
    await saveNote(machineId, dir, draft);
    refresh();
    onClose();
  });
  const saveKey = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key !== "Enter" || !(isMac ? e.metaKey : e.ctrlKey)) return;
    e.preventDefault();
    void submit.run();
  };
  return (
    <Dialog
      as="form"
      className="note-dialog"
      title={
        <>
          {t("Note")}
          <span className="note-where">{name}</span>
        </>
      }
      icon="note"
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        void submit.run();
      }}
    >
      <textarea
        className="note-text"
        autoFocus
        rows={7}
        value={draft}
        placeholder={t("Anything to remember about this worktree")}
        aria-label={t("Note")}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={saveKey}
      />
      <p className="modal-hint note-hint">
        <span>{isMac ? t("⌘↵ saves · Esc leaves it as it was") : t("Ctrl+↵ saves · Esc leaves it as it was")}</span>
        <span className={`note-size${tooLong ? " over" : ""}`}>{bytes > NOTE_MAX * 0.75 ? t("{n} of {max} bytes", { n: bytes, max: NOTE_MAX }) : ""}</span>
      </p>
      {tooLong && <Notice tone="error">{t("A note holds 4 KB at most")}</Notice>}
      {submit.err && <Notice tone="error">{submit.err}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button type="submit" className="btn primary" disabled={submit.busy || tooLong}>
          {submit.busy ? "…" : draft.trim() || !text ? t("Save") : t("Remove the note")}
        </button>
      </div>
    </Dialog>
  );
}

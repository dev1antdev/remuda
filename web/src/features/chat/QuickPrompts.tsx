// Quick prompts: the saved texts offered under ⚡ in the composer, and the dialog that edits them as one list. A text may
// carry {dir}, {branch} and {file}: the pane's folder, its branch (asked of the machine on the pick) and the file last
// opened in the viewer.

import { useState } from "react";
import { useQuery } from "@/lib/query";
import { basename, errMsg } from "@/lib/text";
import { Dialog, Icon, Notice } from "@/ui";
import { fetchReview } from "@/features/review/api";
import { app } from "@/app/store";
import { fetchQuick, saveQuick, type QuickPrompt } from "./api";
import { expand, needs } from "./quickvars";
import { t } from "@/lib/i18n";

// expandPrompt fills a picked text's placeholders; the branch is one shell on the machine, run only when the text wants it
export async function expandPrompt(text: string, machineId: string, dir: string): Promise<string> {
  const wanted = needs(text);
  if (wanted.size === 0) return text;
  const last = app.get().lastFile;
  const file = last && last.machineId === machineId ? last.path : "";
  const branch = wanted.has("branch") && dir ? await fetchReview(machineId, dir).then((r) => r.branch, () => "") : "";
  return expand(text, { dir, branch, file });
}

export function QuickPromptsMenu({ at, dir, onPick, onManage }: { at: { x: number; y: number }; dir: string; onPick: (text: string) => void; onManage: () => void }) {
  const { data: prompts, error } = useQuery(fetchQuick, []);
  const visible = (prompts ?? []).filter((q) => !q.dir || q.dir === dir);
  return (
    <div className="row-menu quick-menu" role="menu" style={{ left: at.x, top: at.y }} onClick={(e) => e.stopPropagation()}>
      {visible.map((q) => (
        <button key={q.id ?? q.label} type="button" role="menuitem" title={q.text} onClick={() => onPick(q.text)}>
          <Icon name="zap" size={12} className="quick-icon" />
          <span className="quick-label">{q.label}</span>
          {q.dir && <span className="sel-hint">{t("this folder")}</span>}
        </button>
      ))}
      {error ? <Notice tone="error">{error}</Notice> : !prompts ? <Notice tone="hint">{t("Loading…")}</Notice> : visible.length === 0 && <Notice tone="hint">{t("No quick prompts yet.")}</Notice>}
      <button type="button" role="menuitem" className="quick-manage" onClick={onManage}>
        <Icon name="sliders" size={12} className="quick-icon" /> Manage…
      </button>
    </div>
  );
}

// QuickPromptsDialog edits the saved prompts as one list: a label, the text, every folder or this one
export function QuickPromptsDialog({ dir, onClose }: { dir: string; onClose: () => void }) {
  // the save replaces the list whole, so nothing is saved until the current one has loaded
  const [list, setList] = useState<QuickPrompt[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const loaded = useQuery(fetchQuick, []);
  if (loaded.data && list === null) setList(loaded.data);
  const edit = (i: number, patch: Partial<QuickPrompt>) => setList((l) => (l ?? []).map((q, j) => (j === i ? { ...q, ...patch } : q)));
  const folder = basename(dir);
  return (
    <Dialog
      title={t("Quick prompts")}
      icon="zap"
      size="md"
      as="form"
      className="quick-dialog"
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        if (!list || busy) return;
        setBusy(true);
        setErr("");
        saveQuick(list.filter((q) => q.text.trim()))
          .then(onClose, (er) => setErr(errMsg(er)))
          .finally(() => setBusy(false));
      }}
    >
      <p className="modal-hint">
        Offered under <Icon name="zap" size={12} /> in the composer; one tap puts the text in. Shared by every device that uses this remuda. <code>{"{dir}"}</code>, <code>{"{branch}"}</code> and <code>{"{file}"}</code> become the pane's folder, its branch and the file you last opened.
      </p>
      {!list && !loaded.error && <Notice tone="hint">{t("Loading…")}</Notice>}
      <div className="quick-list">
        {(list ?? []).map((q, i) => (
          <div key={i} className="quick-row">
            <input value={q.label} placeholder={t("Label")} onChange={(e) => edit(i, { label: e.target.value })} />
            <textarea rows={2} value={q.text} placeholder={t("The prompt")} onChange={(e) => edit(i, { text: e.target.value })} />
            <label className="quick-scope">
              <input type="checkbox" checked={!!q.dir} onChange={(e) => edit(i, { dir: e.target.checked ? dir : "" })} /> only {folder}
            </label>
            <button type="button" className="icon-btn danger" title={t("Remove")} aria-label={t("Remove")} onClick={() => setList((l) => (l ?? []).filter((_, j) => j !== i))}>
              <Icon name="trash" size={14} />
            </button>
          </div>
        ))}
      </div>
      <button type="button" className="btn sm" disabled={!list} onClick={() => setList((l) => [...(l ?? []), { label: "", text: "" }])}>
        <Icon name="plus" size={12} /> Add
      </button>
      {(err || loaded.error) && <Notice tone="error">{err || loaded.error}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button type="submit" className="btn primary" disabled={busy || !list}>
          {t("Save")}
        </button>
      </div>
    </Dialog>
  );
}

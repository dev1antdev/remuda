// Quick prompts: the saved texts offered under ⚡ in the composer, and the dialog that edits them as one list.

import { useState } from "react";
import { useQuery } from "@/lib/query";
import { basename, errMsg } from "@/lib/text";
import { Dialog, Icon, Notice } from "@/ui";
import { fetchQuick, saveQuick, type QuickPrompt } from "./api";

export function QuickPromptsMenu({ at, dir, onPick, onManage }: { at: { x: number; y: number }; dir: string; onPick: (text: string) => void; onManage: () => void }) {
  const { data: prompts, error } = useQuery(fetchQuick, []);
  const visible = (prompts ?? []).filter((q) => !q.dir || q.dir === dir);
  return (
    <div className="row-menu quick-menu" role="menu" style={{ left: at.x, top: at.y }} onClick={(e) => e.stopPropagation()}>
      {visible.map((q) => (
        <button key={q.id ?? q.label} type="button" role="menuitem" title={q.text} onClick={() => onPick(q.text)}>
          <Icon name="zap" size={12} className="quick-icon" />
          <span className="quick-label">{q.label}</span>
          {q.dir && <span className="sel-hint">this folder</span>}
        </button>
      ))}
      {error ? <Notice tone="error">{error}</Notice> : !prompts ? <Notice tone="hint">Loading…</Notice> : visible.length === 0 && <Notice tone="hint">No quick prompts yet.</Notice>}
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
      title="Quick prompts"
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
        Offered under <Icon name="zap" size={12} /> in the composer; one tap puts the text in. Shared by every device that uses this remuda.
      </p>
      {!list && !loaded.error && <Notice tone="hint">Loading…</Notice>}
      <div className="quick-list">
        {(list ?? []).map((q, i) => (
          <div key={i} className="quick-row">
            <input value={q.label} placeholder="Label" onChange={(e) => edit(i, { label: e.target.value })} />
            <textarea rows={2} value={q.text} placeholder="The prompt" onChange={(e) => edit(i, { text: e.target.value })} />
            <label className="quick-scope">
              <input type="checkbox" checked={!!q.dir} onChange={(e) => edit(i, { dir: e.target.checked ? dir : "" })} /> only {folder}
            </label>
            <button type="button" className="icon-btn danger" title="Remove" aria-label="Remove" onClick={() => setList((l) => (l ?? []).filter((_, j) => j !== i))}>
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
          Cancel
        </button>
        <button type="submit" className="btn primary" disabled={busy || !list}>
          Save
        </button>
      </div>
    </Dialog>
  );
}

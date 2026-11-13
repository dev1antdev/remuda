// One run action, after T3 Code's editor: its name, the command, the folder of the checkout it runs in, its icon, and
// where it opens — a new tab, split right, split down.

import { useState } from "react";
import { Dialog, Icon, Notice, Tip } from "@/ui";
import { t, tc } from "@/lib/i18n";
import { actionProblem, ICONS, PLACES, placeWords, type Action, type Place } from "./project";

const ICON_WORDS: Record<string, string> = {
  play: "Play",
  terminal: "Terminal",
  "circle-check": "Test",
  sparkles: "Lint",
  layers: "Build",
  settings: "Configure",
  zap: "Debug",
  "refresh-cw": "Watch",
  monitor: "Serve",
  server: "Server",
  code: "Code",
  download: "Install",
};

const newID = () => `a${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;

export default function ActionDialog({ action, onSave, onDelete, onClose }: { action: Action | null; onSave: (a: Action) => void; onDelete?: () => void; onClose: () => void }) {
  const [draft, setDraft] = useState<Action>(() => action ?? { id: newID(), name: "", command: "", folder: "", icon: "play", place: "tab" });
  const [tried, setTried] = useState(false);
  const problem = actionProblem(draft);
  const set = (patch: Partial<Action>) => setDraft({ ...draft, ...patch });
  return (
    <Dialog
      as="form"
      title={action ? t("Edit action") : t("Add action")}
      icon="play"
      className="action-editor"
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        setTried(true);
        if (!problem) onSave({ ...draft, name: draft.name.trim(), command: draft.command.trim(), folder: (draft.folder ?? "").trim() });
      }}
    >
      <label>
        {t("Name")}
        <input autoFocus value={draft.name} placeholder={t("Dev")} maxLength={40} onChange={(e) => set({ name: e.target.value })} />
      </label>
      <label>
        {t("Command")}
        <input className="mono" value={draft.command} placeholder="npm run dev" spellCheck={false} onChange={(e) => set({ command: e.target.value })} />
      </label>
      <label>
        {t("Folder (optional)")}
        <input className="mono" value={draft.folder ?? ""} placeholder={t("the checkout's top folder")} spellCheck={false} onChange={(e) => set({ folder: e.target.value })} />
      </label>
      <div className="action-choices">
        <div>
          <span className="action-choice-label">{t("Icon")}</span>
          <div className="action-icons" role="radiogroup" aria-label={t("Icon")}>
            {ICONS.map((icon) => (
              <Tip text={tc("run-icon", ICON_WORDS[icon] ?? icon)} key={icon}>
                <button type="button" role="radio" aria-checked={draft.icon === icon} aria-label={tc("run-icon", ICON_WORDS[icon] ?? icon)} className={`icon-btn${draft.icon === icon ? " on" : ""}`} onClick={() => set({ icon })}>
                  <Icon name={icon} size={14} />
                </button>
              </Tip>
            ))}
          </div>
        </div>
        <div>
          <span className="action-choice-label">{t("Runs in")}</span>
          <div className="action-places" role="radiogroup" aria-label={t("Runs in")}>
            {PLACES.map((place: Place) => (
              <button key={place} type="button" role="radio" aria-checked={(draft.place ?? "tab") === place} className={`btn sm${(draft.place ?? "tab") === place ? " on" : ""}`} onClick={() => set({ place })}>
                {placeWords(place)}
              </button>
            ))}
          </div>
        </div>
      </div>
      {tried && problem && <Notice tone="error">{problem}</Notice>}
      <div className="modal-actions">
        {action && onDelete && (
          <button type="button" className="linkish danger action-delete" onClick={onDelete}>
            {t("Delete")}
          </button>
        )}
        <button type="button" className="btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button type="submit" className="btn primary">
          {t("Save")}
        </button>
      </div>
    </Dialog>
  );
}

// The machine's own ⋯ menu: rename, updates, the keep-awake switch, the SSH host to copy, and — for one added
// here — remove.

import { useCallback, useState } from "react";
import { act } from "@/app/act";
import UpdatesDialog from "@/features/updates/Updates";
import { Icon, Tip, useDismiss } from "@/ui";
import { setAwake, type Machine } from "./api";
import { t } from "@/lib/i18n";

export function MachineMenu({ machine: m, onRename, onRemove }: { machine: Machine; onRename: () => void; onRemove?: () => void }) {
  const [open, setOpen] = useState<{ x: number; y: number } | null>(null);
  const [updates, setUpdates] = useState(false);
  const close = useCallback(() => setOpen(null), []);
  useDismiss(!!open, close, [".machine-menu", `[data-machine-menu="${CSS.escape(m.id)}"]`]);
  const pick = (fn: () => void) => () => {
    setOpen(null);
    fn();
  };
  return (
    <>
      <Tip text={t("Rename, copy the SSH host, remove")}>
      <button
        type="button"
        className={`icon-btn${open ? " on" : ""}`}
        data-machine-menu={m.id}
        aria-label={t("{machine}: rename, copy the SSH host, remove", { machine: m.label })}
        aria-haspopup="menu"
        aria-expanded={!!open}
        onClick={(e) => {
          e.stopPropagation();
          const r = e.currentTarget.getBoundingClientRect();
          setOpen(open ? null : { x: Math.min(r.left, window.innerWidth - 230), y: r.bottom + 4 });
        }}
      >
        <Icon name="more" size={14} />
      </button>
      </Tip>
      {open && (
        <div className="row-menu machine-menu" role="menu" style={{ left: open.x, top: open.y }} onClick={(e) => e.stopPropagation()}>
          <button type="button" role="menuitem" onClick={pick(onRename)}>
            <Icon name="edit" size={14} /> {t("Rename…")}
          </button>
          <button type="button" role="menuitem" onClick={pick(() => setUpdates(true))} disabled={m.state !== "connected"}>
            <Icon name="refresh-cw" size={14} /> {t("Updates…")}
          </button>
          <button
            type="button"
            role="menuitemcheckbox"
            aria-checked={!!m.awake?.on}
            className={m.awake?.on ? "on" : undefined}
            onClick={pick(() => void act(setAwake(m.id, !m.awake?.on)))}
            disabled={m.state !== "connected"}
          >
            <Icon name={m.awake?.on ? "check" : "sun"} size={14} /> {t("Keep awake while agents work")}
          </button>
          {m.host && (
            <button type="button" role="menuitem" onClick={pick(() => void navigator.clipboard?.writeText(m.host!))}>
              <Icon name="copy" size={14} /> {t("Copy SSH host")}
            </button>
          )}
          {onRemove && (
            <button type="button" role="menuitem" className="danger" onClick={pick(onRemove)}>
              <Icon name="trash" size={14} /> {t("Remove machine")}
            </button>
          )}
        </div>
      )}
      {updates && <UpdatesDialog machine={m} onClose={() => setUpdates(false)} />}
    </>
  );
}

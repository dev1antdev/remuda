// The floating terminal: a drawer over every mode on a machine's scratch shell — its own plain-shell workspace in the
// home folder, made once — opened and hidden by Ctrl+`, the rail or the palette. Hiding it leaves the shell running and,
// once it was on screen, its attach too, so it comes back as it was; it never changes the pane or lens on screen.

import { useEffect, useState } from "react";
import { act } from "@/app/act";
import { actions, app } from "@/app/store";
import { useFlavor, useMono } from "@/app/theme";
import { machineLabel, machineOptions } from "@/features/roster/options";
import { useRoster } from "@/features/roster/useRoster";
import { monoStack } from "@/lib/look";
import { isMac, isTouch, usePref } from "@/lib/prefs";
import { errMsg } from "@/lib/text";
import { Icon, Notice, PANELS, Resizer, Select, Tip } from "@/ui";
import type { TermApi } from "../engine/types";
import TerminalView from "../Terminal";
import { closeScratch, openScratch } from "./api";
import { keyWords } from "./float";
import { t, tn } from "@/lib/i18n";

export default function FloatTerminal() {
  const float = app.use((s) => s.float);
  const { viewOnly } = useRoster();
  const [shown, setShown] = useState<string | null>(null); // the machine whose drawer is mounted; it stays while hidden
  useEffect(() => {
    if (float) setShown(float.machineId);
  }, [float?.machineId]); // eslint-disable-line react-hooks/exhaustive-deps
  if (viewOnly || !shown) return null;
  return <Drawer key={shown} machineId={shown} open={!!float} />;
}

function Drawer({ machineId, open }: { machineId: string; open: boolean }) {
  const { machines, refresh } = useRoster();
  const [pane, setPane] = useState("");
  const [error, setError] = useState("");
  const [asked, setAsked] = useState(0); // a fresh shell asked for after the last one ended
  const [mounted, setMounted] = useState(false); // the terminal mounts only once on screen: unseen it would attach at 80×24
  const [seen, setSeen] = useState(false); // the roster has listed the shell's pane, so its absence later means it ended
  const [api, setApi] = useState<TermApi | null>(null);
  const flavor = useFlavor();
  const mono = useMono();
  const [fontSize] = usePref("termFont", isTouch ? 12 : 13);
  const machine = machineLabel(machines, machineId);
  const key = keyWords(isMac);

  useEffect(() => {
    let live = true;
    setError("");
    setPane("");
    setSeen(false);
    setMounted(false);
    openScratch(machineId).then(
      (shell) => {
        if (!live) return;
        setPane(shell.pane);
        if (shell.made) refresh();
      },
      (e) => live && setError(errMsg(e)),
    );
    return () => {
      live = false;
    };
  }, [machineId, asked]); // eslint-disable-line react-hooks/exhaustive-deps

  const listed = !!pane && machines.some((m) => m.id === machineId && m.sessions.some((s) => s.tabs.some((tab) => tab.panes.some((p) => p.id === pane))));
  useEffect(() => {
    if (listed) setSeen(true);
  }, [listed]);
  const ended = seen && !listed;

  useEffect(() => {
    if (open && pane && !ended) setMounted(true);
  }, [open, pane, ended]);
  useEffect(() => {
    if (open && api && !isTouch) api.focus();
  }, [open, api]);

  const endShell = () =>
    actions.ask(t("End the floating terminal's shell on {machine}? Whatever runs in it stops.", { machine }), t("End the shell"), () =>
      void act(closeScratch(machineId).then(() => actions.closeFloat())),
    );

  return (
    <section className="float-term" hidden={!open} aria-label={t("Floating terminal")}>
      <Resizer spec={PANELS.float} edge="top" label={t("Floating terminal height")} />
      <header className="float-term-head">
        <Icon name="square-terminal" size={14} />
        <span className="float-term-title">{t("Terminal")}</span>
        <Select className="sm" value={machineId} options={machineOptions(machines)} onChange={(id) => actions.openFloat(id)} />
        <span className="float-term-hint">{t("{key} hides it; the shell keeps running", { key })}</span>
        <Tip text={t("End this shell")}>
          <button type="button" className="icon-btn sm float-term-end" aria-label={t("End this shell")} disabled={!pane} onClick={endShell}>
            <Icon name="trash" size={14} />
          </button>
        </Tip>
        <Tip text={t("Hide ({key}); the shell keeps running", { key })}>
          <button type="button" className="icon-btn sm" aria-label={t("Hide the floating terminal")} onClick={actions.closeFloat}>
            <Icon name="x" size={14} />
          </button>
        </Tip>
      </header>
      <div className="float-term-body">
        {error ? (
          <Notice tone="error">{error}</Notice>
        ) : ended ? (
          <div className="float-term-note">
            <p>{t("The shell ended.")}</p>
            <button type="button" className="btn" onClick={() => setAsked((n) => n + 1)}>
              {t("Open a new one")}
            </button>
          </div>
        ) : mounted ? (
          <TerminalView
            machineId={machineId}
            paneId={pane}
            cols={0}
            rows={0}
            agent={false}
            mode="fit"
            minCols={0}
            flavor={flavor}
            fontFamily={monoStack(mono)}
            fontSize={fontSize}
            modifier={null}
            selectMode={false}
            onModifierUsed={() => {}}
            onReady={setApi}
            onOpenFile={(target) => actions.openFile(machineId, pane, target)}
            onCopied={(n) => actions.notify(tn(n, "Copied {n} character", "Copied {n} characters"), { brief: true })}
            active={open}
          />
        ) : (
          <p className="float-term-note dim">{t("Opening a shell on {machine}…", { machine })}</p>
        )}
      </div>
    </section>
  );
}

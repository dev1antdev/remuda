// The concierge's place: a panel with the machine it runs on, New conversation, and its conversation, which the chat
// lens's own pieces draw (ConciergeChat). Opening it starts the concierge there when it is not running; nothing is ever
// sent to it but what the person types.

import { useCallback, useEffect, useRef, useState } from "react";
import { useRoster } from "@/features/roster/useRoster";
import { t } from "@/lib/i18n";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { Dialog, Icon, Menu, Notice, Select, Tip } from "@/ui";
import { fetchConcierge, newConversation, pickConcierge, startConcierge, stopConcierge, type ConciergeRunning, type ConciergeView } from "@/features/concierge/api";
import { comingUp, conciergePane, labelOf, machineChoices, needsStart } from "@/features/concierge/concierge";
import ConciergeChat from "./ConciergeChat";

export default function Concierge({ onClose }: { onClose: () => void }) {
  const { concierges } = useRoster();
  const { data: view, error: viewError, mutate } = useQuery<ConciergeView>(fetchConcierge, [], { every: 5000, keep: true });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [setAside, setSetAside] = useState<string | null>(null);
  const shown = useRef<string | undefined>(undefined);
  const startedFor = useRef(""); // the machine an opening already started it on: a stop is not undone by the next poll
  const machineId = view?.machine ?? "";
  const running = view?.running?.machine === machineId ? view.running : undefined;
  const pane = running ? conciergePane(concierges, machineId, running.pane) : undefined;
  const label = labelOf(view, machineId);

  const landed = useCallback(
    (run: ConciergeRunning) => mutate((was) => (was ? { ...was, running: run } : was)),
    [mutate],
  );
  const act = useCallback(
    async (work: () => Promise<void>) => {
      setBusy(true);
      setError(null);
      try {
        await work();
      } catch (e) {
        setError(errMsg(e));
      } finally {
        setBusy(false);
      }
    },
    [],
  );
  const start = useCallback(() => act(async () => landed(await startConcierge())), [act, landed]);

  // opening the place, or another machine picked, starts it there once when it is not running or its pane is a bare shell
  const wanted = needsStart(!!running, pane);
  useEffect(() => {
    if (!view || view.trouble || !machineId || !wanted || startedFor.current === machineId) return;
    startedFor.current = machineId;
    void start();
  }, [view, machineId, wanted, start]);

  const pick = (machine: string) =>
    act(async () => {
      const next = await pickConcierge(machine);
      mutate(() => next);
    });
  const fresh = () =>
    act(async () => {
      setSetAside(shown.current ?? null);
      landed(await newConversation());
    });
  const stop = () =>
    act(async () => {
      await stopConcierge();
      mutate((was) => (was ? { ...was, running: undefined } : was));
    });
  const onSession = useCallback((id: string | undefined) => {
    shown.current = id;
  }, []);

  const waiting = !view ? t("Loading…") : view.trouble ? "" : running ? comingUp(label, pane) : busy ? comingUp(label, undefined) : "";
  return (
    <Dialog title={t("Concierge")} icon="concierge" size="lg" className="concierge" onClose={onClose}>
      <div className="concierge-bar">
        {view && view.machines.length > 0 && (
          <Tip text={t("The machine the concierge runs on: a Claude Code agent there, reading through remuda")}>
            <span className="concierge-machine">
              <Select value={machineId} options={machineChoices(view)} onChange={(machine) => void pick(machine)} />
            </span>
          </Tip>
        )}
        <span className="concierge-about">{t("Reads your agents and tasks · files tasks into Proposed")}</span>
        <button type="button" className="btn sm" disabled={busy || !pane?.agent} onClick={() => void fresh()}>
          <Icon name="plus" size={12} /> {t("New conversation")}
        </button>
        <Menu
          label={t("Concierge")}
          tip={t("More")}
          trigger={
            <button type="button" className="icon-btn sm" aria-label={t("More")} disabled={busy || !running}>
              <Icon name="more" size={14} />
            </button>
          }
          items={[{ label: t("Stop the concierge"), icon: "square", hint: t("its conversation comes back the next time it starts"), onSelect: () => void stop() }]}
        />
      </div>
      {view?.trouble && <Notice tone="warn" icon="alert-triangle">{view.trouble}</Notice>}
      {(error || viewError) && (
        <Notice tone="error" onDismiss={() => setError(null)}>
          {error ?? viewError}
        </Notice>
      )}
      {pane?.agent && running ? (
        <ConciergeChat machineId={machineId} pane={pane} dir={running.dir} setAside={setAside} onSession={onSession} />
      ) : (
        <div className="concierge-waiting">
          {waiting ? (
            <>
              <span className="concierge-coming">
                <span className="search-busy" /> {waiting}
              </span>
              {running && pane && !pane.agent && !busy && (
                <button type="button" className="linkish" onClick={() => void start()}>
                  {t("Start it again")}
                </button>
              )}
            </>
          ) : (
            view &&
            !view.trouble &&
            !busy && (
              <>
                <span>{t("The concierge is not running on {machine}.", { machine: label })}</span>
                <button type="button" className="btn primary" onClick={() => void start()}>
                  {t("Start it")}
                </button>
              </>
            )
          )}
        </div>
      )}
    </Dialog>
  );
}

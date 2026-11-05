// Update everywhere: every connected machine's pending updates — herdr, its hooks, the agents, code-server, remuda's
// release — each with the command it runs, shown before Run; then the run, one step after another on the server, the
// page's own host last, with how each went and one summary. Closing the dialog keeps the run going.

import { useEffect, useRef, useState } from "react";
import { useRoster } from "@/features/roster/useRoster";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { Dialog, Notice } from "@/ui";
import { t } from "@/lib/i18n";
import { everywherePlan, fetchEverywhere, startEverywhere, type Batch, type Step } from "./api";
import { byMachine, hostRestarting, restartsHost, stateWords, summaryWords } from "./everywhere";

export default function EverywhereDialog({ onClose }: { onClose: () => void }) {
  const { host } = useRoster();
  const { data: plan, error, loading } = useQuery(everywherePlan, []);
  const [batch, setBatch] = useState<Batch | null>(null);
  const [failure, setFailure] = useState("");
  const [starting, setStarting] = useState(false);
  const [gone, setGone] = useState(false); // the run went down with the page's own remuda, restarting onto its release
  const shown = useRef<Batch | null>(null);
  shown.current = batch;
  useEffect(() => {
    if (!batch || batch.done || gone) return;
    const id = batch.id;
    const lost = (e: unknown) => (hostRestarting(shown.current?.steps ?? [], host?.id) ? setGone(true) : setFailure(errMsg(e)));
    const timer = setInterval(() => fetchEverywhere(id).then(setBatch, lost), 2000);
    return () => clearInterval(timer);
  }, [batch?.id, batch?.done, gone]); // eslint-disable-line react-hooks/exhaustive-deps
  const run = async (steps: Step[]) => {
    setStarting(true);
    setFailure("");
    try {
      setBatch(await startEverywhere(steps));
    } catch (e) {
      setFailure(errMsg(e));
    } finally {
      setStarting(false);
    }
  };
  const steps = plan ?? [];
  const restarts = restartsHost(steps, host?.id);
  return (
    <Dialog title={t("Update everywhere")} icon="refresh-cw" size="md" onClose={onClose} className="updates everywhere">
      {(error || failure) && <Notice tone="error">{failure || error}</Notice>}
      {batch ? (
        <>
          {byMachine(batch.steps).map((group) => (
            <section key={group.machine} className="everywhere-machine">
              <h3>{group.label}</h3>
              <ul className="everywhere-steps">
                {group.steps.map((step) => (
                  <li key={step.what} className={`is-${step.state}`}>
                    <span className="everywhere-title">{step.title}</span>
                    <span className="everywhere-state">{stateWords(step)}</span>
                  </li>
                ))}
              </ul>
            </section>
          ))}
          {gone ? (
            <Notice tone="info">{t("remuda here is restarting onto its new version; the page offers a reload once it answers.")}</Notice>
          ) : (
            <Notice tone={batch.done ? (batch.steps.some((s) => s.state === "failed") ? "warn" : "success") : "info"}>
              {!batch.done && <span className="spinner" aria-hidden="true" />} {summaryWords(batch.steps, batch.done)}
              {batch.done && restarts ? ` · ${t("remuda here restarts now; the page offers a reload once the new version answers.")}` : ""}
            </Notice>
          )}
        </>
      ) : loading && !plan ? (
        <Notice tone="hint">{t("Looking at every machine…")}</Notice>
      ) : steps.length === 0 ? (
        <Notice tone="success">{t("Everything is up to date on every connected machine.")}</Notice>
      ) : (
        <>
          <p className="modal-hint">{t("One step after another, each as its command says; a step that fails leaves the rest to run. The page's own remuda goes last: it restarts once its step ends.")}</p>
          {byMachine(steps).map((group) => (
            <section key={group.machine} className="everywhere-machine">
              <h3>{group.label}</h3>
              <ul className="everywhere-steps">
                {group.steps.map((step) => (
                  <li key={step.what}>
                    <details>
                      <summary className="everywhere-title">{step.title}</summary>
                      <code className="updates-cmd">{step.command}</code>
                    </details>
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </>
      )}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          {batch && !batch.done ? t("Close (it keeps running)") : t("Close")}
        </button>
        {!batch && steps.length > 0 && (
          <button type="button" className="btn primary" disabled={starting} onClick={() => void run(steps)}>
            {t("Run all {n}", { n: steps.length })}
          </button>
        )}
      </div>
    </Dialog>
  );
}

// BroadcastDialog is one message to several agents: tick the agents — by hand, or every one, the working ones, the
// resting ones — write the text, and it goes to each in turn through the chat's own path; the dialog then says how
// each went. An agent busy with a question refuses, and that is shown, never retried.

import { useState } from "react";
import { submitPrompt } from "@/features/roster/api";
import { Dialog, Icon, Notice, useSubmit } from "@/ui";
import Logo from "@/ui/Logo";
import { broadcast, picked, statusWords, type Outcome, type QuickPick } from "./broadcast";
import { targetKey, type AgentTarget } from "./targets";
import { t } from "@/lib/i18n";

const PICKS: { pick: QuickPick; label: string; title: string }[] = [
  { pick: "all", label: t("All"), title: t("Every agent that would take a message") },
  { pick: "working", label: t("Working"), title: t("The agents in the middle of something") },
  { pick: "resting", label: t("Resting"), title: t("The idle and the finished ones") },
  { pick: "none", label: t("None"), title: t("Untick everyone") },
];

export default function BroadcastDialog({ agents, onClose }: { agents: AgentTarget[]; onClose: () => void }) {
  const [ticked, setTicked] = useState<Set<string>>(() => picked(agents, "all"));
  const [text, setText] = useState("");
  const [outcomes, setOutcomes] = useState<Outcome[] | null>(null);
  const chosen = agents.filter((agent) => ticked.has(targetKey(agent)));
  const { busy, err, run } = useSubmit(async () => {
    if (chosen.length === 0 || !text.trim()) return;
    setOutcomes(await broadcast(chosen, text.trim(), (target, message) => submitPrompt(target.machineId, target.paneId, message)));
  });
  const toggle = (key: string) =>
    setTicked((old) => {
      const next = new Set(old);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  const machines = [...new Set(agents.map((agent) => agent.machineLabel))];
  const failed = outcomes?.filter((outcome) => outcome.error).length ?? 0;
  return (
    <Dialog
      title={t("Message several agents")}
      icon="send"
      size="md"
      as="form"
      className="broadcast-dialog"
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        void run();
      }}
    >
      {outcomes ? (
        <>
          <Notice tone={failed ? "warn" : "success"}>{failed ? `Sent to ${outcomes.length - failed} of ${outcomes.length}; ${failed} refused.` : `Sent to ${outcomes.length} agent${outcomes.length === 1 ? "" : "s"}.`}</Notice>
          <ul className="pick-list">
            {outcomes.map(({ target, error }) => (
              <li key={targetKey(target)} className={`pick-row${error ? " failed" : ""}`}>
                <Icon name={error ? "alert-triangle" : "circle-check"} size={14} />
                <Logo agent={target.agent} />
                <span className="pick-name">
                  {target.sessionName} › {target.paneTitle}
                </span>
                <span className="pick-hint">{error ?? "sent"}</span>
              </li>
            ))}
          </ul>
          <div className="modal-actions">
            <button type="button" className="btn primary" onClick={onClose}>
              {t("Done")}
            </button>
          </div>
        </>
      ) : agents.length === 0 ? (
        <Notice tone="hint">{t("No agent is running.")}</Notice>
      ) : (
        <>
          <div className="pick-quick">
            <span className="modal-hint">{chosen.length} of {agents.length} ticked</span>
            {PICKS.map((p) => (
              <button key={p.pick} type="button" className="btn sm" title={p.title} onClick={() => setTicked(picked(agents, p.pick))}>
                {p.label}
              </button>
            ))}
          </div>
          <ul className="pick-list">
            {machines.map((label) => (
              <li key={label} className="pick-group">
                {machines.length > 1 && <span className="pick-machine">{label}</span>}
                <ul>
                  {agents
                    .filter((agent) => agent.machineLabel === label)
                    .map((agent) => {
                      const key = targetKey(agent);
                      return (
                        <li key={key}>
                          <label className={`pick-row${agent.status === "input" ? " asking" : ""}`} title={agent.cwd}>
                            <input type="checkbox" checked={ticked.has(key)} onChange={() => toggle(key)} />
                            <Logo agent={agent.agent} />
                            <span className="pick-name">
                              {agent.sessionName} › {agent.paneTitle}
                            </span>
                            <span className="pick-hint">{statusWords(agent.status)}</span>
                          </label>
                        </li>
                      );
                    })}
                </ul>
              </li>
            ))}
          </ul>
          <label>
            {t("Message")}
            <textarea value={text} onChange={(e) => setText(e.target.value)} rows={5} placeholder={t("One text, to each of them in turn — /compact, a question, a nudge")} />
          </label>
          {err && <Notice tone="error">{err}</Notice>}
          <div className="modal-actions">
            <button type="button" className="btn" onClick={onClose}>
              {t("Cancel")}
            </button>
            <button type="submit" className="btn primary" disabled={busy || chosen.length === 0 || !text.trim()}>
              {busy ? t("Sending…") : `Send to ${chosen.length}`}
            </button>
          </div>
        </>
      )}
    </Dialog>
  );
}

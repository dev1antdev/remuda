// Approve or Reject: the agent to tell, a note, the draft's comments along with it, and on an approval what is asked of
// the agent beyond the word — the commit, or the commit pushed as a pull request; remuda never writes into a repository.

import { useState } from "react";
import { targetKey, type AgentTarget } from "@/features/agents/targets";
import { plural } from "@/lib/text";
import { Dialog, Logo, Notice, Select, useSubmit } from "@/ui";
import type { Verdict } from "./api";
import type { VerdictAsk } from "./draft";
import { t, tn } from "@/lib/i18n";

const ASKS: { value: VerdictAsk; label: string }[] = [
  { value: "none", label: t("Approve only") },
  { value: "commit", label: t("and ask the agent to commit the reviewed work") },
  { value: "pr", label: t("and ask it to commit, push and open a pull request") },
];

export const approveWords = (ask: VerdictAsk) => (ask === "pr" ? t("Approve & open PR") : ask === "commit" ? t("Approve & commit") : t("Approve"));

export function VerdictDialog({ verdict, paneAgent, agents, currentPane, comments, onClose, onSend }: {
  verdict: Verdict;
  paneAgent: string; // "" when this pane has none
  agents: AgentTarget[];
  currentPane: { machineId: string; paneId: string } | null;
  comments: number;
  onClose: () => void;
  onSend: (target: AgentTarget, note: string, ask: VerdictAsk) => Promise<void>;
}) {
  const approving = verdict === "approved";
  const [note, setNote] = useState("");
  const [ask, setAsk] = useState<VerdictAsk>("none");
  const paneTarget =
    currentPane && paneAgent
      ? (agents.find((candidate) => targetKey(candidate) === targetKey(currentPane)) ?? { ...currentPane, agent: paneAgent, machineLabel: "", sessionName: "", paneTitle: "", status: "", cwd: "" })
      : null;
  const targets = paneTarget && !agents.some((candidate) => targetKey(candidate) === targetKey(paneTarget)) ? [paneTarget, ...agents] : agents;
  const [chosenKey, setChosenKey] = useState(paneTarget ? targetKey(paneTarget) : "");
  const chosen = targets.find((candidate) => targetKey(candidate) === chosenKey);
  const nothingToFix = !approving && comments === 0 && !note.trim();
  const { busy, err: failure, run: submit } = useSubmit(async () => {
    if (!chosen || nothingToFix) return;
    await onSend(chosen, note, approving ? ask : "none");
  });
  return (
    <Dialog
      title={approving ? t("Approve the changes") : t("Reject the changes")}
      icon={approving ? "check" : "x"}
      size="md"
      as="form"
      onClose={onClose}
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      {targets.length === 0 ? (
        <Notice tone="hint">{t("No agent is running to tell.")}</Notice>
      ) : targets.length === 1 && chosen ? (
        <p className="modal-hint">
          To {chosen.agent}
          {chosen.sessionName ? ` ${t("in {workspace}", { workspace: chosen.sessionName })}` : t(" in this pane")}.
        </p>
      ) : (
        <label>
          {t("Agent")}
          <Select
            value={chosenKey}
            onChange={setChosenKey}
            placeholder={t("Choose the agent")}
            options={targets.map((candidate) => ({
              value: targetKey(candidate),
              label: candidate.sessionName ? `${candidate.sessionName} › ${candidate.paneTitle}` : t("this pane"),
              hint: candidate.status ? `${candidate.agent} · ${candidate.status}` : candidate.agent,
              group: candidate.machineLabel || undefined,
              icon: <Logo agent={candidate.agent} />,
            }))}
          />
        </label>
      )}
      <label>
        {approving ? "A note for the agent (optional)" : t("What should change")}
        <textarea
          autoFocus
          rows={5}
          value={note}
          placeholder={approving ? t("Anything to take along; it goes with the approval.") : comments > 0 ? t("Goes with the comments.") : t("No comments in the draft: say it here.")}
          onChange={(event) => setNote(event.target.value)}
        />
      </label>
      {approving && (
        <div className="verdict-asks" role="radiogroup" aria-label={t("Afterwards")}>
          <span className="verdict-asks-name">{t("Afterwards")}</span>
          {ASKS.map((option) => (
            <label key={option.value} className="verdict-ask">
              <input type="radio" name="verdict-ask" value={option.value} checked={ask === option.value} onChange={() => setAsk(option.value)} />
              <span>{option.label}</span>
            </label>
          ))}
        </div>
      )}
      <p className="modal-hint">
        {comments > 0 ? tn(comments, "{n} comment from the draft goes along. ", "{n} comments from the draft go along. ") : ""}
        {approving ? t("The agent is told the changes are reviewed and it can carry on; the next look starts from now.") : t("The agent is told to work on this and say when it is ready for another look; the next look starts from now.")}
      </p>
      {failure && <Notice tone="error">{failure}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button type="submit" className={`btn ${approving ? "primary" : "danger"}`} disabled={busy || !chosen || nothingToFix}>
          {busy ? t("Sending…") : approving ? approveWords(ask) : t("Reject")}
        </button>
      </div>
    </Dialog>
  );
}

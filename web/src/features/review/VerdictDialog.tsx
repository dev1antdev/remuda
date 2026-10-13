// Approve or Reject: the agent to tell, a note, and the draft's comments along with it.

import { useState } from "react";
import { targetKey, type AgentTarget } from "@/features/agents/targets";
import { plural } from "@/lib/text";
import { Dialog, Logo, Notice, Select, useSubmit } from "@/ui";
import type { Verdict } from "./api";

export function VerdictDialog({ verdict, paneAgent, agents, currentPane, comments, onClose, onSend }: {
  verdict: Verdict;
  paneAgent: string; // "" when this pane has none
  agents: AgentTarget[];
  currentPane: { machineId: string; paneId: string } | null;
  comments: number;
  onClose: () => void;
  onSend: (target: AgentTarget, note: string) => Promise<void>;
}) {
  const approving = verdict === "approved";
  const [note, setNote] = useState("");
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
    await onSend(chosen, note);
  });
  return (
    <Dialog
      title={approving ? "Approve the changes" : "Reject the changes"}
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
        <Notice tone="hint">No agent is running to tell.</Notice>
      ) : targets.length === 1 && chosen ? (
        <p className="modal-hint">
          To {chosen.agent}
          {chosen.sessionName ? ` in ${chosen.sessionName}` : " in this pane"}.
        </p>
      ) : (
        <label>
          Agent
          <Select
            value={chosenKey}
            onChange={setChosenKey}
            placeholder="Choose the agent"
            options={targets.map((candidate) => ({
              value: targetKey(candidate),
              label: candidate.sessionName ? `${candidate.sessionName} › ${candidate.paneTitle}` : "this pane",
              hint: candidate.status ? `${candidate.agent} · ${candidate.status}` : candidate.agent,
              group: candidate.machineLabel || undefined,
              icon: <Logo agent={candidate.agent} />,
            }))}
          />
        </label>
      )}
      <label>
        {approving ? "A note for the agent (optional)" : "What should change"}
        <textarea
          autoFocus
          rows={5}
          value={note}
          placeholder={approving ? "Anything to take along; it goes with the approval." : comments > 0 ? "Goes with the comments." : "No comments in the draft: say it here."}
          onChange={(event) => setNote(event.target.value)}
        />
      </label>
      <p className="modal-hint">
        {comments > 0 ? `${plural(comments, "comment")} from the draft ${comments === 1 ? "goes" : "go"} along. ` : ""}
        {approving ? "The agent is told the changes are reviewed and it can carry on; the next look starts from now." : "The agent is told to work on this and say when it is ready for another look; the next look starts from now."}
      </p>
      {failure && <Notice tone="error">{failure}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          Cancel
        </button>
        <button type="submit" className={`btn ${approving ? "primary" : "danger"}`} disabled={busy || !chosen || nothingToFix}>
          {busy ? "Sending…" : approving ? "Approve" : "Reject"}
        </button>
      </div>
    </Dialog>
  );
}

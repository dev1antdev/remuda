// One dialog for anything that goes to an agent as a prompt — a task, a message
// forwarded from another agent: pick the agent (every running one, by machine,
// the last used for this key preselected), edit the text, send through the chat's own path.

import { useState } from "react";
import type { AgentTarget } from "@/features/agents/targets";
import { usePref } from "@/lib/prefs";
import Select from "@/ui/Select";
import Logo from "@/ui/Logo";
import { Dialog, Notice, useSubmit } from "@/ui";

export const targetKey = (a: { machineId: string; paneId: string }) => `${a.machineId}|${a.paneId}`;

export default function SendToAgent({ title, text: initialText, agents, currentPane, rememberKey, exclude, onClose, onSend }: {
  title: string;
  text: string;
  agents: AgentTarget[];
  currentPane: { machineId: string; paneId: string } | null;
  rememberKey: string; // the last target for this key is offered first
  exclude?: { machineId: string; paneId: string };
  onClose: () => void;
  onSend: (target: AgentTarget, text: string) => Promise<void>;
}) {
  const [remembered, setRemembered] = usePref<Record<string, string>>("send.targets", {});
  const choices = exclude ? agents.filter((a) => targetKey(a) !== targetKey(exclude)) : agents;
  const initial =
    choices.find((a) => targetKey(a) === remembered[rememberKey]) ??
    (currentPane && choices.find((a) => targetKey(a) === targetKey(currentPane))) ??
    choices[0];
  const [target, setTarget] = useState(initial ? targetKey(initial) : "");
  const [text, setText] = useState(initialText);
  const chosen = choices.find((a) => targetKey(a) === target);
  const { busy, err, run } = useSubmit(async () => {
    if (!chosen || !text.trim()) return;
    setRemembered({ ...remembered, [rememberKey]: target }); // before the dialog goes away
    await onSend(chosen, text);
  });
  return (
    <Dialog
      title={title}
      icon="send"
      size="md"
      as="form"
      className="tasks-send-dialog"
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        void run();
      }}
    >
      {choices.length === 0 ? (
        <Notice tone="hint">No other agent is running.</Notice>
      ) : (
        <label>
          Agent
          <Select
            value={target}
            onChange={setTarget}
            options={choices.map((a) => ({
              value: targetKey(a),
              label: `${a.sessionName} › ${a.paneTitle}`,
              hint: `${a.agent} · ${a.status}`,
              group: a.machineLabel,
              icon: <Logo agent={a.agent} />,
            }))}
          />
        </label>
      )}
      {chosen && <p className="modal-hint tasks-send-cwd">{chosen.cwd}</p>}
      <label>
        Message
        <textarea value={text} onChange={(e) => setText(e.target.value)} rows={9} />
      </label>
      {err && <Notice tone="error">{err}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          Cancel
        </button>
        <button type="submit" className="btn primary" disabled={busy || !chosen || !text.trim()}>
          {busy ? "Sending…" : "Send"}
        </button>
      </div>
    </Dialog>
  );
}

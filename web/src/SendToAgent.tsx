// One dialog for anything that goes to an agent as a prompt — a task, a
// message forwarded from another agent: pick the agent (every running one,
// by machine, the last one used for this key preselected), edit the text,
// send through the chat's own path.

import { useState } from "react";
import type { AgentTarget } from "./Tasks";
import { errMsg, usePref } from "./prefs";
import Select from "./Select";
import Logo from "./Logo";

export const targetKey = (a: { machineId: string; paneId: string }) => `${a.machineId}|${a.paneId}`;

export default function SendToAgent({ title, text: initialText, agents, currentPane, rememberKey, exclude, onClose, onSend }: {
  title: string;
  text: string;
  agents: AgentTarget[];
  currentPane: { machineId: string; paneId: string } | null;
  rememberKey: string; // the last target for this key is offered first
  exclude?: { machineId: string; paneId: string }; // not to itself
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
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const chosen = choices.find((a) => targetKey(a) === target);
  return (
    <div className="modal-scrim" onClick={onClose}>
      <form
        className="modal tasks-send-dialog"
        onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => {
          e.preventDefault();
          if (!chosen || !text.trim()) return;
          setBusy(true);
          setErr("");
          setRemembered({ ...remembered, [rememberKey]: target }); // before the dialog goes away
          onSend(chosen, text).catch((e) => {
            setErr(errMsg(e));
            setBusy(false);
          });
        }}
      >
        <h2>{title}</h2>
        {choices.length === 0 ? (
          <p className="modal-hint">No other agent is running.</p>
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
        {err && <div className="chat-error">{err}</div>}
        <div className="modal-actions">
          <button type="button" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="primary" disabled={busy || !chosen || !text.trim()}>
            {busy ? "…" : "Send"}
          </button>
        </div>
      </form>
    </div>
  );
}

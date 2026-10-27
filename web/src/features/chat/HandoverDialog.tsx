// Continue a conversation with another agent: the recap and the last exchanges become the first prompt of a fresh
// agent in a new tab of the same workspace. The pane handing over stays as it is.

import { useState } from "react";
import type { AgentKind } from "@/features/roster/api";
import { choiceFor, CUSTOM, EMPTY_DRAFT, hasModelFlag, lastModelKey, modelChoices, modelOf, modelPlaceholder, type Draft } from "@/features/roster/startOptions";
import { readPref, writePref } from "@/lib/prefs";
import { FormDialog, Icon, Logo, Select, useSubmit } from "@/ui";
import { handOver, type HandedOver } from "./api";
import { DEFAULT_TURNS, handoverWords, nextAgent, turnOptions } from "./handover";
import { t } from "@/lib/i18n";

export default function HandoverDialog({ machineId, paneId, session, agent, workspace, agents, onClose, onDone }: {
  machineId: string;
  paneId: string;
  session: string | null; // the conversation shown, null for the pane's live one
  agent: string; // the kind handing over
  workspace: string;
  agents: AgentKind[];
  onClose: () => void;
  onDone: (kind: string, made: HandedOver) => Promise<void>;
}) {
  const [kind, setKind] = useState(() => nextAgent(agents.map((a) => a.id), agent));
  const [draft, setDraft] = useState<Draft>(() => ({ ...EMPTY_DRAFT, ...choiceFor(kind, readPref(lastModelKey(kind), "")) }));
  const [turns, setTurns] = useState(DEFAULT_TURNS);
  const [more, setMore] = useState(false);
  const pick = (next: string) => {
    setKind(next);
    setDraft((d) => ({ ...d, ...choiceFor(next, readPref(lastModelKey(next), "")) }));
  };
  const submit = useSubmit(async () => {
    if (!kind) throw new Error("No agent to hand over to");
    const model = modelOf(kind, draft);
    if (hasModelFlag(kind)) writePref(lastModelKey(kind), model);
    const made = await handOver({ machine: machineId, pane: paneId, session, agent: kind, model: model || undefined, args: draft.args.trim() || undefined, turns });
    await onDone(kind, made);
  });
  const choices = modelChoices(kind);
  const typedModel = hasModelFlag(kind) && (choices.length === 0 || draft.choice === CUSTOM);
  const label = agents.find((a) => a.id === kind)?.label ?? kind;
  return (
    <FormDialog
      title={t("Continue with another agent")}
      icon="arrow-right"
      hint={`A new tab in ${workspace}: ${label} starts there with ${handoverWords(turns)} as its first prompt. This conversation stays as it is.`}
      onClose={onClose}
      submit={submit}
      submitLabel={`Hand over to ${label}`}
    >
      <label>
        {t("Agent")}
        <Select value={kind} onChange={pick} options={agents.map((a) => ({ value: a.id, label: a.label, icon: <Logo agent={a.id} /> }))} />
      </label>
      {hasModelFlag(kind) && choices.length > 0 && (
        <label>
          {t("Model")}
          <Select value={draft.choice} onChange={(choice) => setDraft({ ...draft, choice })} options={choices} />
        </label>
      )}
      {typedModel && (
        <label>
          {choices.length > 0 ? t("Model id") : t("Model")}
          <input value={draft.custom} placeholder={modelPlaceholder(kind)} spellCheck={false} onChange={(e) => setDraft({ ...draft, custom: e.target.value })} />
        </label>
      )}
      <label>
        {t("In full")}
        <Select value={turns} onChange={setTurns} options={turnOptions()} />
      </label>
      <div className="start-options">
        <button type="button" className="linkish start-options-toggle" aria-expanded={more} onClick={() => setMore(!more)}>
          <Icon name={more ? "chevron-down" : "chevron-right"} size={12} />
          Extra arguments{!more && draft.args.trim() ? t(" · set") : ""}
        </button>
        {more && (
          <label>
            {t("Extra arguments")}
            <input value={draft.args} placeholder={t("--permission-mode plan")} spellCheck={false} onChange={(e) => setDraft({ ...draft, args: e.target.value })} />
          </label>
        )}
      </div>
    </FormDialog>
  );
}

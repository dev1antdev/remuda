// Ask another agent for a second opinion on what the Changes lens shows: it starts in a new tab of the workspace,
// reviews without touching anything, and its comments come back under the diff's lines.

import { useState } from "react";
import { useRoster } from "@/features/roster/useRoster";
import { choiceFor, CUSTOM, EMPTY_DRAFT, hasModelFlag, lastModelKey, modelChoices, modelOf, modelPlaceholder, type Draft } from "@/features/roster/startOptions";
import { readPref, writePref } from "@/lib/prefs";
import { FormDialog, Logo, Select, useSubmit } from "@/ui";
import { t } from "@/lib/i18n";
import { askOpinion, type OpinionAsked } from "./api";
import { reviewerFor } from "./opinion";

const CLOSE_KEY = "opinion.close";

export function OpinionDialog({ machineId, paneId, dir, paneAgent, onClose, onAsked }: {
  machineId: string;
  paneId: string;
  dir: string;
  paneAgent: string; // the agent whose work is reviewed, "" for a shell pane
  onClose: () => void;
  onAsked: (asked: OpinionAsked, label: string) => void;
}) {
  const { agents } = useRoster();
  const kinds = agents.filter((agent) => agent.id !== "shell");
  const [kind, setKind] = useState(() => reviewerFor(paneAgent, kinds.map((agent) => agent.id)));
  const [draft, setDraft] = useState<Draft>(() => ({ ...EMPTY_DRAFT, ...choiceFor(kind, readPref(lastModelKey(kind), "")) }));
  const [close, setClose] = useState(() => readPref(CLOSE_KEY, true));
  const label = kinds.find((agent) => agent.id === kind)?.label ?? kind;
  const pick = (next: string) => {
    setKind(next);
    setDraft((d) => ({ ...d, ...choiceFor(next, readPref(lastModelKey(next), "")) }));
  };
  const submit = useSubmit(async () => {
    if (!kind) throw new Error(t("No agent to ask"));
    const model = modelOf(kind, draft);
    if (hasModelFlag(kind)) writePref(lastModelKey(kind), model);
    writePref(CLOSE_KEY, close);
    const asked = await askOpinion({ machine: machineId, pane: paneId, dir, agent: kind, model: model || undefined, close });
    onAsked(asked, label);
  });
  const choices = modelChoices(kind);
  const typedModel = hasModelFlag(kind) && (choices.length === 0 || draft.choice === CUSTOM);
  return (
    <FormDialog
      title={t("Second opinion")}
      icon="message"
      hint={t("{agent} starts in a new tab of this workspace and reviews the uncommitted work and the commits since your last look, without changing anything. Its comments appear under the lines of the diff; you add the ones you agree with to your review.", { agent: label })}
      onClose={onClose}
      submit={submit}
      submitLabel={t("Ask {agent}", { agent: label })}
    >
      <label>
        {t("Reviewer")}
        <Select value={kind} onChange={pick} options={kinds.map((agent) => ({ value: agent.id, label: agent.label, icon: <Logo agent={agent.id} /> }))} />
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
      <label className="check-row">
        <input type="checkbox" checked={close} onChange={(e) => setClose(e.target.checked)} />
        {t("Close its tab once it has answered")}
      </label>
    </FormDialog>
  );
}

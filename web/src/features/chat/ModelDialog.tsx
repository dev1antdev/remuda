// Switch the model of a Claude Code pane from the page: Opus, Sonnet, Haiku or a typed id, sent as its own /model
// command through the chat's submit, so the terminal shows the switch and the next answer's foot line names the model.

import { useState } from "react";
import { submitPrompt } from "@/features/roster/api";
import { choiceFor, CUSTOM, EMPTY_DRAFT, modelChoices, modelOf, modelPlaceholder, type Draft } from "@/features/roster/startOptions";
import { usePref } from "@/lib/prefs";
import { FormDialog, Select, useSubmit } from "@/ui";
import { modelCommand } from "./complete";
import { t } from "@/lib/i18n";

const KIND = "claude";

export default function ModelDialog({ machineId, paneId, current, onClose, onDone }: {
  machineId: string;
  paneId: string;
  current?: string; // the model the meter last saw the pane answer with
  onClose: () => void;
  onDone: (id: string) => void;
}) {
  // the model last switched to on this device, offered again
  const [last, setLast] = usePref<string>("chat.model", "");
  const [draft, setDraft] = useState<Draft>(() => ({ ...EMPTY_DRAFT, ...choiceFor(KIND, last || "opus") }));
  const choices = modelChoices(KIND).filter((c) => c.value !== "");
  const submit = useSubmit(async () => {
    const id = modelOf(KIND, draft);
    if (!id) throw new Error(t("Pick a model or type its id"));
    await submitPrompt(machineId, paneId, modelCommand(id));
    setLast(id);
    onDone(id);
  });
  return (
    <FormDialog
      title={t("Switch the model")}
      icon="sparkles"
      size="sm"
      hint={current ? t("Claude Code is told /model and switches at once, keeping the pick as its default for new sessions; it answers with {model} now.", { model: current }) : t("Claude Code is told /model and switches at once, keeping the pick as its default for new sessions; the next answer's foot line names the model.")}
      onClose={onClose}
      submit={submit}
      submitLabel={t("Switch")}
    >
      <label>
        {t("Model")}
        <Select value={draft.choice} onChange={(choice) => setDraft({ ...draft, choice })} options={choices} />
      </label>
      {draft.choice === CUSTOM && (
        <label>
          {t("Model id")}
          <input value={draft.custom} placeholder={modelPlaceholder(KIND)} spellCheck={false} autoFocus onChange={(e) => setDraft({ ...draft, custom: e.target.value })} />
        </label>
      )}
    </FormDialog>
  );
}

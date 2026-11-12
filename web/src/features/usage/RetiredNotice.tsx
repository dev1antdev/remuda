// The notice a form that edits a model shows while it sets a model or mode being retired: when it goes or that it
// went, what to use instead, why it applies when a machine's own settings make it so, and a button that puts the
// replacement in the model field when the field is not overridden by the arguments. Shared by every such form.

import { useRoster } from "@/features/roster/useRoster";
import { useSharing } from "@/features/roster/useSharing";
import { t } from "@/lib/i18n";
import { Notice } from "@/ui";
import { defaultsOf, failingOn, noticeWords, retiredFor } from "./retired";

export default function RetiredNotice({ agent, model, args = "", machine = "", onUse }: {
  agent: string;
  model: string;
  args?: string;
  machine?: string; // "" for wherever it may start
  onUse?: (model: string) => void;
}) {
  const { retirements, machines } = useRoster();
  const share = useSharing();
  const found = retiredFor(retirements, { agent, model, args, machine }, defaultsOf(machines));
  if (!found) return null;
  const now = Date.now();
  const r = found.retirement;
  const labels = found.machines.map((id) => {
    const m = machines.find((x) => x.id === id);
    return m ? share.machineLabel(m) : id;
  });
  const use = onUse && found.model !== "arguments" && (
    <button type="button" className="btn sm" onClick={() => onUse(r.replacement)}>
      {t("Use {model}", { model: r.replacement_name })}
    </button>
  );
  return (
    <Notice tone={failingOn(r, now) ? "error" : "warn"} icon="alert-triangle" className="retired-notice" action={use}>
      {noticeWords(found, now, labels, !!use)}
    </Notice>
  );
}

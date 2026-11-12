// DeniedNotice is the chat's quiet word over the composer on the calls Claude Code denied without asking, nobody being
// there to approve them: each call with what it named, Open in the terminal, and a dismissal that holds until another
// call is denied (denied.ts).

import { denialLine, type Denial } from "@/features/agents/deny";
import { t } from "@/lib/i18n";
import { usePref } from "@/lib/prefs";
import { Notice } from "@/ui";
import { deniedHead, deniedKey, lastDenial, shownDenials, showsDenied } from "./denied";

export function DeniedNotice({ machineId, session, denied, onTerminal }: {
  machineId: string;
  session: string; // the conversation shown, whose dismissal this is
  denied?: Denial[];
  onTerminal: () => void;
}) {
  const [dismissed, setDismissed] = usePref<string>(deniedKey(machineId, session), "");
  if (!denied || !showsDenied(denied, dismissed)) return null;
  const { shown, more } = shownDenials(denied);
  // the way to the terminal goes under the list, not beside it, so a phone gives the calls the notice's whole width
  return (
    <Notice tone="info" icon="alert-triangle" className="chat-notice chat-denied" onDismiss={() => setDismissed(lastDenial(denied))}>
      {deniedHead(denied.length)}
      <span className="chat-denied-list">
        {shown.map((call, i) => (
          <span key={call.tool_id || i} className="chat-denied-call">
            {denialLine(call)}
          </span>
        ))}
        {more > 0 && <span className="chat-denied-call">{t("+{n} more", { n: more })}</span>}
      </span>
      <button type="button" className="linkish" onClick={onTerminal}>
        {t("Open in the terminal")}
      </button>
    </Notice>
  );
}

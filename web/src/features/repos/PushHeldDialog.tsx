// A push the secret check holds back: what it found, each where it is and its rule in words with the line masked, or
// that it could not read the push, at all or in part; Push anyway asks once more before it goes, as Approve & open PR does.

import { actions } from "@/app/store";
import { ruleWords, skippedWords, unreadPaths, whereWords } from "@/features/review/secrets";
import { Dialog, Notice } from "@/ui";
import { t } from "@/lib/i18n";
import { heldWords, pushAnywayAsk } from "./outcome";
import type { HeldPush } from "./useRepoSync";

export function PushHeldDialog({ held, name, onClose, onPush }: { held: Pick<HeldPush, "outcome">; name: string; onClose: () => void; onPush: () => void }) {
  const { outcome } = held;
  const findings = outcome.scan?.findings ?? [];
  const outgoing = outcome.scan?.skipped?.filter((skip) => skip.in_commit); // what the push itself would send
  const notRead = skippedWords(outcome.reason === "check-incomplete" ? outgoing : outcome.scan?.skipped);
  const unread = unreadPaths(outgoing);
  const anyway = () =>
    actions.ask(pushAnywayAsk(outcome, name), t("Push anyway"), () => {
      onClose();
      onPush();
    });
  return (
    <Dialog title={t("Push {name}?", { name })} icon="alert-triangle" size="md" onClose={onClose} className="push-held">
      <Notice tone={outcome.reason === "secrets" ? "error" : "warn"}>{heldWords(outcome, name)}</Notice>
      {findings.length > 0 && (
        <ul className="secret-findings">
          {findings.map((finding, i) => (
            <li key={i} className="push-held-finding">
              <span className="secret-finding-where">{whereWords(finding)}</span>
              <span className="secret-finding-rule">{ruleWords(finding.rule)}</span>
              <code className="secret-finding-excerpt">{finding.excerpt}</code>
            </li>
          ))}
        </ul>
      )}
      {notRead && <p className="modal-hint">{notRead}</p>}
      {unread && <p className="modal-hint">{unread}</p>}
      {outcome.reason === "secrets" && <p className="modal-hint">{t("Take the secret out and commit again, or push it if it is no secret after all.")}</p>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button type="button" className="btn danger" onClick={anyway}>
          {t("Push anyway…")}
        </button>
      </div>
    </Dialog>
  );
}

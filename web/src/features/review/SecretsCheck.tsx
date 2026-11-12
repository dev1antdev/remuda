// The verdict dialog's secret check, under the choice of a commit or a pull request: where it stands, what it found —
// each finding a jump to its row in the diff — what it could not read, and a way to run it again when it failed.

import { t, tn } from "@/lib/i18n";
import { Icon } from "@/ui";
import type { IconName } from "@/ui/icons";
import type { VerdictAsk } from "./draft";
import { jumpable, ruleWords, skippedWords, stateWords, whereWords, type CheckState, type SecretFinding } from "./secrets";
import type { SecretCheck } from "./useSecretCheck";

const ICONS: Record<CheckState, IconName> = { off: "info", checking: "clock", clean: "circle-check", found: "alert-triangle", failed: "alert-triangle" };

export function SecretsCheck({ state, check, ask, listed, onJump }: {
  state: CheckState; // for the dialog's choice right now (stateFor)
  check: SecretCheck;
  ask: VerdictAsk;
  listed: string[]; // the files the lens lists, the ones a finding can be opened in
  onJump: (finding: SecretFinding) => void;
}) {
  if (state === "off") return null;
  const answered = state === "found" || state === "clean";
  const findings = state === "found" ? (check.scan?.findings ?? []) : [];
  const more = state === "found" ? (check.scan?.more ?? 0) : 0;
  const skipped = answered ? skippedWords(check.scan?.skipped) : "";
  return (
    <div className={`secret-check is-${state}`} role="status" aria-live="polite">
      <div className="secret-check-head">
        <Icon name={ICONS[state]} size={14} />
        <span className="secret-check-words">{state === "failed" ? t("The secret check could not run: {error}", { error: check.error }) : stateWords(state, answered ? check.scan : null, ask)}</span>
        {state === "failed" && (
          <button type="button" className="linkish" onClick={check.retry}>
            {t("Check again")}
          </button>
        )}
      </div>
      {findings.length > 0 && (
        <ul className="secret-findings">
          {findings.map((finding, index) => {
            const opens = jumpable(finding, listed);
            return (
              <li key={`${index}:${finding.file}:${finding.line}`}>
                <button type="button" className="linkish secret-finding" disabled={!opens} title={opens ? t("Show this line in the diff") : undefined} onClick={() => onJump(finding)}>
                  <span className="secret-finding-where">{whereWords(finding)}</span>
                  <span className="secret-finding-rule">{ruleWords(finding.rule)}</span>
                  <code className="secret-finding-excerpt">{finding.excerpt}</code>
                </button>
              </li>
            );
          })}
        </ul>
      )}
      {more > 0 && <p className="secret-check-note">{tn(more, "…and {n} more the list leaves out.", "…and {n} more the list leaves out.")}</p>}
      {state === "failed" && <p className="secret-check-note">{t("Nothing is known about secrets in this work: Send anyway asks you first.")}</p>}
      {skipped && <p className="secret-check-note">{skipped}</p>}
    </div>
  );
}

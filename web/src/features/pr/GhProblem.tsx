// A gh that cannot be used on the checkout's machine, in place of everything the panel would show: what is wrong, the
// command to run there, Try again.

import { copyText, openUrl } from "@/lib/host";
import { t } from "@/lib/i18n";
import { actions } from "@/app/store";
import { Icon, Tip } from "@/ui";
import type { Problem } from "./api";

export default function GhProblem({ problem, busy, onRetry }: { problem: Problem; busy: boolean; onRetry: () => void }) {
  const missing = problem.kind === "missing";
  const copy = () => void copyText(problem.command).then(() => actions.notify(t("Copied"), { brief: true }));
  return (
    <div className="pr-problem">
      <Icon name="git-pull-request" size={20} />
      <b>{missing ? t("The GitHub CLI is not installed on {machine}", { machine: problem.machine }) : t("The GitHub CLI is not signed in on {machine}", { machine: problem.machine })}</b>
      <p className="modal-hint">{missing ? t("The panel reads and writes GitHub through gh, as you. Install it there, sign in, then try again.") : t("The panel uses gh's sign-in. Run this in a terminal on that machine, then try again.")}</p>
      <div className="pr-command">
        <code>{problem.command}</code>
        <Tip text={t("Copy")}>
          <button type="button" className="icon-btn sm" aria-label={t("Copy")} onClick={copy}>
            <Icon name="copy" size={14} />
          </button>
        </Tip>
      </div>
      <div className="pr-problem-actions">
        {missing && (
          <button type="button" className="btn" onClick={() => openUrl("https://cli.github.com")}>
            <Icon name="external-link" size={14} /> {t("Get the GitHub CLI")}
          </button>
        )}
        <button type="button" className="btn primary" disabled={busy} onClick={onRetry}>
          {busy ? t("Checking…") : t("Try again")}
        </button>
      </div>
    </div>
  );
}

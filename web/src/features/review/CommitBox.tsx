// The commit box at the foot of the file list, as better-changes has it: a message that grows to eight rows, Commit staged
// (or Commit all, the listed files staged first) with Amend behind its chevron, and the secret check standing before it
// as it stands before Approve & commit — findings hold the button back, Commit anyway… asks first. A failure keeps the
// message and shows git's own words.

import { useEffect, useState } from "react";
import { actions } from "@/app/store";
import { t } from "@/lib/i18n";
import { usePref } from "@/lib/prefs";
import { errMsg } from "@/lib/text";
import { Icon, Menu, Notice } from "@/ui";
import { commitWork, fetchLastCommit, type Outgoing } from "./api";
import { boxRows, commitAnywayAsk, commitHint, commitLabel, committedWords, leaveAmend, loadAmend, whyNotCommit, type CommitMode } from "./commit";
import { commitAllPaths, type ChangeRow, type GroupId } from "./groups";
import type { SecretFinding } from "./secrets";
import { heldBack } from "./secrets";
import { SecretsCheck } from "./SecretsCheck";
import { stateFor, type SecretCheck } from "./useSecretCheck";

type Amend = { head: string | null; loading: boolean; loaded: string | null };
const NO_AMEND: Amend = { head: null, loading: false, loaded: null };

export function CommitBox({ machineId, dir, groups, busy, outgoing, subfolder, look, check, listed, onJump, onCommitted }: {
  machineId: string;
  dir: string;
  groups: Record<GroupId, ChangeRow[]>;
  busy: string; // the agent at work in the checkout, "" when none
  outgoing: Outgoing | undefined;
  subfolder: boolean; // the lens's folder is inside the repository, not its top
  look: number; // a new look of the folder: the secret check reads the work again
  check: SecretCheck;
  listed: string[];
  onJump: (finding: SecretFinding) => void;
  onCommitted: () => void;
}) {
  const [message, setMessage] = usePref(`review.commit:${machineId}|${dir}`, "");
  const [mode, setMode] = useState<CommitMode>("commit");
  const [amend, setAmend] = useState<Amend>(NO_AMEND);
  const [working, setWorking] = useState(false);
  const [failure, setFailure] = useState("");
  const toStage = commitAllPaths(groups);
  const state = { message, mode, staged: groups.staged.length, toStage: toStage.length, conflicts: groups.conflict.length, busy, amendHead: amend.head, loadingAmend: amend.loading, working };
  const whyNot = whyNotCommit(state);
  const { ask } = check;
  useEffect(() => {
    ask(whyNot ? null : "commit", true);
  }, [whyNot, look, ask]);
  useEffect(() => () => ask(null), [ask]);
  const checked = whyNot ? "off" : stateFor(check, "commit");
  const held = heldBack(checked);
  const checking = checked === "checking";
  const amendPushed = !!(mode === "amend" && amend.head && outgoing?.upstream && !outgoing.commits.some((commit) => commit.sha === amend.head));
  const hint = commitHint(state, amendPushed, subfolder);

  const choose = (next: CommitMode) => {
    setFailure("");
    if (next === mode) return;
    setMode(next);
    if (next === "commit") {
      setMessage((current) => leaveAmend(current, amend.loaded));
      setAmend(NO_AMEND);
      return;
    }
    setAmend({ head: null, loading: true, loaded: null });
    fetchLastCommit(machineId, dir).then(
      (last) => {
        let loaded: string | null = null;
        setMessage((current) => {
          loaded = loadAmend(current, last.message);
          return loaded ?? current;
        });
        setAmend({ head: last.head, loading: false, loaded });
      },
      (error: unknown) => {
        setAmend(NO_AMEND);
        setFailure(t("Cannot prepare the amend: {error}", { error: errMsg(error) }));
      },
    );
  };
  const commit = async (anyway: boolean) => {
    if (whyNot || working || checking || (held && !anyway)) return;
    setWorking(true);
    setFailure("");
    const sent = message;
    try {
      const made = await commitWork(machineId, dir, { message: sent, stage: mode === "commit" && state.staged === 0 ? toStage : undefined, amend: mode === "amend", head: amend.head ?? undefined });
      setMessage((current) => (current === sent ? "" : current));
      setMode("commit");
      setAmend(NO_AMEND);
      check.clear();
      actions.notify(committedWords(made.short, made.subject, mode === "amend"), { brief: true });
      onCommitted();
    } catch (error) {
      setFailure(errMsg(error));
    } finally {
      setWorking(false);
    }
  };
  const label = checking && !whyNot ? t("Checking for secrets…") : commitLabel(state);
  return (
    <section className="commit-box" aria-label={t("Commit")}>
      <textarea
        className="commit-message"
        rows={boxRows(message)}
        value={message}
        placeholder={mode === "amend" ? t("The latest commit's message") : t("Commit message")}
        aria-label={t("Commit message")}
        onChange={(event) => setMessage(event.target.value)}
      />
      <div className="commit-actions">
        <span className="commit-split">
          <button type="button" className={`btn${mode === "amend" ? "" : " primary"} commit-go`} disabled={!!whyNot || working || checking || held} onClick={() => void commit(false)}>
            <Icon name={mode === "amend" ? "edit" : "git-commit"} size={14} /> {label}
          </button>
          <Menu
            tip={t("Commit or amend")}
            label={t("Commit or amend")}
            trigger={
              <button type="button" className={`btn${mode === "amend" ? "" : " primary"} commit-mode`} aria-label={t("Commit or amend")} disabled={working}>
                <Icon name="chevron-down" size={14} />
              </button>
            }
            items={[
              { label: t("Commit"), icon: "git-commit", checked: mode === "commit", hint: t("a new commit of what is staged"), onSelect: () => choose("commit") },
              { label: t("Amend"), icon: "edit", checked: mode === "amend", hint: t("rewrite the latest commit"), onSelect: () => choose("amend") },
            ]}
          />
        </span>
        {held && (
          <button type="button" className="btn danger commit-anyway" disabled={working || !!whyNot} onClick={() => actions.ask(commitAnywayAsk(checked, check.scan), t("Commit anyway"), () => void commit(true))}>
            {t("Commit anyway…")}
          </button>
        )}
      </div>
      {(whyNot || hint) && <p className={`commit-hint${mode === "amend" && !whyNot ? " is-amend" : ""}`}>{whyNot || hint}</p>}
      {checked !== "off" && checked !== "clean" && <SecretsCheck state={checked} check={check} ask="commit" listed={listed} onJump={onJump} />}
      {failure && (
        <Notice tone="error" onDismiss={() => setFailure("")}>
          {failure}
        </Notice>
      )}
    </section>
  );
}

// The Overview tab: the agents on the pull request (a click opens one), its reviewers with how each stands and Ask again
// for anyone not already asked, its description as Markdown — each a group that folds, remembered on this device.

import { useState, type ReactNode } from "react";
import { actions, type PRAt } from "@/app/store";
import { useRoster } from "@/features/roster/useRoster";
import { useSharing } from "@/features/roster/useSharing";
import { errMsg } from "@/lib/text";
import { usePref } from "@/lib/prefs";
import { t } from "@/lib/i18n";
import { Icon, Logo, Tip, type IconName } from "@/ui";
import { Markdown } from "@/ui/Markdown";
import { VENDOR_NAME } from "@/ui/brand";
import { askAgain, type Detail } from "./api";
import { agentsOn } from "./tree";

// Group is a part of a tab that folds, its head the label, a count and what it says on the right
export function Group({ id, label, count, aside, children }: { id: string; label: string; count?: number; aside?: ReactNode; children: ReactNode }) {
  const [closed, setClosed] = usePref<string[]>("pr.closed", []);
  const open = !closed.includes(id);
  return (
    <section className="pr-group">
      <button type="button" className="pr-group-head" aria-expanded={open} onClick={() => setClosed(open ? [...closed, id] : closed.filter((x) => x !== id))}>
        <Icon name={open ? "chevron-down" : "chevron-right"} size={12} />
        <span>{label}</span>
        {count !== undefined && <span className="badge quiet">{count}</span>}
        <span className="pr-group-aside">{aside}</span>
      </button>
      {open && <div className="pr-group-body">{children}</div>}
    </section>
  );
}

const REVIEW: Record<string, { label: () => string; icon: IconName; tone: string }> = {
  APPROVED: { label: () => t("Approved"), icon: "circle-check", tone: "good" },
  CHANGES_REQUESTED: { label: () => t("Changes requested"), icon: "circle-x", tone: "bad" },
  COMMENTED: { label: () => t("Commented"), icon: "message", tone: "dim" },
  DISMISSED: { label: () => t("Dismissed"), icon: "circle-slash", tone: "dim" },
  REQUESTED: { label: () => t("Review requested"), icon: "circle-dot", tone: "warn" },
};

// DOT is an agent's state as the sidebar's dot draws it
const DOT: Record<string, string> = { working: "run", blocked: "input", done: "done" };

// stateWords is an agent's state as the line says it
const stateWords = (state: string) => ({ working: t("working"), blocked: t("waiting for you"), done: t("finished"), idle: t("idle") })[state] ?? state;

const DECISION: Record<string, { label: () => string; tone: string }> = {
  APPROVED: { label: () => t("Approved"), tone: "good" },
  CHANGES_REQUESTED: { label: () => t("Changes requested"), tone: "bad" },
  REVIEW_REQUIRED: { label: () => t("Review required"), tone: "warn" },
};

export default function Overview({ at, pr, onChanged, onClose }: { at: PRAt; pr: Detail; onChanged: () => void; onClose: () => void }) {
  const { machines } = useRoster();
  const share = useSharing(); // under the screen-sharing mode a hidden folder's workspace goes by its neutral name
  const agents = agentsOn(machines, at.machineId, { url: pr.url, head: pr.head });
  const [asking, setAsking] = useState("");
  const again = async (login: string) => {
    setAsking(login);
    try {
      await askAgain({ machine: at.machineId, repo: pr.repository, number: pr.number }, login);
      actions.notify(t("Asked {login} to review again", { login }), { brief: true });
      onChanged();
    } catch (e) {
      actions.notify(errMsg(e), { error: true });
    } finally {
      setAsking("");
    }
  };
  const decision = DECISION[pr.review_decision];
  return (
    <>
      {agents.length > 0 && (
        <Group id="agents" label={t("Agents")} count={agents.length}>
          {agents.map((a) => (
            <Tip key={`${a.machineId}|${a.paneId}`} text={t("Go to this agent")}>
              <button
                type="button"
                className="pr-line"
                onClick={() => {
                  onClose();
                  actions.openPane(a.machineId, a.paneId);
                }}
              >
                <span className={`dot dot-${DOT[a.state] ?? "idle"}`} />
                <Logo agent={a.agent} />
                <span className="pr-line-name">
                  {VENDOR_NAME[a.agent] ?? a.agent} · {share.folderHidden(a.machineId, a.folder) ? share.folderName(a.machineId, a.folder, a.workspace) : a.workspace}
                </span>
                <span className="pr-line-aside">{stateWords(a.state)}</span>
              </button>
            </Tip>
          ))}
        </Group>
      )}
      <Group id="reviewers" label={t("Reviewers")} count={pr.reviewers.length} aside={decision && <span className={`is-${decision.tone}`}>{decision.label()}</span>}>
        {pr.reviewers_error && <p className="pr-empty">{t("Could not read the reviewers: {error}", { error: pr.reviewers_error })}</p>}
        {!pr.reviewers_error && pr.reviewers.length === 0 && <p className="pr-empty">{t("No reviewers yet.")}</p>}
        {pr.reviewers.map((r) => {
          const look = REVIEW[r.state] ?? REVIEW.REQUESTED!;
          return (
            <div key={r.login} className="pr-line">
              <Icon name={look.icon} size={14} className={`is-${look.tone}`} />
              <span className="pr-line-name">{r.login}</span>
              <span className="pr-line-aside">{look.label()}</span>
              {pr.state === "OPEN" && r.state !== "REQUESTED" ? (
                <Tip text={t("Ask {login} to review again", { login: r.login })}>
                  <button type="button" className="icon-btn sm" aria-label={t("Ask {login} to review again", { login: r.login })} disabled={!!asking} onClick={() => void again(r.login)}>
                    <Icon name="refresh-cw" size={12} />
                  </button>
                </Tip>
              ) : (
                <span className="pr-line-room" />
              )}
            </div>
          );
        })}
      </Group>
      <Group id="description" label={t("Description")}>
        <div className="pr-markdown">{pr.body.trim() ? <Markdown text={pr.body} onOpenFile={(file) => {
                onClose();
                actions.openFile(at.machineId, at.paneId ?? "", file, at.dir);
              }} /> : <p className="pr-empty">{t("No description.")}</p>}</div>
      </Group>
    </>
  );
}

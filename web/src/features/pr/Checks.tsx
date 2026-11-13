// The Checks tab: the head commit's checks by workflow run, those that need a look first and a failed run open; a check
// unfolds to its job's steps, a failed job to the end of its failed steps' output (the last 60 KB read on the machine,
// the last 8 KB shown), and a failed check goes to an agent with Fix with agent — the failure hand-off, which tells the
// agent the command that reads the log and a short end of it.

import { useState } from "react";
import { actions, type PRAt } from "@/app/store";
import { useAgentTargets } from "@/features/agents/useAgentTargets";
import { targetWords, type AgentTarget } from "@/features/agents/targets";
import { useRoster } from "@/features/roster/useRoster";
import { useSharing } from "@/features/roster/useSharing";
import { openUrl } from "@/lib/host";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { took } from "@/lib/time";
import { t } from "@/lib/i18n";
import { Icon, Menu, Notice, Tip, type MenuItem } from "@/ui";
import { fetchJob, fetchLog, fixCheck, type Check, type Detail } from "./api";
import { checkGroups, kindIcon, kindOf, kindTone, kindWords, problemWords, type CheckKind } from "./model";
import { agentsOn } from "./tree";

const seconds = (from?: string, to?: string) => (from && to ? Math.max(0, (Date.parse(to) - Date.parse(from)) / 1000) : null);

function KindIcon({ kind }: { kind: CheckKind }) {
  return <Icon name={kindIcon(kind)} size={14} className={`is-${kindTone(kind)}${kind === "running" ? " is-spinning" : ""}`} aria-label={kindWords(kind)} />;
}

export default function Checks({ at, pr }: { at: PRAt; pr: Detail }) {
  const groups = checkGroups(pr.checks);
  const [shut, setShut] = useState<Record<string, boolean>>({});
  if (groups.length === 0) return <p className="pr-empty">{t("No checks reported on the head commit.")}</p>;
  return (
    <>
      {groups.map((g) => {
        const open = !(shut[g.key] ?? g.worst !== "failure");
        return (
          <section key={g.key} className="pr-group">
            <button type="button" className="pr-group-head" aria-expanded={open} onClick={() => setShut({ ...shut, [g.key]: open })}>
              <Icon name={open ? "chevron-down" : "chevron-right"} size={12} />
              <KindIcon kind={g.worst} />
              <span>{g.name}</span>
              <span className="badge quiet">{g.checks.length}</span>
            </button>
            {open && (
              <div className="pr-group-body">
                {g.checks.map((c, i) => (
                  <CheckRow key={`${c.name}:${c.job ?? i}`} at={at} pr={pr} check={c} />
                ))}
              </div>
            )}
          </section>
        );
      })}
    </>
  );
}

function CheckRow({ at, pr, check }: { at: PRAt; pr: Detail; check: Check }) {
  const [open, setOpen] = useState(false);
  const kind = kindOf(check);
  const time = seconds(check.started_at, check.completed_at);
  return (
    <div className="pr-check">
      <div className="pr-check-row">
        <button type="button" className="pr-check-name" aria-expanded={open} onClick={() => setOpen(!open)}>
          <Icon name={open ? "chevron-down" : "chevron-right"} size={12} />
          <KindIcon kind={kind} />
          <span>{check.name}</span>
        </button>
        {kind === "failure" ? <FixWithAgent at={at} pr={pr} check={check} /> : <span className="pr-when">{time !== null ? took(time) : kindWords(kind)}</span>}
      </div>
      {open && <CheckBody at={at} pr={pr} check={check} />}
    </div>
  );
}

function CheckBody({ at, pr, check }: { at: PRAt; pr: Detail; check: Check }) {
  const job = useQuery(() => (check.job ? fetchJob(at.machineId, pr.repository, check.job) : Promise.resolve(null)), [at.machineId, pr.repository, check.job]);
  const [showLog, setShowLog] = useState(false);
  return (
    <div className="pr-check-body">
      {check.url?.startsWith("https://") && (
        <button type="button" className="linkish" onClick={() => openUrl(check.url!)}>
          <Icon name="external-link" size={12} /> {t("View on GitHub")}
        </button>
      )}
      {!check.job && <p className="pr-empty">{t("Its steps are not GitHub Actions'; its page has them.")}</p>}
      {check.job && job.loading && <p className="pr-empty">{t("Reading the job's steps…")}</p>}
      {job.error && <Notice tone="error">{job.error}</Notice>}
      {job.data?.problem && <Notice tone="error">{problemWords(job.data.problem)}</Notice>}
      {job.data?.steps?.map((s) => {
        const time = seconds(s.started_at, s.completed_at);
        return (
          <div key={s.number} className="pr-step">
            <KindIcon kind={kindOf(s)} />
            <span>{s.name}</span>
            <span className="pr-when">{time !== null ? took(time) : ""}</span>
          </div>
        );
      })}
      {check.job && kindOf(check) === "failure" && (showLog ? <FailedLog at={at} pr={pr} job={check.job} /> : (
        <button type="button" className="linkish" onClick={() => setShowLog(true)}>
          {t("Failed step output")}
        </button>
      ))}
    </div>
  );
}

function FailedLog({ at, pr, job }: { at: PRAt; pr: Detail; job: number }) {
  const log = useQuery(() => fetchLog(at.machineId, pr.repository, job), [at.machineId, pr.repository, job]);
  if (log.loading) return <p className="pr-empty">{t("Reading the failed steps' output…")}</p>;
  if (log.error) return <Notice tone="error">{log.error}</Notice>;
  if (log.data?.problem) return <Notice tone="error">{problemWords(log.data.problem)}</Notice>;
  return (
    <>
      {log.data?.cut && <p className="pr-empty">{t("The end of the output:")}</p>}
      <pre className="pr-log">{log.data?.text || t("No failed step printed anything.")}</pre>
    </>
  );
}

// FixWithAgent hands the failed check to an agent: the ones on the pull request first, then the rest
function FixWithAgent({ at, pr, check }: { at: PRAt; pr: Detail; check: Check }) {
  const targets = useAgentTargets();
  const { machines } = useRoster();
  const share = useSharing(); // under the screen-sharing mode a hidden folder's agent goes by its neutral name
  const masked = (a: AgentTarget) => (share.folderHidden(a.machineId, a.folder ?? "") ? share.folderName(a.machineId, a.folder ?? "", a.sessionName) : null);
  const onPR = new Set(agentsOn(machines, at.machineId, { url: pr.url, head: pr.head }).map((a) => `${a.machineId}|${a.paneId}`));
  const ordered = [...targets].sort((a, b) => Number(onPR.has(`${b.machineId}|${b.paneId}`)) - Number(onPR.has(`${a.machineId}|${a.paneId}`)));
  const hand = async (machineId: string, paneId: string, name: string) => {
    actions.notify(t("Reading {check} for the agent…", { check: check.name }), { brief: true });
    try {
      await fixCheck(machineId, paneId, pr.url, check.name);
      actions.notify(t("{check} is with {agent}", { check: check.name, agent: name }));
    } catch (e) {
      actions.notify(errMsg(e), { error: true });
    }
  };
  if (ordered.length === 0)
    return (
      <Tip text={t("No agent is running to hand it to.")}>
        <button type="button" className="btn sm" disabled>
          {t("Fix with agent")}
        </button>
      </Tip>
    );
  const items: MenuItem[] = ordered.map((a) => ({ label: targetWords(a, masked(a)), hint: onPR.has(`${a.machineId}|${a.paneId}`) ? t("on this pull request") : share.redact(a.machineLabel), onSelect: () => void hand(a.machineId, a.paneId, masked(a) ?? a.sessionName) }));
  return <Menu label={t("Fix with agent")} items={items} trigger={<button type="button" className="btn sm">{t("Fix with agent")} <Icon name="chevron-down" size={12} /></button>} />;
}

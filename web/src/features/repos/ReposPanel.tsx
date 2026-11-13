// The Repositories place: every repository the fleet knows — the ones its workspaces stand on, each once, and those a
// folder the person scans holds — grouped by machine and then by the folder they sit in, after Manygit. Read every 30 s
// while it is on screen and after every action; Fetch all fetches eight at a time.

import { useState } from "react";
import { useCurrent } from "@/app/current";
import type { Machine } from "@/features/roster/api";
import { useRoster } from "@/features/roster/useRoster";
import { useSharing } from "@/features/roster/useSharing";
import { folderOf } from "@/features/roster/workspaces";
import { useNow } from "@/lib/clock";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { t } from "@/lib/i18n";
import { Dialog, Icon, Menu, Notice, Tip } from "@/ui";
import { listRepos, setScan, type Listed, type Repo } from "./api";
import { runWords } from "./outcome";
import { PushHeldDialog } from "./PushHeldDialog";
import { repoMenu } from "./repoMenu";
import { RepoRow, RepoRowLoading } from "./RepoRow";
import { fetchable, groupsOf, keyOf, machineWords, scanWords } from "./repos";
import { ScanDialog } from "./ScanDialog";
import { useRepoSync } from "./useRepoSync";

export const READ_EVERY = 30_000;

export default function ReposPanel({ onClose }: { onClose: () => void }) {
  const { machines } = useRoster();
  const query = useQuery(() => listRepos(), [], { every: READ_EVERY });
  const sync = useRepoSync(query.mutate, query.reload);
  const [scanning, setScanning] = useState<Listed | null>(null);
  const [reading, setReading] = useState(false);
  const share = useSharing();
  const now = useNow(60_000);
  const listed = query.data?.machines ?? null;
  const anything = (listed ?? []).some((m) => m.repos.some(fetchable));
  const nameOf = (machineId: string, repo: Repo) => share.folderName(machineId, repo.path, repo.name);
  const readAgain = async () => {
    setReading(true);
    try {
      const fresh = await listRepos(true);
      query.mutate(() => fresh);
    } catch (e) {
      sync.say(errMsg(e));
    } finally {
      setReading(false);
    }
  };
  const controls = (
    <>
      <Tip text={t("Read every repository again")}>
        <button type="button" className="icon-btn repos-read-again" aria-label={t("Read every repository again")} disabled={reading} onClick={() => void readAgain()}>
          <Icon name="refresh-cw" size={16} />
        </button>
      </Tip>
      <button type="button" className="btn sm repos-fetch-all" disabled={!!sync.run || !anything} onClick={() => listed && void sync.fetchAll(listed)}>
        <Icon name="refresh-cw" size={14} />
        <span>{sync.run ? runWords(sync.run) : t("Fetch all")}</span>
      </button>
    </>
  );
  return (
    <Dialog title={t("Repositories")} icon="folder-git" size="lg" onClose={onClose} className="repos-panel" controls={controls}>
      <p className="modal-hint">{t("Every repository your workspaces stand on, on every machine, and those in a folder you scan. A row's menu fetches, pulls (a fast-forward, never a merge) and pushes (never forced); nothing waits for a login.")}</p>
      {query.error && <Notice tone="error">{query.error}</Notice>}
      <div className="repos-list">
        {listed
          ? listed.map((m) => <MachineRepos key={m.id} listed={m} machine={machines.find((rm) => rm.id === m.id)} sync={sync} now={now} nameOf={nameOf} onScan={() => setScanning(m)} reload={query.reload} />)
          : machines.filter((m) => m.state === "connected").map((m) => <MachineLoading key={m.id} label={share.machineLabel(m)} />)}
        {listed && listed.length === 0 && <p className="repos-empty">{t("No machine to read: add one from the sidebar's +.")}</p>}
      </div>
      {scanning && <ScanDialog machineId={scanning.id} machineName={scanning.label} scan={scanning.scan} onClose={() => setScanning(null)} onSaved={query.reload} />}
      {sync.held && <PushHeldDialog held={sync.held} name={nameOf(sync.held.machineId, sync.held.repo)} onClose={sync.dropHeld} onPush={() => sync.held && void sync.act(sync.held.machineId, sync.held.repo, "push", true)} />}
    </Dialog>
  );
}

type Sync = ReturnType<typeof useRepoSync>;

function MachineRepos({ listed, machine, sync, now, nameOf, onScan, reload }: { listed: Listed; machine?: Machine; sync: Sync; now: number; nameOf: (machineId: string, repo: Repo) => string; onScan: () => void; reload: () => void }) {
  const share = useSharing();
  const current = useCurrent();
  const here = current && current.machine.id === listed.id ? folderOf(current.session) : "";
  const label = machine ? share.machineLabel(machine) : listed.label;
  const scanItems = [
    { label: listed.scan ? t("Change the scan…") : t("Scan a folder for repositories…"), icon: "folder" as const, onSelect: onScan },
    ...(listed.scan ? [{ label: t("Stop scanning {folder}", { folder: share.redact(listed.scan_where || listed.scan.folder) }), icon: "x" as const, onSelect: () => void setScan(listed.id, "", 0).then(reload, (e) => sync.say(errMsg(e))) }] : []),
  ];
  return (
    <section className="repos-machine" aria-label={label}>
      <div className="repos-machine-head">
        <Icon name="monitor" size={14} />
        <span className="repos-machine-name">{label}</span>
        <span className={`repos-machine-words${listed.away || listed.error ? " is-away" : ""}`}>{share.redact(machineWords(listed))}</span>
        {listed.scan && <span className="repos-machine-scan">{share.redact(scanWords(listed))}</span>}
        <span className="repo-fill" />
        <Menu
          label={t("Scan on {machine}", { machine: label })}
          tip={t("The folder this machine scans for repositories")}
          items={scanItems}
          trigger={
            <button type="button" className="icon-btn sm" aria-label={t("The folder this machine scans for repositories")}>
              <Icon name="more" size={14} />
            </button>
          }
        />
      </div>
      {groupsOf(listed.repos).map((group) => (
        <div className="repos-group" key={group.parent}>
          <div className="repos-group-head">{share.redact(group.parent)}/</div>
          {group.repos.map((repo) => {
            const key = keyOf(listed.id, repo.path);
            const name = nameOf(listed.id, repo);
            return (
              <RepoRow
                key={repo.path}
                repo={repo}
                name={name}
                parent={share.redact(group.parent)}
                here={repo.path === here}
                busy={sync.busy.has(key)}
                fetchFailed={sync.failed.get(key) ?? ""}
                now={now}
                menu={() => repoMenu(listed.id, machine, repo, (action) => void sync.act(listed.id, repo, action))}
              />
            );
          })}
        </div>
      ))}
      {!listed.away && !listed.error && listed.repos.length === 0 && <p className="repos-empty">{listed.scan ? t("No repository here: none of its workspaces stands on one, and the scan found none.") : t("No repository here: none of its workspaces stands on one. Scan a folder from ⋯ to list the others.")}</p>}
    </section>
  );
}

// MachineLoading is a machine's part while its first read is under way, shaped like the rows to come
function MachineLoading({ label }: { label: string }) {
  return (
    <section className="repos-machine" aria-busy="true">
      <div className="repos-machine-head">
        <Icon name="monitor" size={14} />
        <span className="repos-machine-name">{label}</span>
        <span className="repos-machine-words">…</span>
      </div>
      <div className="repos-group">
        <RepoRowLoading />
        <RepoRowLoading />
        <RepoRowLoading />
      </div>
    </section>
  );
}

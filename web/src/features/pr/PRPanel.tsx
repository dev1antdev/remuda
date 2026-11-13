// The pull request panel: the checkout's branch's pull requests and the repository's under two tabs, Branch and Repo,
// one pull request in full under them, and a foot naming the branch and how old the read is. Everything comes from gh on
// the checkout's machine; a gh missing or signed out there takes the place of everything, with what to run.

import { useEffect, useState } from "react";
import { app, type PRAt } from "@/app/store";
import { folderOf, useCurrent } from "@/app/current";
import { useNow } from "@/lib/clock";
import { usePref } from "@/lib/prefs";
import { useQuery } from "@/lib/query";
import { ago } from "@/lib/time";
import { t } from "@/lib/i18n";
import { Dialog, Icon, Tip, useDebounced } from "@/ui";
import { fetchBranch, fetchSearch, type Card, type Problem } from "./api";
import { BranchList, RepoList, type RepoAsk } from "./PRLists";
import PRDetail from "./PRDetail";
import GhProblem from "./GhProblem";
import { searchText } from "./model";
import { prFromURL } from "./links";

export default function PRPanel({ onClose }: { onClose: () => void }) {
  const asked = app.use((s) => s.prAt);
  const current = useCurrent();
  const at: PRAt | null = asked ?? (current ? { machineId: current.machine.id, dir: folderOf(current), paneId: current.pane.id, n: 0 } : null);
  if (!at || !at.dir)
    return (
      <Dialog title={t("Pull requests")} icon="git-pull-request" size="lg" className="pr-panel" onClose={onClose}>
        <p className="modal-hint">{t("Open a workspace on a checkout first: the panel shows its branch's pull requests and its repository's.")}</p>
      </Dialog>
    );
  return <PanelBody key={`${at.machineId}|${at.dir}|${at.n}`} at={at} onClose={onClose} />;
}

export type Opened = { repo: string; number: number };
type View = "branch" | "repo";

function PanelBody({ at, onClose }: { at: PRAt; onClose: () => void }) {
  const [view, setView] = usePref<View>("pr.view", "branch");
  const fromLink = at.url ? prFromURL(at.url) : null;
  const [open, setOpen] = useState<Opened | null>(fromLink);
  const [stamp, setStamp] = useState(0); // a refresh reads every list and the pull request open again
  const branch = useQuery(() => fetchBranch(at.machineId, at.dir), [at.machineId, at.dir, stamp], { every: 60_000, keep: true });
  const checkout = branch.data?.checkout;
  const [remote, setRemote] = useState("");
  const repo = remote || (fromLink?.repo ?? "") || checkout?.repo || "";
  const [ask, setAsk] = useState<RepoAsk>({ text: "", scope: "all", state: "open" });
  const settled = useDebounced(searchText(ask.text), 450);
  const [repoSeen, setRepoSeen] = useState(false); // the Repo list is read while it shows, and once for its count
  const search = useQuery(() => (repo ? fetchSearch(at.machineId, repo, { text: settled, scope: ask.scope, state: ask.state }) : Promise.resolve(undefined)), [at.machineId, repo, settled, ask.scope, ask.state, stamp], {
    enabled: view === "repo" || !repoSeen,
    keep: true,
  });
  useEffect(() => {
    if (search.data) setRepoSeen(true);
  }, [search.data]);
  const items = branch.data?.items ?? [];
  const sole = view === "branch" && !open && items.length === 1 ? items[0]! : null;
  const shown: (Opened & { sole?: boolean }) | null = open ?? (sole ? { repo: sole.repository, number: sole.number, sole: true } : null);
  const problem: Problem | undefined = branch.data?.problem ?? search.data?.problem;
  const now = useNow(30_000);
  const readAt = useReadAt(branch.data, search.data);
  const busy = branch.fetching || search.fetching;
  const pick = (card: Card) => setOpen({ repo: card.repository, number: card.number });
  const switchTo = (next: View) => {
    setOpen(null);
    setView(next);
  };
  const repoCount = search.data && !search.data.problem ? (search.data.total ?? `${search.data.items.length}${Object.keys(search.data.cursors).length ? "+" : ""}`) : null;
  return (
    <Dialog
      title={t("Pull requests")}
      icon="git-pull-request"
      size="lg"
      className="pr-panel"
      onClose={onClose}
      controls={
        <Tip text={t("Read GitHub again")}>
          <button type="button" className={`icon-btn${busy ? " is-busy" : ""}`} aria-label={t("Read GitHub again")} onClick={() => setStamp((n) => n + 1)}>
            <Icon name="refresh-cw" size={16} />
          </button>
        </Tip>
      }
    >
      <div className="pr-views" role="tablist" aria-label={t("Which pull requests")}>
        <ViewTab label={t("Branch")} count={branch.data && !branch.data.problem ? items.length : null} on={view === "branch"} onPick={() => switchTo("branch")} />
        <ViewTab label={t("Repo")} count={repoCount} on={view === "repo"} onPick={() => switchTo("repo")} />
      </div>
      <div className="pr-body">
        {problem ? (
          <GhProblem problem={problem} busy={busy} onRetry={() => setStamp((n) => n + 1)} />
        ) : shown ? (
          <PRDetail key={`${shown.repo}#${shown.number}`} at={at} repo={shown.repo} number={shown.number} stamp={stamp} onBack={shown.sole ? undefined : () => setOpen(null)} onClose={onClose} />
        ) : view === "branch" ? (
          <BranchList query={branch} branch={checkout?.branch ?? ""} onOpen={pick} />
        ) : (
          <RepoList query={search} repo={repo} remotes={checkout?.remotes ?? []} ask={ask} onAsk={setAsk} onRemote={setRemote} machineId={at.machineId} onOpen={pick} />
        )}
      </div>
      <footer className="pr-foot">
        <span className="pr-foot-branch">{checkout?.branch || (branch.loading ? t("Finding the repository…") : t("No branch checked out"))}</span>
        <span className="pr-foot-when">{busy ? t("Updating…") : readAt ? t("Updated {when}", { when: ago(readAt, now) }) : ""}</span>
      </footer>
    </Dialog>
  );
}

// ViewTab is Branch or Repo with its count: "…" while it is still being read, never 0
function ViewTab({ label, count, on, onPick }: { label: string; count: number | string | null; on: boolean; onPick: () => void }) {
  return (
    <button type="button" role="tab" aria-selected={on} className={`btn sm${on ? " on" : ""}`} onClick={onPick}>
      {label} <span className="badge quiet">{count === null ? "…" : count}</span>
    </button>
  );
}

// useReadAt is when the lists on screen were last read
function useReadAt(...answers: unknown[]): number | null {
  const [at, setAt] = useState<number | null>(null);
  useEffect(() => {
    if (answers.some(Boolean)) setAt(Date.now());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, answers);
  return at;
}

// The GitHub issues panel, after Orca's issues mod: "GitHub · <machine> · owner/repo" over a repository picker, the
// table of issues a search finds and one issue's viewer in its place. Everything is read and written through gh on the
// machine of one of the repository's checkouts, as the person; a write only at their press.

import { useEffect, useMemo, useRef, useState } from "react";
import { useCurrent } from "@/app/current";
import { actions, app } from "@/app/store";
import { useRoster } from "@/features/roster/useRoster";
import { openUrl } from "@/lib/host";
import { usePref } from "@/lib/prefs";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { Dialog, Icon, Tip } from "@/ui";
import { t } from "@/lib/i18n";
import { assignIssue, closeIssue, fetchIssues, fetchRepos, reopenIssue, troubleOf, type Item, type List, type Repo } from "./api";
import IssueTable from "./IssueTable";
import IssueViewer, { type Viewed } from "./IssueViewer";
import type { RowActions } from "./IssueRow";
import { openWorkspace, paneOf, startFrom } from "./links";
import { issuePrompt, issueURL } from "./prompt";
import { OPEN_QUERY } from "./query";
import RepoPicker from "./RepoPicker";
import { PICKED_KEY, RECENT_KEY, remember, type Picked, type Recent } from "./repos";
import { TroubleNote } from "./pieces";
import { whyNoRepo } from "./words";

export default function IssuesPanel({ onClose }: { onClose: () => void }) {
  const { machines } = useRoster();
  const current = useCurrent();
  const asked = app.use((s) => s.issuesAt);
  const [picked, setPicked] = usePref<Picked>(PICKED_KEY, null);
  const [recent, setRecent] = usePref<Recent[]>(RECENT_KEY, []);
  // where the panel looks: the folder it was opened on, else the repository picked, else the pane on screen
  const follow = asked ?? (current ? { machineId: current.machine.id, dir: current.session.dir ?? current.pane.cwd } : null);
  const choice = asked ? null : picked;
  const machineId = choice?.machine ?? follow?.machineId ?? machines.find((m) => m.state === "connected")?.id ?? "local";
  const machine = machines.find((m) => m.id === machineId);
  const machineLabel = (id: string) => machines.find((m) => m.id === id)?.label ?? id;
  const followDir = follow && follow.machineId === machineId ? follow.dir : ""; // the pane's folder, asked about on its own machine only
  const repos = useQuery(() => fetchRepos(machineId, followDir || undefined), [machineId, followDir], { keep: true });
  const following = repos.data?.here?.repo ?? "";
  const repo = choice?.repo ?? following;
  const entry: Repo | undefined = repos.data?.repos.find((r) => r.repo.toLowerCase() === repo.toLowerCase());
  const [query, setQuery] = useState(OPEN_QUERY);
  const [viewing, setViewing] = useState<Viewed[]>([]);
  const [busy, setBusy] = useState<number | null>(null);
  const fresh = useRef(false);
  const list = useQuery(
    () => {
      const ask = fresh.current;
      fresh.current = false;
      return repo ? fetchIssues(machineId, repo, query, ask) : Promise.resolve(undefined);
    },
    [machineId, repo, query],
    { every: 60_000, enabled: !!repo && viewing.length === 0, keep: true },
  );
  useEffect(() => setViewing([]), [machineId, repo]);
  const trouble = troubleOf(list.data ?? undefined);
  const shown = list.data && !trouble ? (list.data as List) : null;
  const again = () => {
    fresh.current = true;
    list.reload();
  };

  const pick = (next: Picked) => {
    if (asked) actions.openIssues(null);
    setPicked(next);
    if (next) setRecent(remember(recent, next));
  };
  const openSession = (number: number, repoName = repo) => (repoName.toLowerCase() === repo.toLowerCase() ? openWorkspace(machine, entry, number) : null);
  const start = (number: number, title: string, repoName = repo) => {
    const from = repoName.toLowerCase() === repo.toLowerCase() ? startFrom(machine, entry) : null;
    if (!from || !machine) {
      actions.notify(t("No workspace on {machine} is a checkout of {repo}: open one there, then Start.", { machine: machineLabel(machineId), repo: repoName }), { error: true });
      return;
    }
    onClose();
    actions.openDialog({ kind: "worktree", machineId, session: from, issue: number, repo: repoName, title, prompt: issuePrompt({ repo: repoName, number, title }) });
  };
  const open = (number: number, repoName = repo) => {
    const session = openSession(number, repoName);
    if (!session) return;
    onClose();
    actions.selectPane(machineId, paneOf(session));
  };
  const rowWrite = async (item: Item, run: () => Promise<unknown>, after?: () => void) => {
    setBusy(item.number);
    try {
      await run();
      after?.();
    } catch (e) {
      actions.notify(errMsg(e), { error: true });
    } finally {
      setBusy(null);
      again();
    }
  };
  const act: RowActions = useMemo(
    () => ({
      view: (item) => setViewing([{ repo, number: item.number }]),
      start: (item) => start(item.number, item.title),
      open: (item) => open(item.number),
      assign: (item, me) => void rowWrite(item, () => assignIssue(machineId, repo, item.number, me)),
      close: (item, reason) => void rowWrite(item, () => closeIssue(machineId, repo, item.number, reason), () => actions.setIssueClosed({ machineId, repo, number: item.number, reason })),
      reopen: (item) => void rowWrite(item, () => reopenIssue(machineId, repo, item.number)),
      task: (item) => {
        onClose();
        actions.openDialog({ kind: "import-issue", url: item.url, machineId });
      },
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [machineId, repo, entry, machine],
  );
  const at = viewing[viewing.length - 1];
  const picker = <RepoPicker repo={repo} picked={choice} machine={machineId} following={following} recent={recent} repos={repos.data?.repos ?? []} machineLabel={machineLabel} onPick={pick} />;
  const go = (r: string, n: number) => {
    if (at && r.toLowerCase() === at.repo.toLowerCase() && n === at.number) return;
    setViewing([...viewing, { repo: r, number: n }]);
  };
  const why = !repo && repos.data ? whyNoRepo(repos.data.here, machineLabel(machineId)) : null;
  const crumbs = ["GitHub", machineLabel(machineId), repo || t("no repository")];

  return (
    <Dialog
      title={
        <span className="issue-crumbs">
          {crumbs.map((c, i) => (
            <span key={i} className={i === crumbs.length - 1 ? "issue-crumb is-last" : "issue-crumb"}>
              {c}
            </span>
          ))}
        </span>
      }
      icon="circle-dot"
      size="lg"
      className="issues-panel"
      onClose={onClose}
      controls={
        <>
          <Tip text={t("Read again from GitHub")}>
            <button type="button" className="icon-btn" aria-label={t("Read again from GitHub")} disabled={!repo || !!at} onClick={again}>
              <Icon name="refresh-cw" size={14} className={list.fetching && !at ? "spin" : undefined} />
            </button>
          </Tip>
          <Tip text={t("Open on GitHub")}>
            <button type="button" className="icon-btn" aria-label={t("Open on GitHub")} disabled={!repo} onClick={() => openUrl(at ? issueURL(at.repo, at.number) : `https://github.com/${repo}/issues`)}>
              <Icon name="external-link" size={14} />
            </button>
          </Tip>
        </>
      }
    >
      {at ? (
        <IssueViewer
          key={`${at.repo}#${at.number}`}
          machineId={machineId}
          folder={at.repo.toLowerCase() === repo.toLowerCase() ? (entry?.folder ?? "") : ""}
          at={at}
          back={viewing.length > 1 ? `#${viewing[viewing.length - 2]!.number}` : t("Issues")}
          onBack={() => setViewing(viewing.slice(0, -1))}
          onGo={go}
          onStart={(issue) => start(issue.number, issue.title, at.repo)}
          onOpen={(issue) => open(issue.number, at.repo)}
          linked={(r, n) => !!openSession(n, r)}
          onChanged={() => (fresh.current = true)}
        />
      ) : repo ? (
        <>
          {trouble && <TroubleNote trouble={trouble} onRetry={again} />}
          {list.error && <TroubleNote trouble={{ kind: "failed", text: list.error }} onRetry={again} />}
          <IssueTable
            query={query}
            onQuery={(q) => setQuery(q.trim() || OPEN_QUERY)}
            list={shown}
            loading={!shown && !trouble && !list.error}
            repo={repo}
            isLinked={(n) => !!openSession(n)}
            busy={busy}
            act={act}
            picker={picker}
          />
        </>
      ) : (
        <div className="issue-list">
          <div className="issue-bar">{picker}</div>
          {repos.error ? (
            <TroubleNote trouble={{ kind: "failed", text: repos.error }} onRetry={repos.reload} />
          ) : why ? (
            <TroubleNote trouble={{ kind: "failed", text: why.text, run: why.run }} onRetry={repos.reload} />
          ) : (
            <p className="issue-empty">{t("Looking for the repository…")}</p>
          )}
        </div>
      )}
    </Dialog>
  );
}

// The Changes lens: a workspace folder's uncommitted work in git's groups — conflicts, staged, unstaged, untracked — with
// the files moved since the last look marked, the commits since that look and the ones not on the remote, a commit box,
// and the branch's whole work against its base as another scope; the diffs in one column or two with the changed words
// marked, walked by keys, with comments on diff lines gathered into a draft that goes to the pane's agent.

import { useCallback, useMemo, useRef, useState } from "react";
import { actions, app } from "@/app/store";
import { useAgentTargets } from "@/features/agents/useAgentTargets";
import { fsText } from "@/features/files/api";
import { useRoster } from "@/features/roster/useRoster";
import { isTouch, usePref } from "@/lib/prefs";
import { useQuery } from "@/lib/query";
import { basename } from "@/lib/text";
import { joinPath } from "@/lib/pathname";
import { ago } from "@/lib/time";
import { Notice, useTimedNotice } from "@/ui";
import { VENDOR_NAME } from "@/ui/brand";
import { fetchBase, fetchReview, fetchReviewDiff, type DiffFrom, type Review as ReviewData, type ReviewFile } from "./api";
import { BaseFiles, baseOrder } from "./BaseFiles";
import { ChangeGroups } from "./ChangeGroups";
import { CommitBox } from "./CommitBox";
import { ConflictView } from "./ConflictView";
import { FileRow, type RowLook } from "./Diff";
import { useDraft } from "./useDraft";
import { rowFor, rowsOf, type GroupId } from "./groups";
import { Note } from "./Hunk";
import { useReviewKeys } from "./keys";
import { layoutGroups, visibleKeys } from "./layout";
import { OpinionBlock } from "./OpinionBlock";
import { OpinionDialog } from "./OpinionDialog";
import { subWords, type OpinionComment } from "./opinion";
import { OutgoingCommits } from "./OutgoingCommits";
import { ReviewFoot } from "./ReviewFoot";
import { ReviewHead } from "./ReviewHead";
import { CollisionNotice } from "./CollisionNotice";
import { SinceCommits } from "./SinceCommits";
import { busyAgent, isSubfolder, type Scope } from "./scope";
import type { SecretFinding } from "./secrets";
import { useLensWrites } from "./useLensWrites";
import { useOpinion } from "./useOpinion";
import { useSecretCheck } from "./useSecretCheck";
import { t } from "@/lib/i18n";

export type { DraftComment } from "./draft";

const NO_OPINION: OpinionComment[] = [];
const NO_SECRETS: SecretFinding[] = [];

const toggled = <T,>(set: ReadonlySet<T>, item: T): Set<T> => {
  const next = new Set(set);
  if (!next.delete(item)) next.add(item);
  return next;
};

export default function Review({ machineId, paneId, dir, agent, onSeen, active = true, editor = true }: {
  machineId: string;
  paneId: string;
  dir: string;
  agent: string; // "" for a shell pane
  onSeen?: () => void;
  active?: boolean; // the lens on screen: its keys listen
  editor?: boolean; // the machine has code-server for the per-file button (a Windows PC has none)
}) {
  const agents = useAgentTargets();
  const roster = useRoster();
  const onOpenFile = (path: string, line?: number) => actions.openFile(machineId, paneId, { path, line });
  const [split, setSplit] = usePref("review.split", !isTouch);
  const [tree, setTree] = usePref("review.tree", false);
  const [pinned, setPinned] = usePref(`review.base:${machineId}|${dir}`, "");
  const [scope, setScope] = useState<Scope>("uncommitted");
  const [closed, setClosed] = useState<ReadonlySet<string>>(new Set());
  const [unfolded, setUnfolded] = useState<ReadonlySet<GroupId>>(new Set());
  const rootRef = useRef<HTMLDivElement>(null);
  const [actionError, setActionError] = useState("");
  const [openFile, setOpenFile] = useState<string | null>(null);
  const [look, setLook] = useState(0); // a new look remounts the rows, which re-read their diffs
  const undone = app.use((s) => s.undone); // an undo from the page changed a folder: this one is looked at again
  const [undoneSeen, setUndoneSeen] = useState(undone);
  if (undone !== undoneSeen) {
    setUndoneSeen(undone);
    setLook((count) => count + 1);
  }
  const [notice, showNotice] = useTimedNotice(5000);

  const { data: review, error: fetchError } = useQuery<ReviewData>(() => fetchReview(machineId, dir), [machineId, dir, look]);
  const { data: base, error: baseError } = useQuery(() => (scope === "base" ? fetchBase(machineId, dir, pinned) : Promise.resolve(undefined)), [scope, machineId, dir, pinned, look], { enabled: scope === "base", keep: true });
  const error = actionError || fetchError || review?.error || "";
  const lookAgain = () => setLook((count) => count + 1);
  const loadDiff = useCallback((path: string, from: DiffFrom) => fetchReviewDiff(machineId, dir, path, from), [machineId, dir]); // stable: every row's query keys on it
  const loadSource = useCallback((path: string) => fsText(machineId, `${dir}/${path}`), [machineId, dir]); // the file itself, for the lines around a hunk

  const files = useMemo(() => review?.files ?? [], [review]);
  const commits = review?.commits ?? [];
  const lastLook = review?.baseline ? new Date(review.baseline.at) : null;
  const groups = useMemo(() => rowsOf(files), [files]);
  const layouts = useMemo(() => layoutGroups(groups, { tree, closed, looked: !!lastLook, unfolded }), [groups, tree, closed, lastLook, unfolded]);
  const listed = useMemo(() => files.map((file) => file.path), [files]);
  const draft = useDraft({ machineId, dir, agent, review: review ?? null, files: listed, onError: setActionError, onDone: showNotice });
  const second = useOpinion({ machineId, dir, look, active, files: listed, addComment: draft.addComment, onError: setActionError });
  const check = useSecretCheck(machineId, dir);
  const commitCheck = useSecretCheck(machineId, dir);
  const marks = check.scan ? check.marks : commitCheck.marks;
  const machine = roster.machines.find((candidate) => candidate.id === machineId);
  const busy = busyAgent(machine, review?.repo, dir, (kind) => VENDOR_NAME[kind] ?? kind) || review?.busy || "";
  const writes = useLensWrites({ machineId, dir, onDone: () => (lookAgain(), onSeen?.()), onError: setActionError });
  const whyNotWrite = busy ? t("{agent} is working here: wait until it rests.", { agent: busy }) : "";

  const keys = useReviewKeys({
    active,
    root: rootRef,
    files: scope === "base" ? baseOrder(base, tree, closed) : visibleKeys(layouts),
    openFile,
    setOpenFile,
    onOpen: (key, line) => onOpenFile(joinPath(dir, key.slice(key.indexOf(":") + 1)), line),
    toggleSplit: () => setSplit((on) => !on),
  });
  // a finding clicked in a secret check: the dialog gives way to its row, opened, in view and focused
  const showFinding = (finding: SecretFinding) => {
    draft.closeVerdict();
    const row = rowFor(groups, finding.file);
    if (!row) return;
    setScope("uncommitted");
    if (lastLook && !row.file.fresh) setUnfolded((shown) => new Set([...shown, row.group]));
    keys.reveal(row.key, finding.line);
  };

  if (review === null && !error) return <div className="empty">{t("Looking at the folder…")}</div>;
  if (review && !review.repo) return <div className="empty">{t("{folder} is not a git checkout — nothing to review.", { folder: basename(dir) })}</div>;
  const lastVerdict = review?.baseline?.verdict;
  const lastLookText = !lastLook ? t("not reviewed yet") : lastVerdict === "approved" ? t("approved {ago}", { ago: ago(lastLook) }) : lastVerdict === "changes" ? t("rejected {ago}", { ago: ago(lastLook) }) + (review?.baseline?.ready ? t(" · ready again") : "") : t("since you looked {ago}", { ago: ago(lastLook) });
  const fileRow = (file: ReviewFile, rowLook: RowLook) => {
    const key = rowLook.key ?? file.path;
    return (
      <FileRow
        key={`${key}|${look}`}
        file={file}
        look={rowLook}
        open={openFile === key}
        split={split}
        onToggle={() => setOpenFile(openFile === key ? null : key)}
        load={loadDiff}
        loadSource={loadSource}
        onLoaded={keys.loaded}
        canComment={draft.canComment}
        draft={draft.forFile(file.path)}
        onComment={draft.addComment}
        onRemove={draft.removeComment}
        opinion={second.perFile.get(file.path) ?? NO_OPINION}
        reviewer={second.reviewer}
        onAdopt={second.adopt}
        onDismissOpinion={second.dismissOne}
        secrets={marks.get(file.path) ?? NO_SECRETS}
        onOpen={(line) => onOpenFile(joinPath(dir, file.path), line)}
        onCode={editor ? () => actions.openInCode(machineId, dir, joinPath(dir, file.path)) : undefined}
      />
    );
  };

  return (
    <div className="review" ref={rootRef}>
      <ReviewHead
        review={review}
        scope={scope}
        onScope={(next) => (setScope(next), setOpenFile(null))}
        base={base ?? null}
        pinned={pinned}
        onPin={setPinned}
        lastLookText={lastLookText}
        opinionWords={subWords(second.opinion)}
        split={split}
        onSplit={() => setSplit((on) => !on)}
        tree={tree}
        onTree={() => setTree((on) => !on)}
        onLookAgain={lookAgain}
        opinionRunning={second.running}
        onAskOpinion={() => second.setAsking(true)}
        onUndo={() => actions.openDialog({ kind: "undo-turn", machineId, dir })}
        marking={draft.marking}
        onMark={() => void draft.markSilently(() => (setOpenFile(null), lookAgain(), onSeen?.()))}
      />
      {error && (
        <Notice tone="error" onDismiss={() => setActionError("")}>
          {error}
        </Notice>
      )}
      {notice && <Notice tone="success">{notice}</Notice>}
      <div className="review-body">
        <CollisionNotice machineId={machineId} dir={dir} />
        {second.opinion && (
          <OpinionBlock opinion={second.opinion} reviewer={second.reviewer} asking={second.askingPerson} apart={second.apart} moved={second.moved} onAdopt={second.adopt} onAdoptAll={second.adoptAll} onDismiss={second.dismissOne} onDismissAll={second.dismissAll} />
        )}
        {scope === "base" ? (
          <BaseFiles view={base ?? null} error={baseError ?? ""} tree={tree} closed={closed} onFolder={(key) => setClosed((shut) => toggled(shut, key))} fileRow={fileRow} />
        ) : (
          <>
            <SinceCommits commits={commits} total={review?.commits_total} />
            <ChangeGroups
              layouts={layouts}
              tree={tree}
              whyNot={whyNotWrite}
              writing={writes.writing}
              closed={closed}
              onFolder={(key) => setClosed((shut) => toggled(shut, key))}
              onUnfold={(group) => setUnfolded((shown) => toggled(shown, group))}
              onBulk={writes.onBulk}
              onAction={writes.onAction}
              fileRow={(row, rowLook) => fileRow(row.file, rowLook)}
              conflictBody={(row) => <ConflictView machineId={machineId} dir={dir} path={row.file.path} split={split} />}
            />
            {files.length === 0 && <div className="empty">{commits.length > 0 ? t("Everything here is committed.") : lastLook ? t("Nothing has changed here since you looked.") : t("Nothing is uncommitted here.")}</div>}
            {review && (files.length > 0 || (review.outgoing?.total ?? 0) > 0) && (
              <CommitBox
                machineId={machineId}
                dir={dir}
                groups={groups}
                busy={busy}
                outgoing={review.outgoing}
                subfolder={isSubfolder(review.repo, dir)}
                look={look}
                check={commitCheck}
                listed={listed}
                onJump={showFinding}
                onCommitted={() => (setOpenFile(null), lookAgain(), onSeen?.())}
              />
            )}
            {review?.outgoing && <OutgoingCommits machineId={machineId} dir={dir} outgoing={review.outgoing} fileRow={fileRow} />}
          </>
        )}
        {review && draft.orphans.length > 0 && (
          <div className="review-orphans">
            <Notice tone="hint">{t("Comments on files no longer in this list — still in the draft:")}</Notice>
            {draft.orphans.map((comment) => (
              <Note key={comment.id} comment={comment} where={comment.line > 0 ? `${comment.file}:${comment.line}` : comment.file} onRemove={draft.removeComment} />
            ))}
          </div>
        )}
      </div>
      <ReviewFoot draft={draft} review={review ?? null} agent={agent} agents={agents} machineId={machineId} paneId={paneId} dir={dir} check={check} listed={listed} onJump={showFinding} onError={setActionError} onSent={() => (check.clear(), setOpenFile(null), lookAgain(), onSeen?.())} />
      {second.asking && (
        <OpinionDialog
          machineId={machineId}
          paneId={paneId}
          dir={dir}
          paneAgent={agent}
          onClose={() => second.setAsking(false)}
          onAsked={(asked, label) => {
            second.setAsking(false);
            second.mutate(() => asked.opinion);
            if (asked.agent_error) setActionError(t("{agent} did not start: {error}", { agent: label, error: asked.agent_error }));
            else showNotice(t("{agent} is reviewing in a new tab; its comments appear here when it answers.", { agent: label }));
          }}
        />
      )}
    </div>
  );
}

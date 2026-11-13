// The New worktree dialog, after Orca's Create worktree: a project of the machine, a source — a branch (checked out
// already, local, or only on origin), an open issue, a pull request, or a new branch from a base — and the Details it
// fills, with a sentence saying exactly what Create will do. Create (⌘↵ / Ctrl+↵) asks the server, which works the plan
// out again from a fresh read: herdr makes the worktree, git fetches and tracks around it, the setup script and the agent
// start as the project says.

import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { usePref, writePref } from "@/lib/prefs";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { Dialog, Icon, Notice, Select } from "@/ui";
import { t } from "@/lib/i18n";
import type { AgentKind, Machine, Session } from "../api";
import { useSharing } from "../useSharing";
import { defaultAgent, suggestBranch, useStart } from "../dialogs";
import { DEFAULTS, setupLine } from "../project/project";
import ProjectSettingsDialog from "../project/ProjectSettingsDialog";
import { createFrom, fetchLists, fetchOrigin, fetchSource, lookUp, type Found, type Made } from "./api";
import { Details, noteOf } from "./Details";
import { createWords, fill, fromPR, pickOf, planOf, type Pick } from "./plan";
import { SourceList, SourceTabs } from "./SourcePicker";
import { baseChoices, emptyWords, items, jumpFor, MODES, repositoriesOf, type Lists, type Mode } from "./sources";

const isMac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform);

export default function NewWorktreeDialog({ machine, session, agents, onMade, onClose }: {
  machine: Machine;
  session: Session; // a workspace of the repository it opens on
  agents: AgentKind[];
  onMade: (made: Made) => void;
  onClose: () => void;
}) {
  const share = useSharing();
  const repos = useMemo(() => repositoriesOf(machine, session.folder || session.dir || ""), [machine, session]);
  const [root, setRoot] = useState(repos[0]?.root ?? session.folder ?? session.dir ?? "");
  const repo = repos.find((r) => r.root === root);
  const answer = useQuery(() => fetchSource(machine.id, root), [machine.id, root]);
  const src = answer.data?.source ?? null;
  const settings = answer.data?.settings ?? {};
  const lists = useQuery<Lists | null>(() => (src?.github ? fetchLists(machine.id, src.github) : Promise.resolve(null)), [machine.id, src?.github ?? ""]);
  const [found, setFound] = useState<Found[]>([]);
  const allLists = useMemo(() => withFound(lists.data, found), [lists.data, found]);

  const [savedMode, setSavedMode] = usePref<Mode>("worktree.mode", "new");
  const [mode, setMode] = useState<Mode>(MODES.includes(savedMode) ? savedMode : "new");
  const [query, setQuery] = useState("");
  const [at, setAt] = useState(0);
  const [pick, setPick] = useState<Pick>({ kind: "new" });
  const [branch, setBranch] = useState(suggestBranch);
  const [name, setName] = useState("");
  const [base, setBase] = useState("");
  const [run, setRun] = useState<boolean | null>(true);
  const [fetching, setFetching] = useState(false);
  const [looking, setLooking] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState(false);
  const done = useRef(false);
  const field = useRef<HTMLInputElement>(null);
  const filled = useRef(""); // the first prompt as the template last filled it: an edit of the person's is kept
  const [rememberedAgent] = usePref("worktree.agent", "");
  const start = useStart(defaultAgent(agents, rememberedAgent || "claude"));
  const agentTouched = useRef(false);

  const issueBranch = (number: number, title: string) => fill(settings.issue_branch || DEFAULTS.issue_branch, number, title, src?.github ?? "", "");
  const rows = items(mode, query, src, allLists, answer.data?.home ?? "", (i) => issueBranch(i.number, i.title));
  const where = { home: answer.data?.home ?? "", root: answer.data?.root ?? "", location: settings.location ?? "" };
  const plan = src ? planOf(src, branch, base, pick, where) : null;
  const note = noteOf(plan, fromPR(pick, branch), looking, answer.loading);
  const setup = answer.data?.setup ? setupLine(src?.team?.script, settings.setup) : "";

  // a repository read: its base and what its project starts a worktree with
  useEffect(() => {
    if (!answer.data) return;
    setBase(answer.data.settings.base || answer.data.source.base);
    setRun(answer.data.settings.setup_when === "ask" ? null : answer.data.settings.setup_when !== "skip");
    const own = answer.data.settings.agent;
    if (own && !agentTouched.current) start.pickAgent(own);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [answer.data]);

  // the first row of a list chosen as it comes in, so the Details are never empty while there is something to pick
  useEffect(() => {
    if (mode !== "new" && rows[at] && (pick.kind === "new" || !rows.some((r) => sameSource(r, pick)))) choose(at);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rows.length, mode]);

  const prefill = (next: Pick, nextBranch: string, title: string) => {
    const template = next.kind === "issue" ? settings.issue_prompt || DEFAULTS.issue_prompt : next.kind === "pr" ? settings.pr_prompt || DEFAULTS.pr_prompt : "";
    const text = template && src?.github && next.number ? fill(template, next.number, title, src.github, nextBranch) : "";
    if (start.draft.prompt === filled.current || !start.draft.prompt.trim()) start.setDraft((d) => ({ ...d, prompt: text }));
    filled.current = text;
  };

  const choose = (i: number) => {
    const row = rows[i];
    setAt(i);
    if (!row) return;
    if (row.kind === "lookup") return void resolve(row.want, row.number);
    const filledIn = pickOf(row, issueBranch);
    setPick(filledIn.pick);
    setBranch(filledIn.branch);
    setName(filledIn.name);
    prefill(filledIn.pick, filledIn.branch, filledIn.name);
  };

  const resolve = async (want: "issue" | "pr", number: number) => {
    if (!src?.github || looking) return;
    setLooking(true);
    setPick({ kind: want, number });
    try {
      const hit = await lookUp(machine.id, src.github, number);
      setFound((list) => [hit, ...list.filter((f) => f.number !== hit.number)]);
      switchMode(hit.kind, `#${number}`);
    } catch (e) {
      setError(errMsg(e));
    } finally {
      setLooking(false);
    }
  };

  const switchMode = (next: Mode, text = "") => {
    setMode(next);
    setSavedMode(next);
    setQuery(text);
    setAt(0);
    setError("");
    if (next === "new") {
      setPick({ kind: "new" });
      setBranch(suggestBranch());
      setName("");
      prefill({ kind: "new" }, "", "");
    } else setPick({ kind: "new" });
    queueMicrotask(() => field.current?.focus());
  };

  const typed = (text: string) => {
    const jump = pasted(query, text) ? jumpFor(mode, text, allLists) : null; // a reference pasted whole, never one typed digit by digit
    if (jump) return switchMode(jump, text);
    setQuery(text);
    setAt(0);
    const first = items(mode, text, src, allLists, answer.data?.home ?? "", (i) => issueBranch(i.number, i.title))[0];
    if (!first) return;
    if (first.kind === "lookup") return setPick({ kind: first.want, number: first.number });
    const filledIn = pickOf(first, issueBranch);
    setPick(filledIn.pick);
    setBranch(filledIn.branch);
    setName(filledIn.name);
    prefill(filledIn.pick, filledIn.branch, filledIn.name);
  };

  // the branch typed on the New tab: a pasted issue or pull request goes to its tab, as it does from the search
  const typedBranch = (text: string) => {
    const jump = mode === "new" && pasted(branch, text) ? jumpFor(mode, text, allLists) : null;
    if (jump) return switchMode(jump, text.trim());
    setBranch(text);
  };

  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if ((e.key === "ArrowDown" || e.key === "ArrowUp") && rows.length) {
      e.preventDefault();
      choose((at + (e.key === "ArrowDown" ? 1 : rows.length - 1)) % rows.length);
    }
    const row = rows[at];
    if (e.key === "Enter" && !e.metaKey && !e.ctrlKey && row?.kind === "lookup") {
      e.preventDefault();
      void resolve(row.want, row.number);
    }
  };

  const fetchNow = async () => {
    if (!src || fetching) return;
    setFetching(true);
    setError("");
    try {
      await fetchOrigin(machine.id, src.root);
      answer.reload();
      lists.reload();
    } catch (e) {
      setError(errMsg(e));
    } finally {
      setFetching(false);
    }
  };

  const ready = !!plan && plan.action !== "error" && !busy && !!repo && !looking;
  const go = async () => {
    if (done.current || !ready || !src || !repo) return;
    if (setup && run === null) return setError(t("Choose whether to run the setup script."));
    done.current = true;
    setBusy(true);
    setError("");
    try {
      const extras = start.options();
      writePref("worktree.agent", start.agent);
      const made = await createFrom({ machine: machine.id, workspace: repo.workspace, root, branch: branch.trim(), base, name: name.trim(), agent: start.agent, pick, setup: setup ? (run ? "run" : "skip") : "", ...extras });
      onMade(made);
    } catch (e) {
      done.current = false;
      setError(errMsg(e));
      setBusy(false);
    }
  };

  const onDialogKey = (e: KeyboardEvent) => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      if (!e.repeat) void go();
    }
  };

  const repoLabel = (root: string) => share.redact(root.split(/[\\/]/).filter(Boolean).pop() ?? root);
  return (
    <Dialog
      as="form"
      size="md"
      className="new-worktree"
      title={t("New worktree")}
      icon="git-branch"
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        void go();
      }}
    >
      <div onKeyDown={onDialogKey} className="nw-body">
        <label>
          {t("Project")}
          <Select value={root} onChange={setRoot} options={repos.map((r) => ({ value: r.root, label: repoLabel(r.root), hint: share.machineHidden(machine.id) ? undefined : share.redact(r.root), icon: <Icon name="folder" size={14} /> }))} />
        </label>
        <SourceTabs mode={mode} onMode={(m) => switchMode(m)} bases={src ? baseChoices(src) : [base]} base={base} onBase={setBase} showBase={plan?.action === "new" && plan.fromBase} fetching={fetching} onFetch={() => void fetchNow()} />
        {mode !== "new" && <SourceList mode={mode} query={query} onQuery={typed} rows={rows} at={at} onPick={choose} empty={emptyWords(mode, src, answer.loading, answer.error, allLists)} field={field} onKey={onKey} />}
        {answer.error && mode === "new" && <Notice tone="error">{answer.error}</Notice>}
        <Details
          branch={branch}
          onBranch={typedBranch}
          note={note.note}
          error={note.error}
          name={name}
          onName={setName}
          namePlaceholder={t("Follows the branch")}
          agents={agents}
          agent={start.agent}
          onAgent={(a) => {
            agentTouched.current = true;
            start.pickAgent(a);
          }}
          draft={start.draft}
          onDraft={start.setDraft}
          onRecipe={start.applyRecipe}
          machine={machine.id}
          setup={setup}
          run={run}
          onRun={(v) => {
            setRun(v);
            setError("");
          }}
        />
        {error && <Notice tone="error">{error}</Notice>}
      </div>
      <div className="modal-actions nw-foot">
        <span className="nw-keys">{mode === "new" ? t("↵ create · esc close") : t("↑↓ choose · ↵ create · esc close")}</span>
        <button type="button" className="linkish" disabled={!src} onClick={() => setEditing(true)}>
          {t("Project settings…")}
        </button>
        <button type="button" className="btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button type="submit" className="btn primary nw-create" disabled={!ready}>
          {busy && <span className="spinner" aria-hidden="true" />}
          {createWords(plan, busy)}
          <kbd>{isMac ? "⌘↵" : "Ctrl+↵"}</kbd>
        </button>
      </div>
      {editing && <ProjectSettingsDialog machineId={machine.id} root={root} onClose={() => setEditing(false)} onSaved={() => answer.reload()} />}
    </Dialog>
  );
}

// pasted says the field took more than one character at once: a paste, or a link dropped in
const pasted = (before: string, after: string): boolean => after.length - before.length > 1;

// withFound adds the numbers looked up to the open lists, so their rows say what they are from then on
function withFound(lists: Lists | null, found: Found[]): Lists | null {
  if (!lists || found.length === 0) return lists;
  const issues = [...found.filter((f) => f.kind === "issue" && !lists.issues.some((i) => i.number === f.number)).map((f) => ({ number: f.number, title: f.title })), ...lists.issues];
  const prs = [...found.filter((f) => f.kind === "pr" && !lists.prs.some((p) => p.number === f.number)).map((f) => ({ number: f.number, title: f.title, head: f.head ?? "", cross_repo: f.cross_repo })), ...lists.prs];
  return { ...lists, issues, prs };
}

// sameSource says the row is what the Details were filled from
function sameSource(row: ReturnType<typeof items>[number], pick: Pick): boolean {
  if (row.kind === "issue" || row.kind === "pr") return pick.kind === row.kind && pick.number === row.number;
  if (row.kind === "lookup") return false;
  return pick.kind === "branch" && pick.branch === row.branch;
}

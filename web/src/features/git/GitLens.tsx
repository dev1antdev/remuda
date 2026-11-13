// The Git lens, after git-graph: a pane's checkout as a commit graph — every branch a lane in the theme's terminal
// colours, the labels, HEAD a ring on a tinted row, the uncommitted changes on top — a commit's details under it, and
// git's writes behind the menus, each asking first. A viewer reads the graph and opens no menu.

import { useCallback, useMemo, useState } from "react";
import { actions } from "@/app/store";
import { useFlavor } from "@/app/theme";
import { useNow } from "@/lib/clock";
import { copyText } from "@/lib/host";
import { usePref } from "@/lib/prefs";
import { errMsg } from "@/lib/text";
import { ansiPalette } from "@/lib/theme";
import { MenuAt, type MenuItem } from "@/ui";
import { runOp, type OpResult, type Ref, type Scope } from "./api";
import { CommitDetails } from "./CommitDetails";
import { GitEmpty, GitLoading, GitStopped } from "./GitStates";
import { GitHeader } from "./GitHeader";
import { GraphTable, type MenuPoint, type Pressed } from "./GraphTable";
import { rowCommit } from "./history";
import { menuFor, type Action, type Context, type Target } from "./menus";
import { OpDialog } from "./OpDialog";
import { asksFirst, doneWords, requestOf, type Values, type Writing } from "./ops";
import { useHistory } from "./useHistory";
import { commitsWords, laneColors, operationWords, stoppedWords } from "./words";
import { t, tc } from "@/lib/i18n";

const SCOPE: Scope = { all: true, remotes: true };

export function GitLens({ machineId, dir, active, viewOnly, onChanges }: { machineId: string; dir: string; active: boolean; viewOnly: boolean; onChanges: () => void }) {
  const [scope, setScope] = usePref<Scope>("git.scope", SCOPE);
  const { history, state, error, loading, fetching, more, loadMore, refresh } = useHistory(machineId, dir, scope, active);
  const [selected, setSelected] = useState<string | null>(null);
  const [menu, setMenu] = useState<{ at: MenuPoint; items: MenuItem[]; key: number } | null>(null);
  const [dialog, setDialog] = useState<Writing | null>(null);
  const flavor = useFlavor();
  const colors = useMemo(() => laneColors(ansiPalette(flavor).colors), [flavor]);
  const now = useNow(60_000);
  const ctx: Context = { branch: state?.branch ?? "", head: state?.head ?? "", branches: history?.log.branches ?? [], dirty: state?.dirty ?? 0 };

  const finished = useCallback(
    (action: Writing, values: Values, result: OpResult) => {
      setDialog(null);
      if (result.ok) actions.notify(doneWords(action, values), { brief: true });
      else if (result.operation) actions.notify(`${stoppedWords(result.operation, result.conflicts ?? 0)} ${t("Resolve them in Changes, or abort it.")}`, { error: true });
      else actions.notify(result.output || t("git refused."), { error: true });
      refresh();
    },
    [refresh],
  );

  const act = (action: Action) => {
    if (action.type === "copy") return void copyText(action.text).then((ok) => actions.notify(ok ? t("Copied {text}", { text: action.text }) : t("Could not copy"), { brief: true, error: !ok }));
    if (action.type === "changes") return onChanges();
    if (asksFirst(action, ctx)) return setDialog(action);
    runOp(machineId, dir, requestOf(action, {}, ctx)).then(
      (result) => finished(action, {}, result),
      (e) => actions.notify(errMsg(e), { error: true }),
    );
  };

  const targetOf = (pressed: Pressed): Target | null => {
    if (!history) return null;
    const commit = rowCommit(history, pressed.index);
    if (!commit) return { kind: "uncommitted" };
    if (pressed.ref && pressed.ref.kind !== "head") return { kind: pressed.ref.kind, name: pressed.ref.name, hash: commit.hash };
    return { kind: "commit", hash: commit.hash, subject: commit.subject, parents: commit.parents };
  };

  const openMenu = (pressed: Pressed, at: MenuPoint) => {
    const target = targetOf(pressed);
    if (!target) return;
    const items: MenuItem[] = menuFor(target, ctx).map((row) => ("sep" in row ? { sep: true } : { label: row.label, icon: row.icon, danger: row.danger, onSelect: () => act(row.action) }));
    setMenu({ at, items, key: Date.now() });
  };

  const pick = (index: number) => {
    if (!history) return;
    const commit = rowCommit(history, index);
    if (!commit) return viewOnly ? undefined : onChanges();
    setSelected((was) => (was === commit.hash ? null : commit.hash));
  };

  const checkout = (ref: { kind: Ref["kind"]; name: string }) => {
    if (ref.kind === "branch") return ref.name !== ctx.branch && act({ type: "checkout", name: ref.name });
    const branch = ref.name.slice(ref.name.indexOf("/") + 1);
    if (branch === ctx.branch) return;
    act(ctx.branches.includes(branch) ? { type: "checkout", name: branch } : { type: "checkout-remote", name: ref.name, branch });
  };

  const abort = () => {
    const operation = state?.operation;
    if (!operation) return;
    actions.ask(t("Abort {what}? The checkout goes back to where it stood before it began, and the conflicts you resolved are lost.", { what: operationWords(operation).toLowerCase() }), tc("git", "Abort"), () =>
      runOp(machineId, dir, { op: "abort", operation, head: ctx.head, branch: ctx.branch }).then(
        (result) => {
          actions.notify(result.ok ? t("Aborted") : result.output || t("git refused."), { brief: result.ok, error: !result.ok });
          refresh();
        },
        (e) => actions.notify(errMsg(e), { error: true }),
      ),
    );
  };

  const picked = selected && history ? history.commits.find((c) => c.hash === selected) : undefined;
  const body = !dir ? (
    <GitEmpty icon="folder" title={t("No folder")} body={t("This pane has no folder, so there is no history to show.")} />
  ) : error && !history ? (
    <GitEmpty icon="alert-triangle" title={t("Can't read the history")} body={error} />
  ) : loading && !history ? (
    <GitLoading />
  ) : !history?.log.repo ? (
    <GitEmpty icon="folder" title={t("Not a git repository")} body={t("{dir} is not inside a git repository.", { dir })} />
  ) : history.log.error ? (
    <GitEmpty icon="alert-triangle" title={t("Can't read the history")} body={history.log.error} />
  ) : !history.rows.length ? (
    <GitEmpty icon="git-commit" title={t("No commits yet")} body={t("Make the first commit and it shows up here.")} />
  ) : (
    <GraphTable
      history={history}
      colors={colors}
      remotes={scope.remotes}
      selected={selected}
      now={now}
      dirty={state?.dirty ?? 0}
      menus={!viewOnly}
      more={more}
      onPick={pick}
      onMenu={openMenu}
      onCheckout={checkout}
      onMore={() => void loadMore()}
    />
  );
  return (
    <div className="git-lens" onKeyDown={(e) => e.key === "Escape" && selected && !e.defaultPrevented && setSelected(null)}>
      <GitHeader branch={state?.branch} head={state?.head} scope={scope} onScope={setScope} count={history?.log.repo ? commitsWords(history.commits.length, history.log.more) : "…"} fetching={fetching} onRefresh={refresh} />
      {state?.operation && <GitStopped operation={state.operation} conflicts={state.conflicts} readOnly={viewOnly} onAbort={abort} onChanges={onChanges} />}
      {body}
      {picked && <CommitDetails key={picked.hash} machineId={machineId} dir={dir} commit={picked} now={now} onClose={() => setSelected(null)} />}
      {menu && <MenuAt key={menu.key} at={menu.at} items={menu.items} label={t("Git")} onClose={() => setMenu(null)} />}
      {dialog && <OpDialog machineId={machineId} dir={dir} action={dialog} ctx={ctx} changes={state?.changes} onDone={(result, values) => finished(dialog, values, result)} onClose={() => setDialog(null)} />}
    </div>
  );
}

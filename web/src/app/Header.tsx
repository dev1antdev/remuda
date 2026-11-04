// The header: [burger][title — takes the slack][usage pill][lens switch][pop-out][search]. The places live on the rail.
// Nothing in it moves when state changes: the lens group is anchored right and the pill grows into the title's slack;
// the pop-out button is there whenever the browser can do it, disabled without a pane.

import { basename, plural } from "@/lib/text";
import { machineLabel } from "@/features/roster/options";
import { useRoster } from "@/features/roster/useRoster";
import { folderOf } from "@/features/roster/workspaces";
import { budgetTone, budgetWords, budgetsFor, graver, ratio, tightest, toneClass, type BudgetTone } from "@/features/usage/budgets";
import { fillClass, tok, usd } from "@/features/usage/format";
import { accountWindows, gravest, limitWords, type AccountWindow } from "@/features/usage/limits";
import { FolderIcon, Icon, Tip } from "@/ui";
import type { Located } from "./current";
import { popOut, popOutSupported } from "./pip";
import { actions, app, type Lens } from "./store";
import { t } from "@/lib/i18n";

export default function Header({ current }: { current: Located | null }) {
  const { machines, budgets, viewOnly } = useRoster();
  const mode = app.use((s) => s.mode);
  const lens = app.use((s) => s.lens);
  const drawer = app.use((s) => s.drawer);
  const viewer = app.use((s) => s.viewer);
  return (
    <header className="topbar">
      <Tip text={t("The machines and their agents")}>
        <button type="button" className="burger icon-btn" onClick={actions.toggleDrawer} aria-label={t("Machines")} aria-expanded={drawer}>
          <Icon name="menu" size={18} />
        </button>
      </Tip>
      <span className="title">
        {mode !== "herdr" ? null : viewer ? (
          <>
            {viewer.machine !== "local" && <span className="title-machine">{machineLabel(machines, viewer.machine)} ›</span>}
            {basename(viewer.dir)}
            <span className="title-agent"> {t("· transcript")}</span>
          </>
        ) : current ? (
          <>
            {current.machine.id !== "local" && <span className="title-machine">{current.machine.label} ›</span>}
            <FolderIcon icon={current.session.icon} name={basename(folderOf(current.session)) || current.session.name} />
            {current.session.name}
            <span className="title-agent"> · {current.pane.agent || "shell"}</span>
          </>
        ) : (
          "remuda"
        )}
        {viewOnly && <span className="badge quiet viewonly-badge">{t("Viewing only")}</span>}
      </span>
      {mode === "herdr" && current && !viewer && (
        <div className="lenses">
          {current.pane.usage && current.pane.usage.window > 0 && <UsagePill usage={current.pane.usage} budget={tightest(budgetsFor(budgets, current.machine.id, current.session.dir || current.pane.cwd))} limits={accountWindows(machines, Date.now())} />}
          <LensSwitch lens={lens} current={current} viewOnly={viewOnly} />
        </div>
      )}
      {popOutSupported && (
        <Tip text={t("Pop out this agent: a small window above every other app")}>
          <button type="button" className="icon-btn bell" aria-label={t("Pop out")} disabled={!current || !!viewer} onClick={() => current && void popOut({ machineId: current.machine.id, paneId: current.pane.id })}>
            <Icon name="pip" size={18} />
          </button>
        </Tip>
      )}
      {!viewOnly && <Tip text={t("Search every conversation on every machine (⌘K / Ctrl+K has it too)")}>
        <button type="button" className="icon-btn bell" aria-label={t("Search")} onClick={() => actions.openSearch()}>
          <Icon name="search" size={18} />
        </button>
      </Tip>}
    </header>
  );
}

// UsagePill: the context's fill and the session's cost; the budget furthest along and the fullest plan limit lend
// their tone when that is graver, and their lines join the tooltip — the pill's width never changes with them.
function UsagePill({ usage, budget, limits }: { usage: NonNullable<Located["pane"]["usage"]>; budget: ReturnType<typeof tightest>; limits: AccountWindow[] }) {
  const fill = usage.context / usage.window;
  const percent = Math.round(100 * fill);
  const contextTone: BudgetTone = fill >= 0.9 ? "red" : fill >= 0.7 ? "amber" : "";
  const tone = graver(budget ? graver(contextTone, budgetTone(ratio(budget))) : contextTone, gravest(limits));
  const cls = budget || limits.length ? toneClass(tone) : fillClass(fill);
  const planWords = limits.map((w) => limitWords(w, Date.now())).join("; ");
  const told = t("{model}: context {used} of {window} ({pct}%) · {responses} · {cost} API-equivalent so far.", { model: usage.name, used: tok(usage.context), window: tok(usage.window), pct: percent, responses: plural(usage.turns, "response"), cost: usd(usage.cost) });
  return (
    <button
      type="button"
      className={`usage-pill${cls}`}
      title={`${told}${budget ? ` ${t("Budget: {words}.", { words: budgetWords(budget) })}` : ""}${planWords ? ` ${t("Plan: {words}.", { words: planWords })}` : ""} ${t("Click for the usage report.")}`}
      onClick={() => actions.openPanel("usage")}
    >
      <span className="usage-pill-bar">
        <span style={{ width: `${Math.min(100, 100 * fill)}%` }} />
      </span>
      {percent}%<span className="usage-pill-cost"> · {usd(usage.cost)}</span>
    </button>
  );
}

function LensSwitch({ lens, current, viewOnly }: { lens: Lens; current: Located; viewOnly: boolean }) {
  const tab = (id: Lens, label: string, extra?: { disabled?: boolean; title?: string; badge?: number }) => (
    <button role="tab" aria-selected={lens === id} className={lens === id ? "on" : ""} disabled={extra?.disabled} title={extra?.title} onClick={() => actions.setLens(id)}>
      {label}
      {extra?.badge ? <span className="lens-badge">{extra.badge}</span> : null}
    </button>
  );
  return (
    <nav className="lens-switch" role="tablist" aria-label={t("View")}>
      {tab("chat", t("Chat"))}
      {tab("terminal", t("Terminal"))}
      {tab("changes", t("Changes"), {
        disabled: viewOnly || !(current.session.dir || current.pane.cwd),
        title: viewOnly ? t("Viewing only") : t("What changed in this workspace's folder since you last looked"),
        badge: current.session.review?.fresh,
      })}
    </nav>
  );
}

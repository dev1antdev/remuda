// TileView is one agent as the wall and the pop-out show it: its head as the sidebar row reads it — mark, logo,
// label, workspace · machine, the pull request it named, context % · cost — its state line (how long, and what it runs this second), then the
// tail of its chat, or its terminal as it is, with a line to answer.

import Tail, { TailAnswer, TileState } from "@/features/chat/Tail";
import { displayOf, markOf, rowLabel, stateWords, type RadarPrefs } from "@/features/roster/radar";
import { hueStyle } from "@/features/roster/hue";
import { prMenuItems } from "@/features/roster/prMenu";
import { checksTone, checksWords, prWords, reviewTone, reviewWords } from "@/features/roster/prStatus";
import type { Tile, WallView } from "@/features/roster/wall";
import { Screen } from "@/features/terminal/Screen";
import { fillClass, usd } from "@/features/usage/format";
import { openUrl } from "@/lib/host";
import { Icon, Logo, Menu, Tip } from "@/ui";
import { brandColor } from "@/ui/brand";
import { folderOf } from "./current";
import { actions } from "./store";
import { t } from "@/lib/i18n";

export function TileView({ tile, view = "chat", radar, step, now, brief = false, focused = false, pinned = false, onFocus, onPin, onPick, onHide, onPopOut }: {
  tile: Tile;
  view?: WallView;
  radar: RadarPrefs;
  step: number;
  now: number;
  brief?: boolean; // the strip under a focused tile, or a card of office mode: the head and the state line only
  focused?: boolean; // this tile has the whole wall
  pinned?: boolean; // a card of office mode pinned by hand: its agent stays the featured one
  onFocus?: () => void; // the wall's focus button: give this tile the wall, or the wall back
  onPin?: () => void; // office mode's pin button: keep this agent featured, or let the wall pick again
  onPick?: () => void; // a card of office mode: the head's click pins its agent instead of opening it
  onHide?: () => void; // the wall's eye
  onPopOut?: () => void; // the wall's pop-out, when the browser can
}) {
  const { machine, session, tab, pane } = tile;
  const d = displayOf(pane, radar, now);
  const alert = pane.radar?.alert;
  const markStyle = d === "working" && !alert ? { color: brandColor(pane.agent) } : undefined;
  const usage = pane.usage;
  const fill = usage && usage.window > 0 ? usage.context / usage.window : 0;
  const where = `${session.name}${machine.id !== "local" ? ` · ${machine.label}` : ""}`;
  const screen = view === "screen";
  const open = () => {
    actions.selectPane(machine.id, pane.id);
    actions.setLens(screen ? "terminal" : "chat");
    window.focus();
  };
  const dir = folderOf(tile);
  const words = alert ?? stateWords(d, pane.radar?.last_active, now, pane.radar?.since);
  const settled = !alert && (d === "done" || d.startsWith("idle"));
  const said = settled ? pane.said : undefined;
  const pr = pane.pr;
  return (
    <section className={`tile is-${d}${alert ? " is-alert" : ""}${brief ? " brief" : ""}${pinned ? " is-pinned" : ""}${machine.id !== "local" ? " has-hue" : ""}`} style={hueStyle(machine.id)} aria-label={t("{agent} in {where}", { agent: pane.agent, where })}>
      <header className="tile-head">
        <button
          type="button"
          className="linkish tile-open"
          aria-pressed={onPick ? pinned : undefined}
          title={`${words}\n${pane.cwd}\n${onPick ? t("Click to feature this agent and keep it there (pin)") : screen ? t("Open the terminal in Agents") : t("Open the chat in Agents")}`}
          onClick={onPick ?? open}
        >
          <span className="radar-mark" style={markStyle}>
            {alert ? "!" : markOf(d, step)}
          </span>
          <Logo agent={pane.agent} />
          <span className="tile-label">{rowLabel(pane, tab, session.name, radar)}</span>
          <span className="tile-where">{where}</span>
        </button>
        {pr && (
          <span className="tile-pr-group">
            <button type="button" className="linkish tile-pr" title={`${pr.url}${pr.status ? `\n${prWords(pr.status)}` : ""}\nOpen in a new tab`} onClick={() => openUrl(pr.url)}>
              <Icon name="external-link" size={12} /> {pr.label}
            </button>
            {pr.status && (
              <Menu
                tip={t("{pr}: {status} · read with gh on the machine", { pr: pr.label, status: prWords(pr.status) })}
                label={t("{pr} actions", { pr: pr.label })}
                items={prMenuItems(machine.id, pane.id, pr)}
                trigger={
                  <button type="button" className="linkish pr-chips" aria-label={t("{pr}: {status}. Open it, or hand its checks or review to the agent", { pr: pr.label, status: prWords(pr.status) })}>
                    <span className={`pr-state is-${checksTone(pr.status)}`}>
                      <span className="pr-dot" />
                      {checksWords(pr.status)}
                    </span>
                    {reviewWords(pr.status) && <span className={`pr-state pr-review is-${reviewTone(pr.status)}`}>{reviewWords(pr.status)}</span>}
                  </button>
                }
              />
            )}
          </span>
        )}
        {usage && usage.window > 0 && (
          <span className={`radar-ctx tile-usage${fillClass(fill)}`} title={t("Context {pct}% full · {model} · {cost} API-equivalent so far", { pct: Math.round(100 * fill), model: usage.name, cost: usd(usage.cost) })}>
            {Math.round(100 * fill)}% · {usd(usage.cost)}
          </span>
        )}
        {onPin && (
          <Tip text={pinned ? t("Pinned: this agent stays featured. Unpin to let the wall pick again") : t("Pin: keep this agent featured")}>
            <button type="button" className={`icon-btn sm tile-pin${pinned ? " on" : ""}`} aria-label={pinned ? t("Unpin this agent") : t("Pin this agent as the featured one")} aria-pressed={pinned} onClick={onPin}>
              <Icon name="pin" size={14} />
            </button>
          </Tip>
        )}
        {onFocus && (
          <Tip text={focused ? "Back to the grid (Esc)" : t("Focus: this agent takes the wall, the others wait in a strip")}>
            <button type="button" className={`icon-btn sm tile-focus${focused ? " on" : ""}`} aria-label={focused ? t("Back to the grid") : t("Focus this agent")} aria-pressed={focused} onClick={onFocus}>
              <Icon name={focused ? "minimize" : "maximize"} size={14} />
            </button>
          </Tip>
        )}
        {onPopOut && (
          <Tip text={t("Pop out: a small window above every other app")}>
            <button type="button" className="icon-btn sm tile-pop" aria-label={t("Pop out")} onClick={onPopOut}>
              <Icon name="pip" size={14} />
            </button>
          </Tip>
        )}
        {onHide && (
          <Tip text={t("Hide this tile; Show hidden brings it back")}>
            <button type="button" className="icon-btn sm tile-hide" aria-label={t("Hide this tile")} onClick={onHide}>
              <Icon name="eye-off" size={14} />
            </button>
          </Tip>
        )}
      </header>
      {brief ? (
        <TileState words={words} said={said} />
      ) : screen ? (
        <>
          <TileState words={words} said={said} />
          <Screen machineId={machine.id} paneId={pane.id} cols={pane.cols} busy={pane.status === "run" || pane.status === "input"} active onOpen={open} />
          <TailAnswer machineId={machine.id} paneId={pane.id} status={pane.status} dir={dir} active />
        </>
      ) : (
        <Tail machineId={machine.id} paneId={pane.id} status={pane.status} dir={dir} words={words} settled={settled} said={said} active />
      )}
    </section>
  );
}

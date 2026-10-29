// The rail: the page's places down the left edge — the five modes, then the panels that are no place of their own and
// Settings — each an icon with its name under it, so nothing needs guessing. Under 920 px it lies along the bottom, the
// four modes a thumb reaches and the rest behind More.

import { unread } from "@/features/activity/group";
import { DEFAULT_FILTER, filterWords, type Filter } from "@/features/activity/filter";
import { machineLabel } from "@/features/roster/options";
import { useRoster } from "@/features/roster/useRoster";
import { usePref } from "@/lib/prefs";
import { Icon, Menu, Tip, type IconName, type MenuItem } from "@/ui";
import { actions, app, type Mode, type Panel } from "./store";
import { t } from "@/lib/i18n";

type Place = { label: string; icon: IconName; about: string };
const MODES: (Place & { mode: Mode; desk?: boolean })[] = [
  { mode: "herdr", label: t("Agents"), icon: "terminal", about: t("Your agents and terminals, from herdr") },
  { mode: "wall", label: t("Wall"), icon: "layers", about: t("Every agent on one screen, a tile each") },
  { mode: "tasks", label: t("Tasks"), icon: "list", about: t("Your tasks, kept on this remuda") },
  { mode: "code", label: t("Code"), icon: "code", about: t("VS Code on a machine, through remuda"), desk: true },
  { mode: "review", label: t("Review"), icon: "circle-check", about: t("Every workspace with changes to review, in one list") },
];
const PANELS: (Place & { panel: Panel })[] = [
  { panel: "standup", label: t("Standup"), icon: "sun", about: t("What every agent did today, in one report") },
  { panel: "usage", label: t("Usage"), icon: "chart", about: t("What the agents cost, by day, project and session") },
  { panel: "schedules", label: t("Schedules"), icon: "calendar", about: t("Prompts sent to agents on a timetable") },
  { panel: "activity", label: t("Activity"), icon: "bell", about: t("What happened across the agents while you were away; the alert settings are inside") },
];
const SETTINGS: Place = { label: t("Settings"), icon: "sliders", about: t("The theme, the sidebar, the panels, the alerts, the keys") };

export default function Rail() {
  const mode = app.use((s) => s.mode);
  const panel = app.use((s) => s.panel);
  const dialog = app.use((s) => s.dialog);
  const { feed, machines } = useRoster();
  const [seen] = usePref<number>("feed.seen", 0);
  const news = unread(feed, seen);
  const [filter] = usePref<Filter>("activity.filter", DEFAULT_FILTER);
  const filtered = filterWords(filter, (family) => t(family.label), machineLabel(machines, filter.machine));
  // the bell's tooltip says when the panel is filtered, so a quiet panel is not mistaken for a quiet day
  const placeOf = (p: Place & { panel: Panel }): Place => (p.panel === "activity" && filtered ? { ...p, about: `${p.about} · ${t("filtered: {words}", { words: filtered })}` } : p);
  const more: MenuItem[] = [
    ...MODES.filter((m) => m.desk).map((m) => ({ label: m.label, icon: m.icon, hint: m.about, checked: mode === m.mode, onSelect: () => actions.setMode(m.mode) })),
    { sep: true },
    ...PANELS.map((p) => ({ label: p.label, icon: p.icon, hint: p.about, checked: panel === p.panel, onSelect: () => actions.togglePanel(p.panel) })),
    { sep: true },
    { label: SETTINGS.label, icon: SETTINGS.icon, hint: SETTINGS.about, onSelect: () => actions.openDialog({ kind: "settings" }) },
  ];
  return (
    <nav className="rail" aria-label={t("Places")}>
      <div className="rail-group">
        {MODES.map((m) => (
          <RailButton key={m.mode} place={m} on={mode === m.mode} desk={m.desk} onClick={() => actions.setMode(m.mode)} />
        ))}
        <Menu
          label={t("More places")}
          side="top"
          trigger={
            <button type="button" className="rail-btn rail-more" aria-label={t("More")}>
              <Icon name="more" size={18} />
              <span className="rail-label">{t("More")}</span>
            </button>
          }
          items={more}
        />
      </div>
      <div className="rail-group rail-foot">
        {PANELS.map((p) => (
          <RailButton key={p.panel} place={placeOf(p)} on={panel === p.panel} dot={p.panel === "activity" && news} onClick={() => actions.togglePanel(p.panel)} />
        ))}
        <RailButton place={SETTINGS} on={dialog?.kind === "settings"} onClick={() => actions.openDialog({ kind: "settings" })} />
      </div>
    </nav>
  );
}

function RailButton({ place, on, dot = false, desk = false, onClick }: { place: Place; on: boolean; dot?: boolean; desk?: boolean; onClick: () => void }) {
  return (
    <Tip text={place.about} side="right">
      <button type="button" className={`rail-btn${on ? " on" : ""}${dot ? " has-news" : ""}${desk ? " rail-desk" : ""}`} aria-pressed={on} onClick={onClick}>
        <Icon name={place.icon} size={18} />
        <span className="rail-label">{place.label}</span>
      </button>
    </Tip>
  );
}

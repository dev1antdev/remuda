// The rail: the page's places down the left edge — the three modes, then the tools and Settings — each an icon with
// its name under it, so nothing needs guessing. Under 920 px it lies along the bottom, the two modes a thumb reaches
// and the rest behind More.

import { useRoster } from "@/features/roster/useRoster";
import { keyWords } from "@/features/terminal/float/float";
import { toggleFloat } from "@/features/terminal/float/useFloatKey";
import { useCurrent } from "./current";
import { viewerPlaces } from "./viewOnly";
import { isMac } from "@/lib/prefs";
import { Icon, Menu, Tip, type IconName, type MenuItem } from "@/ui";
import { actions, app, type Mode } from "./store";
import { t } from "@/lib/i18n";

// short is the word under the icon when the language's full name does not fit there
type Place = { label: string; short?: string; icon: IconName; about: string };
const MODES: (Place & { mode: Mode; desk?: boolean })[] = [
  { mode: "herdr", label: t("Agents"), icon: "terminal", about: t("Your agents and terminals, in remuda-core") },
  { mode: "tasks", label: t("Tasks"), icon: "list", about: t("Your tasks, kept on this remuda") },
  { mode: "code", label: t("Code"), icon: "code", about: t("VS Code on a machine, through remuda"), desk: true },
];
const SETTINGS: Place = { label: t("Settings"), icon: "sliders", about: t("The theme, the sidebar, the panels, the alerts, the keys") };
const FLOAT: Place = { label: t("Shell"), icon: "square-terminal", about: t("A floating terminal over any screen, on a shell of its own · {key}", { key: keyWords(isMac) }) };

export default function Rail() {
  const mode = app.use((s) => s.mode);
  const dialog = app.use((s) => s.dialog);
  const float = app.use((s) => s.float);
  const { machines, viewOnly } = useRoster();
  const current = useCurrent();
  const onFloat = () => toggleFloat(machines, current?.machine.id ?? null);
  const modes = viewerPlaces(MODES, viewOnly);
  const more: MenuItem[] = [
    ...modes.filter((m) => m.desk).map((m) => ({ label: m.label, icon: m.icon, hint: m.about, checked: mode === m.mode, onSelect: () => actions.setMode(m.mode) })),
    { sep: true },
    ...(viewOnly
      ? []
      : [
          { label: FLOAT.label, icon: FLOAT.icon, hint: FLOAT.about, checked: !!float, onSelect: onFloat } as MenuItem,
          { sep: true } as MenuItem,
          { label: SETTINGS.label, icon: SETTINGS.icon, hint: SETTINGS.about, onSelect: () => actions.openDialog({ kind: "settings" }) } as MenuItem,
        ]),
  ];
  return (
    <nav className="rail" aria-label={t("Places")}>
      <div className="rail-group">
        {modes.map((m) => (
          <RailButton key={m.mode} place={m} on={mode === m.mode} desk={m.desk} onClick={() => actions.setMode(m.mode)} />
        ))}
        <Menu
          label={t("More places")}
          side="top"
          trigger={
            <button type="button" className="rail-btn rail-more" aria-label={t("More")}>
              <span className="rail-icon">
                <Icon name="more" size={18} />
              </span>
              <span className="rail-label">{t("More")}</span>
            </button>
          }
          items={more}
        />
      </div>
      <div className="rail-group rail-foot">
        {!viewOnly && <RailButton place={FLOAT} on={!!float} onClick={onFloat} />}
        {!viewOnly && <RailButton place={SETTINGS} on={dialog?.kind === "settings"} onClick={() => actions.openDialog({ kind: "settings" })} />}
      </div>
    </nav>
  );
}

// tone: the Server place's dot, amber for a look and red for a concern in the host's body or disk
function RailButton({ place, on, dot = false, tone = "", desk = false, onClick }: { place: Place; on: boolean; dot?: boolean; tone?: "bad" | "warn" | ""; desk?: boolean; onClick: () => void }) {
  return (
    <Tip text={place.about} side="right">
      <button type="button" className={`rail-btn${on ? " on" : ""}${dot ? " has-news" : ""}${tone ? ` has-${tone}` : ""}${desk ? " rail-desk" : ""}`} aria-pressed={on} aria-label={place.label} onClick={onClick}>
        <span className="rail-icon">
          <Icon name={place.icon} size={18} />
        </span>
        <span className="rail-label" aria-hidden="true">{place.short ?? place.label}</span>
      </button>
    </Tip>
  );
}

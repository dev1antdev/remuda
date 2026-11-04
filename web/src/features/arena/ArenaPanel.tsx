// The Arenas panel: every arena, the newest first, its lanes as cards side by side — the agent and its model, how it
// stands, the first line of what it answered, +ins −del · files, how long it ran, what it cost — with Open, and
// "Keep this one", which closes the other lanes once their work is saved as patches.

import { act } from "@/app/act";
import { actions } from "@/app/store";
import type { Machine } from "@/features/roster/api";
import { DEFAULT_RADAR, displayOf, stateWords, type RadarPrefs } from "@/features/roster/radar";
import { useRoster } from "@/features/roster/useRoster";
import { usd } from "@/features/usage/format";
import { usePref } from "@/lib/prefs";
import { useQuery } from "@/lib/query";
import { useNow } from "@/lib/clock";
import { ago, spent } from "@/lib/time";
import { Dialog, Icon, Logo, Notice } from "@/ui";
import { VENDOR_NAME } from "@/ui/brand";
import { t, tn } from "@/lib/i18n";
import { fetchArenas, forgetArena, keepLane, type Arena, type Lane } from "./api";
import { keepWords, laneTone, paneOf, runSeconds, statWords } from "./arena";

export default function ArenaPanel({ onClose }: { onClose: () => void }) {
  const { machines } = useRoster();
  const { data, error, reload } = useQuery(fetchArenas, [], { every: 5000, keep: true });
  const arenas = data ?? [];
  return (
    <Dialog
      title={t("Arenas")}
      icon="columns"
      size="lg"
      className="arena-panel"
      onClose={onClose}
      controls={
        <button type="button" className="btn sm" onClick={() => actions.openDialog({ kind: "arena" })}>
          <Icon name="plus" size={14} /> {t("New arena")}
        </button>
      }
    >
      {error && <Notice tone="error">{error}</Notice>}
      {!error && data && arenas.length === 0 && (
        <p className="modal-hint">{t("No arena yet. An arena gives one prompt to several agents, each in a worktree of its own, so you can compare their work and keep the best.")}</p>
      )}
      {arenas.map((a) => (
        <ArenaView key={a.id} arena={a} machines={machines} onChanged={reload} />
      ))}
    </Dialog>
  );
}

function ArenaView({ arena: a, machines, onChanged }: { arena: Arena; machines: Machine[]; onChanged: () => void }) {
  const decided = !!a.kept;
  const forget = () => void act(forgetArena(a.id)).then(onChanged);
  return (
    <section className="arena">
      <header className="arena-head">
        <b className="arena-title" title={a.prompt}>
          {a.title}
        </b>
        <span className="arena-when">
          {tn(a.lanes.length, "{n} agent", "{n} agents")} · {ago(a.created)}
        </span>
        {decided && (
          <button type="button" className="btn sm ghost" onClick={forget}>
            {t("Forget")}
          </button>
        )}
      </header>
      <div className="arena-cards">
        {a.lanes.map((lane) => (
          <LaneCard key={lane.id} arena={a} lane={lane} machines={machines} onChanged={onChanged} />
        ))}
      </div>
    </section>
  );
}

function LaneCard({ arena: a, lane, machines, onChanged }: { arena: Arena; lane: Lane; machines: Machine[]; onChanged: () => void }) {
  const [radar] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);
  const now = useNow(1000);
  const pane = paneOf(machines, lane);
  const machine = machines.find((m) => m.id === lane.machine);
  const tone = laneTone(lane);
  const live = lane.state === "running" || lane.state === "starting";
  const display = pane?.agent ? displayOf(pane, radar, now) : undefined;
  const state =
    lane.state === "kept" ? t("kept") : lane.state === "discarded" ? t("closed, its work saved") : lane.state === "failed" ? t("did not start") : lane.state === "starting" && !display ? t("starting…") : display ? stateWords(display, pane?.radar?.last_active, now, pane?.radar?.since) : t("its pane is gone");
  const words = lane.note || (pane?.said ?? "");
  const stat = statWords(a.stats?.[String(lane.id)]);
  const seconds = runSeconds(lane, pane, now);
  const cost = pane?.usage?.cost;
  const keep = () =>
    actions.ask(keepWords(a.lanes, lane.id), t("Keep this one"), () =>
      void act(keepLane(a.id, lane.id)).then(onChanged),
    );
  return (
    <article className={`arena-card${tone ? ` is-${tone}` : ""}${display ? ` is-${display}` : ""}`}>
      <div className="arena-card-head">
        <Logo agent={lane.agent} />
        <b>{VENDOR_NAME[lane.agent] ?? lane.agent}</b>
        {lane.model && <span className="arena-model">{lane.model}</span>}
        {machine && machine.id !== "local" && <span className="arena-machine">{machine.label}</span>}
      </div>
      <div className="arena-state">{state}</div>
      {words && (
        <p className="arena-words" title={lane.patch ? `${words}\n${lane.patch}` : words}>
          {words}
        </p>
      )}
      <div className="arena-figures">
        {[stat, seconds ? spent(seconds) : "", cost ? usd(cost) : ""].filter(Boolean).join(" · ")}
      </div>
      {lane.branch && (
        <div className="arena-branch" title={lane.dir}>
          <Icon name="git-branch" size={12} /> {lane.branch}
        </div>
      )}
      {lane.patch && (
        <div className="arena-patch" title={lane.patch}>
          <Icon name="file" size={12} /> {lane.patch.split("/").pop()}
        </div>
      )}
      <div className="arena-actions">
        {pane && (
          <button type="button" className="btn sm" onClick={() => actions.openPane(lane.machine, pane.id)}>
            {t("Open")}
          </button>
        )}
        {!a.kept && live && lane.dir && (
          <button type="button" className="btn sm" onClick={keep}>
            <Icon name="check" size={14} /> {t("Keep this one")}
          </button>
        )}
      </div>
    </article>
  );
}

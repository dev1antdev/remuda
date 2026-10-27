// The standup panel: what every agent on every machine did since midnight (or over the last day or week), the ones that
// need the person first — a row each with its numbers, its files and its last answer — and the report as Markdown to paste.

import { useMemo } from "react";
import { actions } from "@/app/store";
import { copyText } from "@/lib/host";
import { usePref } from "@/lib/prefs";
import { useQuery } from "@/lib/query";
import { plural } from "@/lib/text";
import { usd } from "@/features/usage/format";
import { Dialog, Icon, Logo, Notice, Select } from "@/ui";
import { fetchStandup, type Item } from "./api";
import { RecapBody } from "./RecapBody";
import { order, PERIODS, quiet, sinceFor, stateLabel, where, type Period } from "./standup";
import { t } from "@/lib/i18n";

export default function Standup({ onClose }: { onClose: () => void }) {
  const [period, setPeriod] = usePref<Period>("standup.period", "today");
  const tz = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  const { data, error, loading } = useQuery(() => fetchStandup(sinceFor(period), tz), [period, tz], { every: 60_000, keep: true });
  const items = useMemo(() => order(data?.items ?? []), [data]);
  const busy = items.filter((item) => item.error || !quiet(item.recap));
  const idle = items.filter((item) => !item.error && quiet(item.recap));

  const open = (item: Item) => {
    actions.selectPane(item.machine, item.pane);
    actions.setLens("chat");
    onClose();
  };
  // a stretch of the strip opens the chat at the turn it belongs to
  const jump = (item: Item, turn: string) => {
    actions.selectPane(item.machine, item.pane);
    actions.jumpTo(item.machine, item.pane, "", turn);
    onClose();
  };
  const copy = async () => {
    if (!data) return;
    if (await copyText(data.markdown)) actions.notify(t("Copied the standup as Markdown"), { brief: true });
    else actions.notify(t("Could not copy"), { error: true });
  };
  const periodWords = period === "today" ? "today" : period === "day" ? t("in the last 24 hours") : t("in the last 7 days");

  return (
    <Dialog
      title={t("Standup")}
      icon="sun"
      size="lg"
      className="standup"
      onClose={onClose}
      controls={
        <>
          <Select value={period} onChange={setPeriod} options={PERIODS} />
          {loading && <span className="search-busy" />}
        </>
      }
    >
      {error && <Notice tone="error">{error}</Notice>}
      {data && items.length === 0 && <Notice tone="hint">{t("No agent is running on a connected machine.")}</Notice>}
      {data && items.length > 0 && busy.length === 0 && <Notice tone="hint">{t("Nothing happened {period}: no prompt, no answer, no tool call from any agent.", { period: periodWords })}</Notice>}
      {busy.map((item) => (
        <article key={`${item.machine}|${item.pane}`} className="standup-item">
          <button type="button" className="linkish standup-open" onClick={() => open(item)} title={`${item.dir}\n${t("Open this agent's chat")}`}>
            <Logo agent={item.agent} />
            <span className="standup-where">
              {item.workspace}
              {item.machine !== "local" && <span className="standup-machine"> · {item.label}</span>}
            </span>
            <span className={`standup-state is-${item.alert ? "blocked" : item.state}`} title={item.alert}>
              {t(stateLabel(item))}
            </span>
            {item.recap.cost > 0 && <span className="standup-cost">{usd(item.recap.cost)}</span>}
          </button>
          {item.error ? <Notice tone="error">{item.error}</Notice> : <RecapBody recap={item.recap} dir={item.dir} since={data?.since} onJump={(turn) => jump(item, turn)} />}
        </article>
      ))}
      {idle.length > 0 && (
        <details className="standup-quiet">
          <summary>
            {plural(idle.length, "agent")} with nothing to report {periodWords}
          </summary>
          <ul>
            {idle.map((item) => (
              <li key={`${item.machine}|${item.pane}`}>
                <button type="button" className="linkish" onClick={() => open(item)}>
                  {where(item)}
                </button>
                {stateLabel(item) && ` · ${stateLabel(item)}`}
              </li>
            ))}
          </ul>
        </details>
      )}
      <div className="modal-actions">
        <button type="button" className="btn primary" disabled={!data} onClick={() => void copy()}>
          <Icon name="copy" size={14} /> {t("Copy as Markdown")}
        </button>
      </div>
    </Dialog>
  );
}

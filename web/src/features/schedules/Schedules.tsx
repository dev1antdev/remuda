// Schedules: prompts sent to an agent on a timetable, the standup pushed as a digest, or a fresh agent started in a
// folder. A list with the next and last run, a switch per row, "Run now", and the editor.

import { useState } from "react";
import type { Machine } from "@/features/roster/api";
import { machineLabel } from "@/features/roster/options";
import { useRoster } from "@/features/roster/useRoster";
import { useQuery } from "@/lib/query";
import { basename, errMsg } from "@/lib/text";
import { ago, whenAhead } from "@/lib/time";
import { Dialog, Icon, Logo, Notice, Tip } from "@/ui";
import { agentWords, deniedWords, watchedWords } from "./agent";
import { deleteSchedule, fetchSchedules, putSchedule, runSchedule, type Schedule } from "./api";
import { NEVER } from "./cron";
import { cronWords } from "./cronWords";
import { kindOf, sinceWords } from "./digest";
import { resultTone, resultWords } from "./runs";
import { ScheduleEditor } from "./ScheduleEditor";
import { t } from "@/lib/i18n";

export default function Schedules({ machines, onClose, ask }: {
  machines: Machine[];
  onClose: () => void;
  ask: (text: string, label: string, run: () => void) => void;
}) {
  const { agents } = useRoster();
  const agentLabel = (id: string | undefined) => agents.find((a) => a.id === id)?.label ?? id ?? "";
  const [actionError, setActionError] = useState("");
  const [editing, setEditing] = useState<Partial<Schedule> | null>(null);
  const { data: items, error: fetchError, reload, mutate } = useQuery(fetchSchedules, []);
  const error = actionError || fetchError;
  const fail = (e: unknown) => setActionError(errMsg(e));

  // the switch flips at once; a failed save flips it back
  const toggle = (s: Schedule, enabled: boolean) => {
    const flip = (to: boolean) => mutate((list) => list?.map((x) => (x.id === s.id ? { ...x, enabled: to } : x)) ?? list);
    flip(enabled);
    putSchedule({ ...s, enabled }).then(reload, (e) => {
      fail(e);
      flip(s.enabled);
    });
  };

  return (
    <Dialog
      title={t("Schedules")}
      icon="calendar"
      size="md"
      className="schedules"
      onClose={onClose}
      controls={
        <button type="button" className="btn sm" onClick={() => setEditing({ enabled: true })}>
          <Icon name="plus" size={12} /> {t("New")}
        </button>
      }
    >
      <Notice tone="hint">
        {t(
          "A prompt sent to the agent running in a folder, the standup pushed to your devices as a digest, or a fresh agent started in a folder with a prompt, on a timetable — in the remuda host's local time. A run is skipped and noted while the agent it would reach — or the one its last run started — is still working or waits for an answer, so nothing piles up; a minute with no agent there is noted and pushed.",
        )}
      </Notice>
      {error && (
        <Notice tone="error" onDismiss={() => setActionError("")}>
          {error}
        </Notice>
      )}
      {items && items.length === 0 && !editing && <div className="empty">{t("No schedules yet.")}</div>}
      <div className="schedules-list">
        {items?.map((s) => (
          <div key={s.id} className={`schedule${s.enabled ? "" : " off"}`}>
            <input type="checkbox" checked={s.enabled} title={s.enabled ? t("On") : t("Off")} aria-label={s.enabled ? t("On") : t("Off")} onChange={(e) => toggle(s, e.target.checked)} />
            <div className="schedule-copy">
              <div className="schedule-label">
                {kindOf(s) === "digest" && <Icon name="sun" size={12} className="schedule-digest-mark" aria-label={t("Standup digest")} />}
                {kindOf(s) === "agent" && (
                  <span className="schedule-agent-mark" aria-label={t("Starts {agent}", { agent: agentLabel(s.agent) })}>
                    <Logo agent={s.agent ?? ""} />
                  </span>
                )}
                {s.label}
              </div>
              <div className="schedule-sub">
                {cronWords(s.cron) ?? <code>{s.cron}</code>} ·{" "}
                {kindOf(s) === "digest"
                  ? t("to your devices · {since}", { since: sinceWords(s.since) })
                  : kindOf(s) === "agent"
                    ? agentWords(s, machineLabel(machines, s.machine), basename(s.dir), agentLabel(s.agent))
                    : `${machineLabel(machines, s.machine)} · ${basename(s.dir)}`}
                {watchedWords(s) && <span> · {watchedWords(s)}</span>}
                {s.enabled && s.next && s.next !== NEVER && <span> · {t("next {when}", { when: whenAhead(s.next) })}</span>}
                {s.last_run && s.last_run !== NEVER && (
                  <>
                    {" · "}
                    <span className={resultTone(s)}>
                      {t("last {ago}: {result}", { ago: ago(s.last_run), result: resultWords(s) })}
                    </span>
                  </>
                )}
              </div>
              {deniedWords(s) && (
                <div className="schedule-denied">
                  <Icon name="alert-triangle" size={12} /> {deniedWords(s)}
                </div>
              )}
            </div>
            <Tip text={t("Run now")}>
              <button type="button" className="icon-btn" aria-label={t("Run now")} onClick={() => void runSchedule(s.id ?? "").then(reload, fail)}>
                <Icon name="zap" size={14} />
              </button>
            </Tip>
            <Tip text={t("Edit")}>
              <button type="button" className="icon-btn" aria-label={t("Edit")} onClick={() => setEditing(s)}>
                <Icon name="sliders" size={14} />
              </button>
            </Tip>
            <Tip text={t("Delete")}>
              <button
                type="button"
                className="icon-btn danger"
                aria-label={t("Delete")}
                onClick={() => ask(t("Delete the schedule “{label}”?", { label: s.label }), t("Delete"), () => void deleteSchedule(s.id ?? "").then(reload, fail))}
              >
                <Icon name="trash" size={14} />
              </button>
            </Tip>
          </div>
        ))}
      </div>
      {editing && (
        <ScheduleEditor
          key={editing.id ?? "new"} // a different schedule is a fresh form
          machines={machines}
          value={editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            reload();
          }}
        />
      )}
    </Dialog>
  );
}

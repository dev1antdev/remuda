// Schedules: prompts sent to an agent on a timetable, the standup pushed as a digest, or a fresh agent started in a
// folder. A list with the next and last run, a switch per row, "Run now", and the editor.

import { useState } from "react";
import type { Machine } from "@/features/roster/api";
import { machineLabel } from "@/features/roster/options";
import { useRoster } from "@/features/roster/useRoster";
import { useQuery } from "@/lib/query";
import { basename, errMsg } from "@/lib/text";
import { ago, whenShort } from "@/lib/time";
import { Dialog, Icon, Logo, Notice } from "@/ui";
import { agentWords, watchedWords } from "./agent";
import { deleteSchedule, fetchSchedules, putSchedule, runSchedule, type Schedule } from "./api";
import { NEVER } from "./cron";
import { kindOf, sinceWords } from "./digest";
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
        A prompt sent to the agent running in a folder, the standup pushed to your devices as a digest, or a fresh agent started in a folder with a
        prompt, on a timetable — in the remuda host's local time. A minute with no agent there, or an agent busy with a question, is skipped and noted.
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
                <code>{s.cron}</code> ·{" "}
                {kindOf(s) === "digest"
                  ? t("to your devices · {since}", { since: sinceWords(s.since) })
                  : kindOf(s) === "agent"
                    ? agentWords(s, machineLabel(machines, s.machine), basename(s.dir), agentLabel(s.agent))
                    : `${machineLabel(machines, s.machine)} · ${basename(s.dir)}`}
                {watchedWords(s) && <span> · {watchedWords(s)}</span>}
                {s.enabled && s.next && s.next !== NEVER && <span> · {t("next {when}", { when: whenShort(s.next) })}</span>}
                {s.last_run && s.last_run !== NEVER && (
                  <>
                    {" · "}
                    <span className={s.last_ok ? "ok" : "bad"}>
                      last {ago(s.last_run)}: {s.last_result}
                    </span>
                  </>
                )}
              </div>
            </div>
            <button type="button" className="icon-btn" title={t("Run now")} onClick={() => void runSchedule(s.id ?? "").then(reload, fail)}>
              <Icon name="zap" size={14} />
            </button>
            <button type="button" className="icon-btn" title={t("Edit")} onClick={() => setEditing(s)}>
              <Icon name="sliders" size={14} />
            </button>
            <button
              type="button"
              className="icon-btn danger"
              title={t("Delete")}
              onClick={() => ask(`Delete the schedule “${s.label}”?`, t("Delete"), () => void deleteSchedule(s.id ?? "").then(reload, fail))}
            >
              <Icon name="trash" size={14} />
            </button>
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

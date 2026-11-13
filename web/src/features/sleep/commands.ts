// The palette's rows for the sleeps: every workspace asleep, to wake, and every machine whose chats a reboot stopped,
// its "Resume after restart".

import { actions } from "@/app/store";
import type { Entry } from "@/features/palette/rank";
import type { Machine } from "@/features/roster/api";
import { stoppedWords } from "@/features/roster/stopped";
import { t, tn } from "@/lib/i18n";
import { wake } from "./flows";

type Command = { id: string; title: string; run: () => void; more: Partial<Entry> };

export function sleepCommands(machines: Machine[]): Command[] {
  const out: Command[] = [];
  for (const m of machines.filter((m) => m.state === "connected")) {
    const elsewhere = machines.length > 1 ? ` · ${m.label}` : "";
    for (const s of m.sessions.filter((s) => s.asleep && !s.scratch))
      out.push({
        id: `wake:${m.id}|${s.id}`,
        title: t("Wake {workspace}", { workspace: `“${s.name}”` }),
        run: () => void wake(m.id, s),
        more: { hint: tn(s.asleep!.agents, "asleep · {n} agent back on its conversation", "asleep · {n} agents back on their conversations") + elsewhere, icon: "sun", keywords: ["wake", "asleep", "sleep", "hibernate", "resume", s.name, m.label] },
      });
    if (m.stopped?.length && !m.resuming)
      out.push({
        id: `resume-after-restart:${m.id}`,
        title: t("Resume after restart on {machine}…", { machine: m.label }),
        run: () => actions.openDialog({ kind: "resume-after-restart", machineId: m.id }),
        more: { hint: stoppedWords(m.stopped.length), icon: "refresh-cw", keywords: ["resume", "reboot", "restart", "stopped", "chats", "recover", m.label] },
      });
  }
  return out;
}

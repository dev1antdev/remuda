// The project's agents in its settings: where they work (a machine and a repository folder their worktrees branch
// off), which agent with which model and arguments — a recipe fills them — and the queue's switch with how many of
// its agents may work at once. The Start dialog starts from these too.

import { whyNoDeny } from "@/features/agents/deny";
import { DenySwitch } from "@/features/agents/DenySwitch";
import type { AgentKind, Machine } from "@/features/roster/api";
import type { Recipe } from "@/features/recipes/api";
import Icon from "@/ui/icons";
import Select from "@/ui/Select";
import { Logo } from "@/ui";
import RetiredNotice from "@/features/usage/RetiredNotice";
import { t } from "@/lib/i18n";
import type { Automation, Status } from "./api";
import { denyCovers, MAX_QUEUED, repositories, whyNotQueue } from "./autonomy";

export function AgentsSettings({ value, onChange, machines, agents, recipes, statuses, onAddQueued }: {
  value: Automation;
  onChange: (a: Automation) => void;
  machines: Machine[];
  agents: AgentKind[];
  recipes: Recipe[];
  statuses: Status[]; // the columns as edited
  onAddQueued: () => void; // a Queued column added to the edit, for the switch to have one
}) {
  const set = (patch: Partial<Automation>) => onChange({ ...value, ...patch });
  const repos = repositories(machines, value.machine);
  const known = repos.some((r) => r.value === value.dir);
  const agentChoices = agents.filter((a) => a.id !== "shell").map((a) => ({ value: a.id, label: a.label, icon: <Logo agent={a.id} /> }));
  const why = whyNotQueue(value, statuses);
  const usable = recipes.filter((r) => r.agent && r.agent !== "shell");
  return (
    <>
      <div className="tasks-field-name">{t("Agents")}</div>
      <p className="modal-hint">{t("Where this project's agents work and which agent starts: the Start dialog begins from here, and so does the queue.")}</p>
      <div className="schedule-row">
        <label>
          {t("Machine")}
          <Select
            value={value.machine}
            onChange={(machine) => set({ machine, dir: machine === value.machine ? value.dir : "" })}
            placeholder={t("Not set")}
            options={[{ value: "", label: t("Not set") }, ...machines.map((m) => ({ value: m.id, label: m.label, icon: <Icon name="monitor" size={14} /> }))]}
          />
        </label>
        <label className="grow">
          {t("Repository")}
          <Select
            value={value.dir}
            onChange={(dir) => set({ dir })}
            placeholder={value.machine ? t("Pick a repository") : t("Pick the machine first")}
            options={[...(value.dir && !known ? [{ value: value.dir, label: value.dir, hint: t("no workspace open on it now") }] : []), ...repos.map((r) => ({ ...r, icon: <Icon name="folder" size={14} /> }))]}
          />
        </label>
      </div>
      <div className="schedule-row ps-where">
        <label className="grow">
          {t("The queue starts agents on")}
          <Select
            value={value.anywhere ? "any" : "one"}
            onChange={(where) => set({ anywhere: where === "any" })}
            options={[
              { value: "one", label: t("This machine only"), icon: <Icon name="monitor" size={14} /> },
              { value: "any", label: t("Any machine that has room"), icon: <Icon name="server" size={14} /> },
            ]}
          />
        </label>
      </div>
      {value.anywhere && (
        <p className="modal-hint">
          {t(
            "A queued card starts on the connected machine with the most room that has a workspace open on a checkout of this repository, matched by its remote: the lightest load per core, a cool CPU, on its charger, room on the agent's plan window, the fewest agents working. A hot machine, one on a nearly empty battery, one at two of load per core or over its budget is passed over; this remuda's own host never runs one when it only serves the page.",
          )}
        </p>
      )}
      <div className="schedule-row">
        <label>
          {t("Agent")}
          <Select value={value.agent || "claude"} onChange={(agent) => set({ agent })} options={agentChoices} />
        </label>
        <label className="grow">
          {t("Model")}
          <input value={value.model ?? ""} placeholder={t("the agent's default")} spellCheck={false} onChange={(e) => set({ model: e.target.value })} />
        </label>
        <label className="grow">
          {t("Extra arguments")}
          <input value={value.args ?? ""} placeholder="--permission-mode plan" spellCheck={false} onChange={(e) => set({ args: e.target.value })} />
        </label>
      </div>
      <RetiredNotice agent={value.agent || "claude"} model={value.model ?? ""} args={value.args ?? ""} machine={value.machine} onUse={(model) => set({ model })} />
      {usable.length > 0 && (
        <label>
          {t("Fill from a recipe")}
          <Select
            value=""
            onChange={(id) => {
              const r = usable.find((x) => x.id === id);
              if (r) set({ agent: r.agent, model: r.model ?? "", args: r.args ?? "" });
            }}
            placeholder={t("A saved way to start an agent")}
            options={usable.map((r) => ({ value: r.id ?? r.name, label: r.name, icon: <Logo agent={r.agent} /> }))}
          />
        </label>
      )}
      <div className="schedule-row ps-queue">
        <label className="tasks-toggle">
          <input type="checkbox" checked={value.queue} disabled={!value.queue && !!why} onChange={(e) => set({ queue: e.target.checked })} /> {t("Start queued tasks by themselves")}
        </label>
        <label className="ps-max">
          {t("At most")}
          <input type="number" min={1} max={MAX_QUEUED} value={value.max || 1} onChange={(e) => set({ max: Math.max(1, Math.min(MAX_QUEUED, Number(e.target.value) || 1)) })} />
          {t("working at once")}
        </label>
      </div>
      <p className="modal-hint">
        {t(
          "The queue starts the top card of the Queued column once a slot is free, the machine is there, the agent's plan window is not nearly spent and no budget over the folder is crossed — only cards you put there, and a refused one only once you move it again. A window at 95 %, or forecast to run out within half an hour, holds the card, and you are offered to run it now on the other vendor's agent.",
        )}
        {why && (
          <>
            {" "}
            <b>{why}.</b>{" "}
            {why === t("Mark a column Queued first") && (
              <button type="button" className="linkish" onClick={onAddQueued}>
                {t("Add a Queued column")}
              </button>
            )}
          </>
        )}
      </p>
      <div className="ps-deny">
        <DenySwitch
          checked={!!value.deny_prompts}
          onChange={(deny_prompts) => set({ deny_prompts })}
          why={whyNoDeny(denyCovers(value).map((id) => ({ id, label: agents.find((a) => a.id === id)?.label ?? id })))}
        />
        <p className="modal-hint">{t("It covers the Claude Code agents the queue starts and every stage of the pipeline; an agent you start yourself from a task asks as usual.")}</p>
      </div>
    </>
  );
}

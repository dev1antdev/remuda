// The dialog's Details, filled from the pick and editable: the branch and the sentence that says what Create will do
// with it, the workspace's name, the agent with its first prompt (a template's for an issue or a pull request), and the
// setup script's Run or Skip.

import type { Recipe } from "@/features/recipes/api";
import { Icon, Logo, Select } from "@/ui";
import { t } from "@/lib/i18n";
import type { AgentKind } from "../api";
import { StartOptionsFields } from "../dialogs";
import type { Draft } from "../startOptions";
import type { Plan } from "./plan";

export function Details({ branch, onBranch, note, error, name, onName, namePlaceholder, agents, agent, onAgent, draft, onDraft, onRecipe, machine, setup, run, onRun }: {
  branch: string;
  onBranch: (b: string) => void;
  note: string;
  error: boolean;
  name: string;
  onName: (n: string) => void;
  namePlaceholder: string;
  agents: AgentKind[];
  agent: string;
  onAgent: (a: string) => void;
  draft: Draft;
  onDraft: (d: Draft) => void;
  onRecipe: (r: Recipe) => void;
  machine: string;
  setup: string; // the setup script on one line, "" when the project has none
  run: boolean | null; // null: the project asks every time and nothing is chosen yet
  onRun: (run: boolean) => void;
}) {
  const shell = agent === "shell" || agent === "";
  return (
    <div className="nw-details">
      <div className="nw-details-head">{t("Details")}</div>
      <label>
        {t("Branch")}
        <input className="mono" value={branch} spellCheck={false} autoComplete="off" onChange={(e) => onBranch(e.target.value)} />
        <span className={`nw-note${error ? " is-error" : ""}`} aria-live="polite">
          {note || " "}
        </span>
      </label>
      <label>
        {t("Workspace name")}
        <input value={name} placeholder={namePlaceholder} onChange={(e) => onName(e.target.value)} />
      </label>
      <label>
        <span>
          {t("Agent")} <small>{t("optional")}</small>
        </span>
        <Select
          value={shell ? "shell" : agent}
          onChange={onAgent}
          options={[{ value: "shell", label: t("None · just a terminal"), icon: <Icon name="terminal" size={14} /> }, ...agents.map((a) => ({ value: a.id, label: a.label, icon: <Logo agent={a.id} /> }))]}
        />
      </label>
      {!shell && (
        <label>
          {t("First prompt")} <small>{t("sent once the agent is up, only when you create")}</small>
          <textarea value={draft.prompt} rows={3} placeholder={t("What the agent should start on")} onChange={(e) => onDraft({ ...draft, prompt: e.target.value })} />
        </label>
      )}
      <StartOptionsFields kind={shell ? "shell" : agent} draft={draft} agents={agents} machine={machine} onChange={onDraft} onRecipe={onRecipe} prompt={false} />
      {setup && (
        <div className="nw-setup">
          <span className="nw-setup-label">
            {t("Setup script")}
            {run === null && <small> {t("choose")}</small>}
          </span>
          <div className="nw-setup-row">
            <button type="button" className={`btn sm${run === true ? " on" : ""}`} aria-pressed={run === true} onClick={() => onRun(true)}>
              {t("Run")}
            </button>
            <button type="button" className={`btn sm${run === false ? " on" : ""}`} aria-pressed={run === false} onClick={() => onRun(false)}>
              {t("Skip")}
            </button>
            <code className="nw-setup-script" title={setup}>
              {setup}
            </code>
          </div>
        </div>
      )}
    </div>
  );
}

// noteOf is the line under the branch: what Create will do, the pull request it comes from, or what is wrong
export function noteOf(plan: Plan | null, prefix: string, looking: boolean, reading: boolean): { note: string; error: boolean } {
  if (looking) return { note: t("Looking it up on GitHub…"), error: false };
  if (!plan) return { note: reading ? t("Reading branches…") : "", error: false };
  if (plan.action === "error") return { note: plan.note, error: true };
  return { note: prefix + plan.note, error: false };
}

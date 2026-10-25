// The "make something" dialogs: a workspace, a tab, a worktree, a rename, a machine added or renamed.

import { useState } from "react";
import { readPref, writePref } from "@/lib/prefs";
import { dirname } from "@/lib/text";
import { FormDialog, Icon, Logo, Select, useSubmit } from "@/ui";
import type { AgentKind, Machine, Session, StartExtras } from "./api";
import FolderPicker from "./FolderPicker";
import { machineOptions } from "./options";
import { choiceFor, CUSTOM, EMPTY_DRAFT, hasModelFlag, hasOptions, lastModelKey, modelChoices, modelPlaceholder, optionsFor, type Draft } from "./startOptions";

const ADJ = ["clear", "quiet", "amber", "brisk", "calm", "early", "fresh", "keen", "mild", "swift"];
const NOUN = ["forest", "river", "harbor", "meadow", "summit", "valley", "ridge", "grove", "lake", "field"];
function suggestBranch(): string {
  const pick = (a: string[]) => a[Math.floor(Math.random() * a.length)]!;
  return `worktree/${pick(ADJ)}-${pick(NOUN)}-${1000 + Math.floor(Math.random() * 9000)}`;
}

// defaultAgent is the one a "make something" dialog offers first: claude when herdr knows it, else the first listed
function defaultAgent(agents: AgentKind[], want: string): string {
  return agents.some((a) => a.id === want) ? want : (agents[0]?.id ?? "shell");
}

function AgentSelect({ agents, value, onChange }: { agents: AgentKind[]; value: string; onChange: (v: string) => void }) {
  return (
    <label>
      Agent
      <Select
        value={value}
        onChange={onChange}
        options={[
          ...agents.map((a) => ({ value: a.id, label: a.label, icon: <Logo agent={a.id} /> })),
          { value: "shell", label: "Plain shell", icon: <Icon name="terminal" size={14} /> },
        ]}
      />
    </label>
  );
}

// useStart is the agent a dialog starts and the person's choices for it: the model last used for the kind comes back
// from this device's prefs, a pick of another kind brings that kind's; what the create route takes comes out of options()
function useStart(initial: string) {
  const [agent, setAgent] = useState(initial);
  const [draft, setDraft] = useState<Draft>(() => ({ ...EMPTY_DRAFT, ...choiceFor(initial, readPref(lastModelKey(initial), "")) }));
  const pickAgent = (kind: string) => {
    setAgent(kind);
    setDraft((d) => ({ ...d, ...choiceFor(kind, readPref(lastModelKey(kind), "")) }));
  };
  // options is the request's extras, remembering the model for the kind on the way out
  const options = (): StartExtras => {
    const extras = optionsFor(agent, draft);
    if (hasModelFlag(agent)) writePref(lastModelKey(agent), extras.model ?? "");
    return extras;
  };
  return { agent, pickAgent, draft, setDraft, options };
}

// StartOptionsFields is the folded "Options" under the agent: the model, extra arguments, a first prompt; nothing for a shell
function StartOptionsFields({ kind, draft, onChange }: { kind: string; draft: Draft; onChange: (d: Draft) => void }) {
  const [open, setOpen] = useState(false);
  if (kind === "" || kind === "shell") return null;
  const choices = modelChoices(kind);
  const set = (patch: Partial<Draft>) => onChange({ ...draft, ...patch });
  const typedModel = hasModelFlag(kind) && (choices.length === 0 || draft.choice === CUSTOM);
  return (
    <div className="start-options">
      <button type="button" className="linkish start-options-toggle" aria-expanded={open} onClick={() => setOpen(!open)}>
        <Icon name={open ? "chevron-down" : "chevron-right"} size={12} />
        Options{!open && hasOptions(kind, draft) ? " · set" : ""}
      </button>
      {open && (
        <>
          {hasModelFlag(kind) && choices.length > 0 && (
            <label>
              Model
              <Select value={draft.choice} onChange={(choice) => set({ choice })} options={choices} />
            </label>
          )}
          {typedModel && (
            <label>
              {choices.length > 0 ? "Model id" : "Model"}
              <input value={draft.custom} placeholder={modelPlaceholder(kind)} spellCheck={false} onChange={(e) => set({ custom: e.target.value })} />
            </label>
          )}
          <label>
            Extra arguments
            <input value={draft.args} placeholder="--permission-mode plan" spellCheck={false} onChange={(e) => set({ args: e.target.value })} />
          </label>
          <label>
            First prompt <small>sent once the agent is up</small>
            <textarea value={draft.prompt} rows={3} placeholder="What the agent should start on" onChange={(e) => set({ prompt: e.target.value })} />
          </label>
        </>
      )}
    </div>
  );
}

export function RenameDialog({ what, label, onClose, onRename }: { what: "tab" | "workspace"; label: string; onClose: () => void; onRename: (label: string) => Promise<void> }) {
  const [value, setValue] = useState(label);
  const submit = useSubmit(() => onRename(value.trim()));
  return (
    <FormDialog title={what === "tab" ? "Rename tab" : "Rename workspace"} icon="edit" onClose={onClose} submit={submit} submitLabel="Rename">
      <label>
        Name
        <input autoFocus value={value} onChange={(e) => setValue(e.target.value)} onFocus={(e) => e.target.select()} />
      </label>
    </FormDialog>
  );
}

export function NewWorkspaceDialog({ machines, agents, defaultMachine, onCreate, onClose }: {
  machines: Machine[];
  agents: AgentKind[];
  defaultMachine: string;
  onCreate: (machine: string, dir: string, name: string, agent: string, extras: StartExtras) => Promise<void>;
  onClose: () => void;
}) {
  const [machine, setMachine] = useState(defaultMachine);
  const [dir, setDir] = useState("");
  const [name, setName] = useState("");
  const { agent, pickAgent, draft, setDraft, options: extras } = useStart(defaultAgent(agents, "claude"));
  const [picking, setPicking] = useState(false);
  const submit = useSubmit(async () => {
    if (!dir.trim()) throw new Error("Project folder is required");
    await onCreate(machine, dir.trim(), name.trim(), agent, extras());
  });
  const target = machines.find((m) => m.id === machine);
  const options = machineOptions(machines, { onlyConnected: false }).map((o) => {
    const m = machines.find((x) => x.id === o.value);
    return m && m.state !== "connected" ? { ...o, hint: m.state, disabled: true } : { ...o, hint: m?.host };
  });
  return (
    <FormDialog title="New workspace" icon="folder" onClose={onClose} submit={submit} submitLabel="Create">
      {machines.length > 1 && (
        <label>
          Machine
          <Select value={machine} onChange={setMachine} options={options} />
        </label>
      )}
      <label>
        Project folder{target && target.id !== "local" ? ` on ${target.label}` : ""}
        <span className="field-row">
          <input autoFocus value={dir} placeholder="/Users/you/Projects/app" onChange={(e) => setDir(e.target.value)} />
          <button type="button" className="icon-btn lg" title="Browse the folders on the machine" disabled={!target || target.state !== "connected"} onClick={() => setPicking(true)}>
            <Icon name="folder" size={16} />
          </button>
        </span>
      </label>
      {picking && target && (
        <FolderPicker
          machine={target}
          machines={machines}
          start={dir.trim() ? dirname(dir.trim()) || dir.trim() : ""}
          onPick={(p) => {
            setDir(p);
            setPicking(false);
          }}
          onClose={() => setPicking(false)}
        />
      )}
      <label>
        Name (optional)
        <input value={name} placeholder="checkout-api" onChange={(e) => setName(e.target.value)} />
      </label>
      <AgentSelect agents={agents} value={agent} onChange={pickAgent} />
      <StartOptionsFields kind={agent} draft={draft} onChange={setDraft} />
    </FormDialog>
  );
}

export function NewTabDialog({ session, agents, onCreate, onClose }: {
  session: Session;
  agents: AgentKind[];
  onCreate: (name: string, agent: string, extras: StartExtras) => Promise<void>;
  onClose: () => void;
}) {
  const [name, setName] = useState("");
  const { agent, pickAgent, draft, setDraft, options } = useStart("shell");
  const submit = useSubmit(() => onCreate(name.trim(), agent, options()));
  return (
    <FormDialog title={`New tab in ${session.name}`} icon="plus" hint="Opens in the workspace's folder, like herdr's prefix+c." onClose={onClose} submit={submit} submitLabel="Create tab">
      <label>
        Name (optional)
        <input autoFocus value={name} placeholder="tests" onChange={(e) => setName(e.target.value)} />
      </label>
      <AgentSelect agents={agents} value={agent} onChange={pickAgent} />
      <StartOptionsFields kind={agent} draft={draft} onChange={setDraft} />
    </FormDialog>
  );
}

export function NewWorktreeDialog({ session, agents, onCreate, onClose }: {
  session: Session;
  agents: AgentKind[];
  onCreate: (branch: string, base: string, name: string, agent: string, extras: StartExtras) => Promise<void>;
  onClose: () => void;
}) {
  const [branch, setBranch] = useState(suggestBranch);
  const [base, setBase] = useState("");
  const [name, setName] = useState("");
  const { agent, pickAgent, draft, setDraft, options } = useStart(defaultAgent(agents, "claude"));
  const submit = useSubmit(async () => {
    if (!branch.trim()) throw new Error("Branch name is required");
    await onCreate(branch.trim(), base.trim(), name.trim(), agent, options());
  });
  const repo = session.worktree?.repo_name ?? session.name;
  return (
    <FormDialog
      title={`New worktree of ${repo}`}
      icon="git-branch"
      hint={
        <>
          <code>git worktree add</code> creates the branch's own checkout; it opens as a workspace nested under {repo}, and the agent starts there beside the first one.
        </>
      }
      onClose={onClose}
      submit={submit}
      submitLabel="Create worktree"
    >
      <label>
        Branch
        <input autoFocus value={branch} onChange={(e) => setBranch(e.target.value)} />
      </label>
      <label>
        Base (optional — branch, tag or commit; default: the current HEAD)
        <input value={base} placeholder="main" onChange={(e) => setBase(e.target.value)} />
      </label>
      <label>
        Workspace name (optional)
        <input value={name} placeholder="defaults to the branch" onChange={(e) => setName(e.target.value)} />
      </label>
      <AgentSelect agents={agents} value={agent} onChange={pickAgent} />
      <StartOptionsFields kind={agent} draft={draft} onChange={setDraft} />
    </FormDialog>
  );
}

export function RenameMachineDialog({ machine, onRename, onClose }: { machine: Machine; onRename: (label: string) => Promise<void>; onClose: () => void }) {
  const [label, setLabel] = useState(machine.label);
  const submit = useSubmit(() => onRename(label.trim()));
  const registered = machine.id === "local" ? "this host's name" : machine.source === "herdr" ? "the name in herdr's machine list" : "the name it was added with";
  return (
    <FormDialog title="Rename machine" icon="edit" hint={`Shown everywhere in remuda. Leave it empty to go back to ${registered}.`} onClose={onClose} submit={submit} submitLabel="Rename">
      <label>
        Name
        <input autoFocus value={label} maxLength={60} placeholder={machine.host || machine.label} onChange={(e) => setLabel(e.target.value)} />
      </label>
    </FormDialog>
  );
}

export function AddMachineDialog({ onAdd, onClose }: { onAdd: (label: string, host: string) => Promise<void>; onClose: () => void }) {
  const [host, setHost] = useState("");
  const [label, setLabel] = useState("");
  const submit = useSubmit(async () => {
    if (!host.trim()) throw new Error("SSH host is required");
    await onAdd(label.trim(), host.trim());
  });
  return (
    <FormDialog
      title="Add a remote PC"
      icon="monitor"
      hint={
        <>
          Machines saved in herdr appear on their own. This adds another: any host your <code>ssh</code> reaches with keys, with herdr running there.
        </>
      }
      onClose={onClose}
      submit={submit}
      submitLabel="Add & connect"
    >
      <label>
        SSH host
        <input autoFocus value={host} placeholder="build-box" onChange={(e) => setHost(e.target.value)} />
      </label>
      <label>
        Label (optional)
        <input value={label} placeholder="Build box" onChange={(e) => setLabel(e.target.value)} />
      </label>
    </FormDialog>
  );
}

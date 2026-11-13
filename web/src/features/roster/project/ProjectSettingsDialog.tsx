// A repository's project settings, after Orca's: WORKTREES (the base, the location, the issue's branch), WORKTREE SETUP
// (the team's script read-only, a local one, when it runs, whether the agent waits), NEW WORKTREES START WITH (the agent,
// the issue and pull request prompts), RUN ACTIONS (the ▶ button's commands). Saved whole on the cockpit.

import { useEffect, useState, type ReactNode } from "react";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { Dialog, Icon, Logo, Notice, Select, Tip } from "@/ui";
import { t } from "@/lib/i18n";
import { useRoster } from "../useRoster";
import { useSharing } from "../useSharing";
import { fetchProject, saveProject } from "./api";
import ActionDialog from "./ActionDialog";
import { actionHint, DEFAULTS, HOOK_VARS, sameSettings, VARIABLES, whenWords, withAction, withoutAction, type Action, type SetupWhen, type Settings } from "./project";

const WHENS: SetupWhen[] = ["ask", "", "skip"];

export default function ProjectSettingsDialog({ machineId, root, onClose, onSaved }: { machineId: string; root: string; onClose: () => void; onSaved?: () => void }) {
  const { agents } = useRoster();
  const share = useSharing();
  const project = useQuery(() => fetchProject(machineId, root), [machineId, root]);
  const [draft, setDraft] = useState<Settings | null>(null);
  const [editing, setEditing] = useState<Action | "new" | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    if (project.data && !draft) setDraft(project.data.settings ?? {});
  }, [project.data, draft]);
  const p = project.data;
  const s = draft ?? {};
  const set = (patch: Partial<Settings>) => setDraft({ ...s, ...patch });
  const name = share.redact(p?.name ?? root.split(/[\\/]/).filter(Boolean).pop() ?? root);
  const save = async () => {
    setBusy(true);
    setError("");
    try {
      await saveProject(machineId, root, s);
      onSaved?.();
      onClose();
    } catch (e) {
      setError(errMsg(e));
      setBusy(false);
    }
  };
  const field = (key: keyof Settings, placeholder: string, mono = false) => (
    <input className={mono ? "mono" : undefined} value={(s[key] as string | undefined) ?? ""} placeholder={placeholder} spellCheck={false} onChange={(e) => set({ [key]: e.target.value })} />
  );
  return (
    <Dialog
      as="form"
      size="lg"
      className="repo-settings"
      title={t("Project settings · {name}", { name })}
      icon="settings"
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        void save();
      }}
    >
      {project.loading && <p className="modal-hint">{t("Reading the project…")}</p>}
      {project.error && <Notice tone="error">{project.error}</Notice>}
      {p?.trouble && <Notice tone="warn">{t("The repository could not be read just now ({why}); its own setup script is not shown.", { why: p.trouble })}</Notice>}
      {draft && (
        <>
          <h3 className="rs-section">{t("Worktrees")}</h3>
          <Row label={t("Default worktree base")} hint={t("New branches start from it")}>
            {field("base", p?.base || "origin/main", true)}
          </Row>
          <Row label={t("Worktree location")} hint={t("The folder new worktrees go in")}>
            {field("location", `~/.herdr/worktrees/${p?.name ?? "repo"}`, true)}
          </Row>
          <Row label={t("Issue branch name")} hint={t("{number} and {slug} are filled in")}>
            {field("issue_branch", DEFAULTS.issue_branch, true)}
          </Row>

          <h3 className="rs-section">{t("Worktree setup")}</h3>
          <p className="modal-hint">{t("A script that runs in a new worktree once it is made: install the dependencies, copy the env files. The repository's herdr.yaml (or Orca's orca.yaml) is your team's and runs first; the local one is kept on this cockpit.")}</p>
          {p && !p.setup && <Notice tone="warn">{t("Setup scripts do not run on a Windows PC yet.")}</Notice>}
          <div className="rs-vars">
            {HOOK_VARS.map(([v, about]) => (
              <Tip key={v} text={t(about)}>
                <code tabIndex={0}>${v}</code>
              </Tip>
            ))}
          </div>
          {p?.team && (
            <div className="rs-team">
              <span>{t("From {file}, shared with your team, runs first:", { file: p.team.file })}</span>
              <pre>{p.team.script}</pre>
            </div>
          )}
          <label>
            {t("Local setup script")}
            <textarea className="mono" rows={4} value={s.setup ?? ""} placeholder={"pnpm i\nln -s $HERDR_ROOT_PATH/.env $HERDR_WORKTREE_PATH/.env"} spellCheck={false} onChange={(e) => set({ setup: e.target.value })} />
            <small>{t("Runs as one shell script in your login shell, so what your profile puts on PATH is there. Orca's ORCA_* names work too.")}</small>
          </label>
          <div className="rs-when">
            <span>{t("When to run")}</span>
            <div className="rs-pills">
              {WHENS.map((w) => (
                <button key={w || "run"} type="button" className={`btn sm${(s.setup_when ?? "") === w ? " on" : ""}`} aria-pressed={(s.setup_when ?? "") === w} onClick={() => set({ setup_when: w })}>
                  {whenWords(w)}
                </button>
              ))}
            </div>
          </div>
          <label className="check-row">
            <input type="checkbox" checked={!!s.setup_wait} onChange={(e) => set({ setup_wait: e.target.checked })} />
            <span>
              {t("Wait for setup before starting the agent")}
              <small>{s.setup_wait ? t("Setup runs first, then the agent, in one terminal.") : t("The agent starts at once; setup runs in a Setup tab beside it.")}</small>
            </span>
          </label>

          <h3 className="rs-section">{t("New worktrees start with")}</h3>
          <Row label={t("Default agent")} hint={t("Started in each new worktree")}>
            <Select
              value={s.agent ?? ""}
              onChange={(agent) => set({ agent })}
              options={[
                { value: "", label: t("Last used"), icon: <Icon name="clock" size={14} /> },
                { value: "shell", label: t("None · just a terminal"), icon: <Icon name="terminal" size={14} /> },
                ...agents.map((a) => ({ value: a.id, label: a.label, icon: <Logo agent={a.id} /> })),
              ]}
            />
          </Row>
          <label>
            {t("First prompt for an issue")}
            <textarea rows={3} value={s.issue_prompt ?? ""} placeholder={DEFAULTS.issue_prompt} onChange={(e) => set({ issue_prompt: e.target.value })} />
          </label>
          <label>
            {t("First prompt for a pull request")}
            <textarea rows={3} value={s.pr_prompt ?? ""} placeholder={DEFAULTS.pr_prompt} onChange={(e) => set({ pr_prompt: e.target.value })} />
          </label>
          <div className="rs-vars">
            <span>{t("Filled in:")}</span>
            {VARIABLES.map((v) => (
              <code key={v}>{v}</code>
            ))}
            <span>{t("Blank keeps the wording shown.")}</span>
          </div>

          <h3 className="rs-section">{t("Run actions")}</h3>
          <p className="modal-hint">{t("Commands the ▶ button in the tab strip starts in a new shell: a dev server, the tests.")}</p>
          <div className="rs-actions">
            {(s.actions ?? []).map((a) => (
              <div key={a.id || a.name} className="rs-action">
                <Icon name={a.icon ?? "play"} size={14} />
                <span className="rs-action-name">{a.name}</span>
                <code className="rs-action-hint">{actionHint(a)}</code>
                <button type="button" className="btn ghost sm" onClick={() => setEditing(a)}>
                  {t("Edit")}
                </button>
                <Tip text={t("Remove {name}", { name: a.name })}>
                  <button type="button" className="icon-btn danger" aria-label={t("Remove {name}", { name: a.name })} onClick={() => set({ actions: withoutAction(s.actions, a.id) })}>
                    <Icon name="trash" size={14} />
                  </button>
                </Tip>
              </div>
            ))}
            {(s.actions ?? []).length === 0 && <p className="modal-hint">{t("No run actions yet.")}</p>}
            <button type="button" className="btn sm rs-add" onClick={() => setEditing("new")}>
              <Icon name="plus" size={14} /> {t("Add action")}
            </button>
          </div>
        </>
      )}
      {error && <Notice tone="error">{error}</Notice>}
      <div className="modal-actions">
        <span className="rs-foot-hint">{t("Leave a field blank to keep the usual behaviour.")}</span>
        <button type="button" className="btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button type="submit" className="btn primary" disabled={!draft || busy || (!!project.data && sameSettings(project.data.settings ?? {}, s))}>
          {busy ? "…" : t("Save")}
        </button>
      </div>
      {editing && (
        <ActionDialog
          action={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
          onSave={(a) => {
            set({ actions: withAction(s.actions, a) });
            setEditing(null);
          }}
        />
      )}
    </Dialog>
  );
}

function Row({ label, hint, children }: { label: string; hint: string; children: ReactNode }) {
  return (
    <div className="rs-row">
      <span className="rs-row-label">
        {label}
        <small>{hint}</small>
      </span>
      <div className="rs-row-control">{children}</div>
    </div>
  );
}

// Updates on a machine, from the page: what can be updated (herdr, the agents,
// remuda itself), each shown as the command it would run; a chosen one runs as
// a job whose output streams in. Run sends the command shown back: the server runs that or refuses.

import { useEffect, useRef, useState } from "react";
import { fetchJob, listJobs, listUpdates, startUpdate, type Job, type Update } from "@/features/updates/api";
import type { Machine } from "@/features/roster/api";
import Icon from "@/ui/icons";
import { errMsg } from "@/lib/text";
import { Dialog, Notice } from "@/ui";
import { useQuery } from "@/lib/query";

// missingText says why the update asked for is not on the machine's list
function missingText(what: string): string {
  switch (what) {
    case "remuda":
      return "This remuda cannot be deployed there: that needs a remote machine of the same OS and CPU, and a released build here.";
    case "remuda-release":
      return "No newer release of remuda is known for this machine.";
    case "hooks":
      return "herdr's hooks there are current.";
    case "plugins":
      return "No herdr plugin from GitHub is installed there, or herdr there is older than 0.9.4.";
  }
  return `Nothing to update called “${what}” there any more.`;
}

// lastJob is the job last started per machine, so the dialog opened again shows the one still running
const lastJob = new Map<string, string>();

export default function UpdatesDialog({ machine, what, onClose, onDone }: {
  machine: Machine;
  what?: string; // open on this one straight away
  onClose: () => void;
  onDone?: () => void;
}) {
  const { data, error, loading } = useQuery(() => listUpdates(machine.id), [machine.id], { keep: true });
  const [listTick, setListTick] = useState(0);
  const recent = useQuery(() => listJobs(machine.id), [machine.id, listTick], { keep: true });
  const [picked, setPicked] = useState<Update | null>(null);
  const [job, setJob] = useState<Job | null>(null);
  const [err, setErr] = useState("");
  const [missing, setMissing] = useState("");
  // the one asked for is picked once, on arrival; Back then shows the list
  const autoPicked = useRef(false);
  useEffect(() => {
    if (!what || !data || autoPicked.current) return;
    autoPicked.current = true;
    const u = data.find((u) => u.what === what);
    if (u) setPicked(u);
    else setMissing(missingText(what));
  }, [what, data]);
  useEffect(() => {
    const id = lastJob.get(machine.id);
    if (!id) return;
    fetchJob(id).then((j) => { if (!j.done) setJob(j); }, () => lastJob.delete(machine.id));
  }, [machine.id]);
  useEffect(() => {
    if (!job || job.done) return;
    const id = job.id;
    const t = setInterval(() => {
      fetchJob(id).then(setJob, (e) => {
        clearInterval(t);
        setErr(errMsg(e));
        setJob((j) => (j && j.id === id ? { ...j, done: true, ok: false, error: "lost track of the job" } : j));
      });
    }, 1000);
    return () => clearInterval(t);
  }, [job?.id, job?.done]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (job?.done) onDone?.();
  }, [job?.done]); // eslint-disable-line react-hooks/exhaustive-deps
  const run = async (u: Update) => {
    setErr("");
    try {
      const { job: id } = await startUpdate(machine.id, u.what, u.command);
      lastJob.set(machine.id, id);
      setJob(await fetchJob(id));
    } catch (e) {
      setErr(errMsg(e));
    }
  };
  const log = useRef<HTMLPreElement>(null);
  useEffect(() => {
    const el = log.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [job?.output]);
  const label = machine.label.split(" · ")[0];
  const back = () => {
    setJob(null);
    setPicked(null);
    setErr("");
    setListTick((n) => n + 1);
  };
  const open = (id: string) => {
    fetchJob(id).then((j) => {
      if (!j.done) lastJob.set(machine.id, id);
      setJob(j);
    }, (e) => setErr(errMsg(e)));
  };
  const listShown = !job && !picked;
  useEffect(() => {
    if (!listShown) return;
    const t = setInterval(() => setListTick((n) => n + 1), 4000);
    return () => clearInterval(t);
  }, [listShown]);
  const when = (iso: string) => new Date(iso).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  return (
    <Dialog title={job ? job.title : picked ? picked.title : `Updates on ${label}`} icon="refresh-cw" size="md" onClose={onClose} className="updates">
      {error && <Notice tone="error">{error}</Notice>}
      {err && <Notice tone="error" onDismiss={() => setErr("")}>{err}</Notice>}
      {job ? (
        <>
          {job.command && <code className="updates-cmd">{job.command}</code>}
          <pre className="updates-log" ref={log}>
            {job.output || (job.done ? "" : "…")}
          </pre>
          {job.done ? (
            job.ok ? (
              picked?.what === "remuda-release" && machine.id === "local" ? (
                <Notice tone="info" icon="refresh-cw">
                  Done. remuda is restarting here; the page offers a reload once the new version answers.
                </Notice>
              ) : (
                <Notice tone="success" icon="check">
                  Done.
                </Notice>
              )
            ) : (
              <Notice tone="error">{job.error || "failed"}</Notice>
            )
          ) : (
            <Notice tone="info">
              <span className="spinner" aria-hidden="true" /> Running on {label}… (closing this keeps it running)
            </Notice>
          )}
          <div className="modal-actions">
            {job.done && (
              <button type="button" className="btn" onClick={back}>
                Back
              </button>
            )}
            <button type="button" className="btn primary" onClick={onClose}>
              Close
            </button>
          </div>
        </>
      ) : picked ? (
        <>
          <p className="updates-note">{picked.note}</p>
          <p className="dim">
            {picked.what.startsWith("restart-")
              ? `This goes through herdr on ${label}, pane by pane:`
              : picked.what === "remuda-release"
                ? `This runs on ${label}, as you: the download from GitHub, the check, the install, the restart of remuda there:`
                : `This runs on ${label}, as you, through the shell remuda already uses there:`}
          </p>
          <code className="updates-cmd">{picked.command}</code>
          <div className="modal-actions">
            <button type="button" className="btn" onClick={back}>
              Back
            </button>
            <button type="button" className="btn primary" onClick={() => void run(picked)}>
              <Icon name="play" size={14} /> Run
            </button>
          </div>
        </>
      ) : (
        <>
          {missing && <Notice tone="warn" onDismiss={() => setMissing("")}>{missing}</Notice>}
          {loading && <div className="notice hint">Looking…</div>}
          {data && data.length === 0 && <div className="empty">Nothing here that remuda knows how to update.</div>}
          <div className="updates-list">
            {data?.map((u) => (
              <button key={u.what} type="button" className="updates-row" onClick={() => setPicked(u)}>
                <Icon name={u.what.startsWith("restart-") ? "play" : u.what === "herdr" || u.what === "hooks" || u.what === "plugins" || u.what.startsWith("remuda") ? "refresh-cw" : "sparkles"} size={14} />
                <span className="updates-row-text">
                  <span>{u.title}</span>
                  <span className="dim">{u.note?.split(". ")[0]}</span>
                </span>
                <Icon name="chevron-right" size={12} />
              </button>
            ))}
          </div>
          {recent.data && recent.data.jobs.length > 0 && (
            <div className="updates-recent">
              <span className="dim">
                Ran here lately
                {recent.data.pending > 0 ? ` · ${recent.data.pending} chat${recent.data.pending > 1 ? "s" : ""} waiting to restart` : ""}
              </span>
              {recent.data.jobs.map((j) => (
                <button key={j.id} type="button" className="linkish updates-job" onClick={() => open(j.id)} title={j.error || (j.done ? (j.ok ? "done" : "failed") : "running")}>
                  <Icon name={j.done ? (j.ok ? "check" : "x") : "refresh-cw"} size={12} className={j.done ? (j.ok ? "ok" : "bad") : "spin"} /> {j.title} · {when(j.started)}
                </button>
              ))}
            </div>
          )}
          <div className="modal-actions">
            <button type="button" className="btn" onClick={onClose}>
              Close
            </button>
          </div>
        </>
      )}
    </Dialog>
  );
}

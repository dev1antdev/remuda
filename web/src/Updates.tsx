// Updates on a machine, from the page: what can be updated (herdr, the
// agents, herdr-ui itself on a remote), each shown as the command it
// would run; a chosen one runs as a job whose output streams in. The
// server runs the command shown and nothing else: Run sends it back and
// is refused when the plan has changed since.

import { useEffect, useRef, useState } from "react";
import { fetchJob, listUpdates, startUpdate, type Job, type Machine, type Update } from "./api";
import Icon from "./icons";
import { errMsg } from "./prefs";
import { Dialog, Notice, useAsync } from "./ui";

// the job last started per machine: closing the dialog does not stop a
// job, and opening it again shows the one still running
const lastJob = new Map<string, string>();

export default function UpdatesDialog({ machine, what, onClose, onDone }: {
  machine: Machine;
  what?: string; // open on this one straight away
  onClose: () => void;
  onDone?: () => void;
}) {
  const { data, error, loading } = useAsync(() => listUpdates(machine.id), [machine.id]);
  const [picked, setPicked] = useState<Update | null>(null);
  const [job, setJob] = useState<Job | null>(null);
  const [err, setErr] = useState("");
  const [missing, setMissing] = useState(""); // `what` asked for, not offered
  // the one asked for is picked once, on arrival; Back then shows the list
  const autoPicked = useRef(false);
  useEffect(() => {
    if (!what || !data || autoPicked.current) return;
    autoPicked.current = true;
    const u = data.find((u) => u.what === what);
    if (u) setPicked(u);
    else setMissing(what === "herdr-ui" ? "This herdr-ui cannot be deployed there: that needs a remote machine of the same OS and CPU, and a released build here." : `Nothing to update called “${what}” there any more.`);
  }, [what, data]);
  // a job still running from last time
  useEffect(() => {
    const id = lastJob.get(machine.id);
    if (!id) return;
    fetchJob(id).then((j) => { if (!j.done) setJob(j); }, () => lastJob.delete(machine.id));
  }, [machine.id]);
  // the running job, polled while it runs; an error ends the polling
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
  };
  return (
    <Dialog title={job ? job.title : picked ? picked.title : `Updates on ${label}`} icon="refresh-cw" size="md" onClose={onClose} className="updates">
      {error && <Notice tone="error">{error}</Notice>}
      {err && <Notice tone="error" onDismiss={() => setErr("")}>{err}</Notice>}
      {job ? (
        <>
          <code className="updates-cmd">{job.command}</code>
          <pre className="updates-log" ref={log}>
            {job.output || (job.done ? "" : "…")}
          </pre>
          {job.done ? (
            job.ok ? (
              <Notice tone="success" icon="check">Done.</Notice>
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
          <p className="dim">This runs on {label}, as you, through the shell herdr-ui already uses there:</p>
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
          {data && data.length === 0 && <div className="empty">Nothing here that herdr-ui knows how to update.</div>}
          <div className="updates-list">
            {data?.map((u) => (
              <button key={u.what} type="button" className="updates-row" onClick={() => setPicked(u)}>
                <Icon name={u.what === "herdr" || u.what === "herdr-ui" ? "refresh-cw" : "sparkles"} size={14} />
                <span className="updates-row-text">
                  <span>{u.title}</span>
                  <span className="dim">{u.note?.split(". ")[0]}</span>
                </span>
                <Icon name="chevron-right" size={12} />
              </button>
            ))}
          </div>
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

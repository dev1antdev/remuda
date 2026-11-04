// QueueList is the notes queued for the agent, above the composer: each sent on its own or all as one numbered
// message, only on a press; a note's text clicked goes back into the box to be edited; × drops it.

import { Icon, Tip } from "@/ui";
import { noteLine, type QueuedNote } from "./queue";
import { tn, t } from "@/lib/i18n";

export function QueueList({ notes, busy, onSend, onEdit, onRemove }: {
  notes: QueuedNote[];
  busy: boolean;
  onSend: (notes: QueuedNote[]) => void;
  onEdit: (note: QueuedNote) => void;
  onRemove: (note: QueuedNote) => void;
}) {
  return (
    <div className="chat-queue" aria-label={t("Notes queued for the agent")}>
      <div className="chat-queue-head">
        <Icon name="inbox" size={12} />
        <span className="chat-queue-count">{tn(notes.length, "{n} note queued for the agent — nothing goes until you send it", "{n} notes queued for the agent — nothing goes until you send them")}</span>
        {notes.length > 1 && (
          <button type="button" className="btn sm" disabled={busy} onClick={() => onSend(notes)}>
            {t("Send all as one message")}
          </button>
        )}
      </div>
      {notes.map((note) => {
        const line = noteLine(note);
        const more = note.text.length > line.length;
        return (
          <div key={note.id} className="chat-queue-row">
            <button type="button" className="linkish chat-queue-text" title={`${note.text}\n\n${t("Click to put it back in the box")}`} onClick={() => onEdit(note)}>
              {line}
              {more && <span className="chat-queue-more"> …</span>}
            </button>
            <button type="button" className="btn sm" disabled={busy} onClick={() => onSend([note])}>
              {t("Send")}
            </button>
            <Tip text={t("Drop this note")}>
              <button type="button" className="icon-btn sm" aria-label={t("Drop this note")} disabled={busy} onClick={() => onRemove(note)}>
                <Icon name="x" size={12} />
              </button>
            </Tip>
          </div>
        );
      })}
    </div>
  );
}

import { useCallback, useEffect, useRef, useState } from "react";
import { uploadFiles, type Upload } from "./api";
import { errMsg } from "./prefs";

// Attachments: files from the phone's picker, the clipboard (a pasted
// screenshot) or a drop. Each is uploaded into the pane's folder at once; the
// agent only hears about it when the user sends the message that mentions it.

export type Attachment = {
  id: string;
  file: File;
  thumb: string | null; // object URL for images
  upload: Upload | null;
  error: string | null;
};

export const MAX_ATTACHMENTS = 4;

const IMAGE = /^image\/(png|jpeg|gif|webp|svg\+xml)$/;

// filesFrom pulls files out of a drop or paste; a pasted screenshot arrives as "image.png" and gets a name.
export function filesFrom(dt: DataTransfer | null): File[] {
  if (!dt) return [];
  const out: File[] = [];
  if (dt.items) {
    for (const item of Array.from(dt.items)) {
      if (item.kind !== "file") continue;
      const f = item.getAsFile();
      if (f) out.push(f);
    }
  } else {
    out.push(...Array.from(dt.files));
  }
  return out.map((f) =>
    f.name && f.name !== "image.png" ? f : new File([f], `clip-${Date.now()}.${(f.type.split("/")[1] ?? "bin").replace("+xml", "")}`, { type: f.type }),
  );
}

// shellQuote wraps a path for a POSIX shell (the terminal lens types paths).
export function shellQuote(p: string): string {
  return /^[\w./-]+$/.test(p) ? p : "'" + p.replace(/'/g, "'\\''") + "'";
}

export function useAttachments(machineId: string, paneId: string) {
  const [items, setItems] = useState<Attachment[]>([]);
  const seq = useRef(0);
  const itemsRef = useRef(items);
  itemsRef.current = items;

  const revoke = (list: Attachment[]) => list.forEach((a) => a.thumb && URL.revokeObjectURL(a.thumb));
  useEffect(() => () => revoke(itemsRef.current), []);

  const add = useCallback(
    async (files: File[]): Promise<Upload[]> => {
      if (files.length === 0) return [];
      // itemsRef, not a setItems updater: updaters run at render time and would see a stale count
      const batch = files.slice(0, Math.max(0, MAX_ATTACHMENTS - itemsRef.current.length));
      if (batch.length === 0) return [];
      const fresh: Attachment[] = batch.map((file) => ({
        id: `a${++seq.current}`,
        file,
        thumb: IMAGE.test(file.type) ? URL.createObjectURL(file) : null,
        upload: null,
        error: null,
      }));
      itemsRef.current = [...itemsRef.current, ...fresh]; // a second drop in the same tick sees these
      setItems((cur) => [...cur, ...fresh]);
      try {
        const ups = await uploadFiles(machineId, paneId, batch);
        setItems((cur) =>
          cur.map((a) => {
            const i = fresh.findIndex((f) => f.id === a.id);
            return i >= 0 ? { ...a, upload: ups[i] ?? null } : a;
          }),
        );
        // only the ones still wanted: a chip removed mid-upload must not come back as a mention in the draft
        const kept = new Set(itemsRef.current.map((a) => a.id));
        return ups.filter((_, i) => kept.has(fresh[i]!.id));
      } catch (e) {
        const msg = errMsg(e);
        setItems((cur) => cur.map((a) => (fresh.some((f) => f.id === a.id) ? { ...a, error: msg } : a)));
        return [];
      }
    },
    [machineId, paneId],
  );

  // remove and clear update the ref at once: an upload finishing a moment later checks it
  const remove = useCallback((id: string) => {
    revoke(itemsRef.current.filter((a) => a.id === id));
    itemsRef.current = itemsRef.current.filter((a) => a.id !== id);
    setItems(itemsRef.current);
  }, []);
  const clear = useCallback(() => {
    revoke(itemsRef.current);
    itemsRef.current = [];
    setItems([]);
  }, []);

  return { items, add, remove, clear, uploading: items.some((a) => !a.upload && !a.error) };
}

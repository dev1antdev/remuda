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

// filesFrom pulls files out of a drop or paste; a pasted screenshot arrives
// as an "image.png" item with no name worth keeping.
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

  // object URLs are revoked when their item goes away, and all on unmount
  const revoke = (list: Attachment[]) => list.forEach((a) => a.thumb && URL.revokeObjectURL(a.thumb));
  useEffect(() => () => revoke(itemsRef.current), []);

  const add = useCallback(
    async (files: File[]): Promise<Upload[]> => {
      if (files.length === 0) return [];
      let room = MAX_ATTACHMENTS;
      setItems((cur) => {
        room = MAX_ATTACHMENTS - cur.length;
        return cur;
      });
      const batch = files.slice(0, Math.max(0, room));
      if (batch.length === 0) return [];
      const fresh: Attachment[] = batch.map((file) => ({
        id: `a${++seq.current}`,
        file,
        thumb: IMAGE.test(file.type) ? URL.createObjectURL(file) : null,
        upload: null,
        error: null,
      }));
      setItems((cur) => [...cur, ...fresh]);
      try {
        const ups = await uploadFiles(machineId, paneId, batch);
        setItems((cur) =>
          cur.map((a) => {
            const i = fresh.findIndex((f) => f.id === a.id);
            return i >= 0 ? { ...a, upload: ups[i] ?? null } : a;
          }),
        );
        return ups;
      } catch (e) {
        const msg = errMsg(e);
        setItems((cur) => cur.map((a) => (fresh.some((f) => f.id === a.id) ? { ...a, error: msg } : a)));
        return [];
      }
    },
    [machineId, paneId],
  );

  const remove = useCallback(
    (id: string) =>
      setItems((cur) => {
        revoke(cur.filter((a) => a.id === id));
        return cur.filter((a) => a.id !== id);
      }),
    [],
  );
  const clear = useCallback(
    () =>
      setItems((cur) => {
        revoke(cur);
        return [];
      }),
    [],
  );

  return { items, add, remove, clear, uploading: items.some((a) => !a.upload && !a.error) };
}

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

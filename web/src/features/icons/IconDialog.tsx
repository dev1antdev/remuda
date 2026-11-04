// IconDialog gives a folder its icon: an emoji from the system's set, by group or by name; a site's favicon, read once
// by remuda at the person's word; or a picture from their computer. Remove goes back to the initial on a tile.

import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { act } from "@/app/act";
import type { FolderIconRef } from "@/features/roster/api";
import { usePref } from "@/lib/prefs";
import { Dialog, FolderIcon, Notice, useSubmit } from "@/ui";
import { t } from "@/lib/i18n";
import { clearFolderIcon, fetchFavicon, setFolderIcon, type IconChoice } from "./api";
import { pushRecent, searchEmoji } from "./emoji";
import type { EmojiGroup } from "./emojiData";
import { ICON_SIZE, fitInside, pictureKind, siteOf } from "./picture";

type Source = "emoji" | "site" | "picture";

// GROUP_FACES is the emoji each group's button wears
const GROUP_FACES: Record<string, string> = { smileys: "😀", people: "👋", animals: "🦊", food: "🍎", travel: "✈️", activities: "⚽", objects: "💡", symbols: "💠", flags: "🏁" };

export default function IconDialog({ machineId, folder, name, icon, onClose }: { machineId: string; folder: string; name: string; icon?: FolderIconRef; onClose: () => void }) {
  const [source, setSource] = useState<Source>("emoji");
  const [recent, setRecent] = usePref<string[]>("icons.recent", []);
  const save = useSubmit(async (choice: IconChoice) => {
    await setFolderIcon(machineId, folder, choice);
    if ("emoji" in choice) setRecent((r) => pushRecent(r, choice.emoji));
    await act(Promise.resolve());
    onClose();
  });
  const remove = useSubmit(async () => {
    await clearFolderIcon(machineId, folder);
    await act(Promise.resolve());
    onClose();
  });
  const tab = (id: Source, label: string) => (
    <button type="button" role="tab" aria-selected={source === id} className={source === id ? "on" : ""} onClick={() => setSource(id)}>
      {label}
    </button>
  );
  return (
    <Dialog title={t("Icon for {name}", { name })} icon="folder" size="md" onClose={onClose} className="icon-dialog">
      <div className="icon-head">
        <FolderIcon icon={icon} name={name} size={32} />
        <div className="fv-tabs" role="tablist" aria-label={t("Where the icon comes from")}>
          {tab("emoji", t("Emoji"))}
          {tab("site", t("A site's icon"))}
          {tab("picture", t("A picture"))}
        </div>
      </div>
      {source === "emoji" && <EmojiPicker recent={recent} busy={save.busy} onPick={(emoji) => void save.run({ emoji })} />}
      {source === "site" && <SiteIcon busy={save.busy} onUse={(image) => void save.run({ image })} />}
      {source === "picture" && <PictureIcon busy={save.busy} onUse={(image) => void save.run({ image })} />}
      {(save.err || remove.err) && <Notice tone="error">{save.err || remove.err}</Notice>}
      <div className="modal-actions">
        {(icon?.emoji || icon?.image) && (
          <button type="button" className="btn danger" disabled={remove.busy} onClick={() => void remove.run()}>
            {t("Remove the icon")}
          </button>
        )}
        <button type="button" className="btn" onClick={onClose}>
          {t("Close")}
        </button>
      </div>
    </Dialog>
  );
}

// EmojiPicker is the system's emoji by group, the recent ones first, or the ones whose names match what is typed
function EmojiPicker({ recent, busy, onPick }: { recent: string[]; busy: boolean; onPick: (emoji: string) => void }) {
  const [groups, setGroups] = useState<EmojiGroup[] | null>(null);
  const [query, setQuery] = useState("");
  const scroller = useRef<HTMLDivElement>(null);
  useEffect(() => {
    let live = true;
    void import("./emojiData").then((data) => live && setGroups(data.EMOJI_GROUPS));
    return () => {
      live = false;
    };
  }, []);
  const found = useMemo(() => (groups && query.trim() ? searchEmoji(groups, query) : null), [groups, query]);
  const jump = (id: string) => scroller.current?.querySelector(`[data-group="${id}"]`)?.scrollIntoView({ block: "start" });
  const grid = (list: [string, string][]) => (
    <div className="emoji-grid">
      {list.map(([emoji, label]) => (
        <button key={emoji} type="button" className="icon-btn emoji-cell" disabled={busy} title={label || emoji} aria-label={label || emoji} onClick={() => onPick(emoji)}>
          {emoji}
        </button>
      ))}
    </div>
  );
  return (
    <>
      <input autoFocus type="search" className="emoji-search" placeholder={t("Search by name: fox, rocket, flag ukraine")} value={query} onChange={(e) => setQuery(e.target.value)} />
      {!found && groups && (
        <div className="emoji-groups">
          {groups.map((g) => (
            <button key={g.id} type="button" className="btn sm ghost" onClick={() => jump(g.id)}>
              {GROUP_FACES[g.id] ?? g.emoji[0]?.[0]} {t(g.name)}
            </button>
          ))}
        </div>
      )}
      <div className="emoji-scroll" ref={scroller}>
        {!groups && <p className="dim">{t("Reading the emoji…")}</p>}
        {found && (found.length ? grid(found) : <p className="dim">{t("No emoji by that name — the names are in English")}</p>)}
        {!found && groups && (
          <>
            {recent.length > 0 && (
              <section>
                <h4>{t("Recent")}</h4>
                {grid(recent.map((emoji) => [emoji, ""]))}
              </section>
            )}
            {groups.map((g) => (
              <section key={g.id} data-group={g.id}>
                <h4>{t(g.name)}</h4>
                {grid(g.emoji)}
              </section>
            ))}
          </>
        )}
      </div>
    </>
  );
}

// SiteIcon has remuda read a site's favicon once, shows it, and uses it when asked
function SiteIcon({ busy, onUse }: { busy: boolean; onUse: (image: string) => void }) {
  const [site, setSite] = useState("");
  const [found, setFound] = useState<{ data: string; from: string } | null>(null);
  const read = useSubmit(async () => {
    setFound(null);
    const url = siteOf(site);
    if (!url) throw new Error(t("Type the site's address"));
    setFound(await fetchFavicon(url));
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    void read.run();
  };
  return (
    <form className="icon-source" onSubmit={submit}>
      <p className="modal-hint">{t("remuda reads the site's icon once, now, and keeps a copy; nothing is fetched later.")}</p>
      <div className="icon-row">
        <input autoFocus value={site} placeholder="gog.com" spellCheck={false} onChange={(e) => setSite(e.target.value)} />
        <button type="submit" className="btn" disabled={read.busy}>
          {read.busy ? "…" : t("Read the icon")}
        </button>
      </div>
      {read.err && <Notice tone="error">{read.err}</Notice>}
      {found && (
        <div className="icon-found">
          <img src={found.data} alt="" width={32} height={32} />
          <span className="icon-from">{found.from}</span>
          <button type="button" className="btn primary" disabled={busy} onClick={() => onUse(found.data)}>
            {t("Use this icon")}
          </button>
        </div>
      )}
    </form>
  );
}

// PictureIcon takes a picture from the computer: an SVG as it is, anything else drawn into a small square
function PictureIcon({ busy, onUse }: { busy: boolean; onUse: (image: string) => void }) {
  const [picture, setPicture] = useState("");
  const [err, setErr] = useState("");
  const pick = async (file: File) => {
    setErr("");
    setPicture("");
    const kind = pictureKind(file.type, file.size);
    if (typeof kind === "object") return setErr(t(kind.refused));
    try {
      setPicture(kind === "svg" ? await dataURLOf(file) : await drawn(file));
    } catch {
      setErr(t("This picture could not be read"));
    }
  };
  return (
    <div className="icon-source">
      <p className="modal-hint">{t("A PNG, JPEG, GIF, WebP, ICO or SVG: a company's logo, say. It is made into a small square icon.")}</p>
      <label className="btn">
        {t("Choose a picture…")}
        <input
          type="file"
          hidden
          accept="image/png,image/jpeg,image/gif,image/webp,image/svg+xml,image/x-icon,.ico"
          onChange={(e) => {
            const file = e.target.files?.[0];
            if (file) void pick(file);
            e.target.value = "";
          }}
        />
      </label>
      {err && <Notice tone="error">{err}</Notice>}
      {picture && (
        <div className="icon-found">
          <img src={picture} alt="" width={32} height={32} />
          <button type="button" className="btn primary" disabled={busy} onClick={() => onUse(picture)}>
            {t("Use this icon")}
          </button>
        </div>
      )}
    </div>
  );
}

function dataURLOf(file: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

// drawn is the picture fitted into a square of ICON_SIZE pixels, as a PNG
async function drawn(file: File): Promise<string> {
  const url = URL.createObjectURL(file);
  try {
    const image = new Image();
    image.src = url;
    await image.decode();
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = ICON_SIZE;
    const box = fitInside(image.naturalWidth, image.naturalHeight, ICON_SIZE);
    canvas.getContext("2d")!.drawImage(image, box.x, box.y, box.w, box.h);
    return canvas.toDataURL("image/png");
  } finally {
    URL.revokeObjectURL(url);
  }
}

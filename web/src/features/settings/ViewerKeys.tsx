// The viewer links, the rows after the agents' keys in the Devices section: a link signs a TV or a colleague in
// read-only — the Wall, the chats and the screens, nothing that types. Make one under a name, the link shows this
// once with Copy, a Revoke behind a confirm signs every screen it opened out.

import { useState, type FormEvent } from "react";
import { actions } from "@/app/store";
import { copyText } from "@/lib/host";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { fetchViewerKeys, makeViewerKey, revokeViewerKey, type ViewerKey } from "./api";
import { revokeViewerQuestion, viewerLine } from "./viewerLinks";
import { t } from "@/lib/i18n";

export default function ViewerKeys() {
  const { data: keys, error, reload } = useQuery<ViewerKey[]>(() => fetchViewerKeys(), []);
  const [name, setName] = useState("");
  const [made, setMade] = useState<{ name: string; link: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const now = Date.now();
  const fail = (e: unknown) => actions.notify(errMsg(e), { error: true });

  const make = (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim() || busy) return;
    setBusy(true);
    makeViewerKey(name.trim()).then(
      (res) => {
        setMade({ name: res.name, link: res.link });
        setName("");
        setBusy(false);
        reload();
      },
      (err) => {
        setBusy(false);
        fail(err);
      },
    );
  };
  const copy = () => {
    if (!made) return;
    void copyText(made.link).then(() => actions.notify(t("Link copied"), { brief: true }), fail);
  };
  const revoke = (key: ViewerKey) => {
    const { text, label } = revokeViewerQuestion(key);
    actions.ask(text, label, () =>
      void revokeViewerKey(key.id).then(({ screens }) => {
        actions.notify(screens > 0 ? t("The link “{name}” is revoked; {n} screens signed out", { name: key.name, n: screens }) : t("The link “{name}” is revoked", { name: key.name }), { brief: true });
        if (made?.name === key.name) setMade(null);
        reload();
      }, fail),
    );
  };

  return (
    <>
      <div className="settings-row device-row keys-head">
        <span>
          {t("Viewer links")}
          <small>{t("A link that signs a screen in to look only: the Wall, the chats, the screens and the reports, never a key or a word into an agent. For the TV in the office, or a colleague watching a run.")}</small>
        </span>
      </div>
      {error && <div className="settings-row device-row"><span><small>{error}</small></span></div>}
      {keys?.map((key) => (
        <div key={key.id} className="settings-row device-row key-row">
          <span>
            <span className="device-head">{key.name}</span>
            <small>{viewerLine(key, now)}</small>
          </span>
          <button type="button" className="btn danger sm" onClick={() => revoke(key)}>
            {t("Revoke")}
          </button>
        </div>
      ))}
      {made && (
        <div className="settings-row device-row viewer-link-row">
          <span>
            <span className="device-head">{made.name}</span>
            <small>{t("Shown this once: open it on the screen that should view, then keep it to yourself.")}</small>
          </span>
          <span className="viewer-link">
            <code>{made.link}</code>
            <button type="button" className="btn sm" onClick={copy}>
              {t("Copy")}
            </button>
            <button type="button" className="btn ghost sm" onClick={() => setMade(null)} aria-label={t("Hide the link")}>
              {t("Hide")}
            </button>
          </span>
        </div>
      )}
      <form className="settings-row device-row viewer-link-form" onSubmit={make}>
        <input className="sm" value={name} placeholder={t("Whose screen: the office TV")} aria-label={t("The link's name")} onChange={(e) => setName(e.target.value)} />
        <button type="submit" className="btn sm" disabled={!name.trim() || busy}>
          {t("Make a link")}
        </button>
      </form>
    </>
  );
}

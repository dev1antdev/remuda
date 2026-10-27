// The agents' keys, the rows at the end of the Devices section: one per machine whose agents reach the tasks here,
// what it opens and when it was used, a Revoke behind a confirm.

import { actions } from "@/app/store";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { fetchKeys, revokeKey, type AgentKey } from "./api";
import { keyLine, revokeKeyQuestion } from "./keys";
import { t } from "@/lib/i18n";

export default function AgentKeys() {
  const { data: keys, error, reload } = useQuery<AgentKey[]>(() => fetchKeys(), []);
  const now = Date.now();
  const fail = (e: unknown) => actions.notify(errMsg(e), { error: true });
  const revoke = (key: AgentKey) => {
    const { text, label } = revokeKeyQuestion(key);
    actions.ask(text, label, () =>
      void revokeKey(key.id).then(() => {
        actions.notify(`The key of ${key.name} is revoked`, { brief: true });
        reload();
      }, fail),
    );
  };
  return (
    <>
      <div className="settings-row device-row keys-head">
        <span>
          {t("Agent keys")}
          <small>What the agents on each machine use to read and write your tasks through remuda mcp and remuda tasks, and to read who is doing what (the roster, a recap, the standup): never the rest. A revoked one is replaced when remuda next connects to that machine (this one at its next start).</small>
        </span>
      </div>
      {error && <div className="settings-row device-row"><span><small>{error}</small></span></div>}
      {keys?.length === 0 && <div className="settings-row device-row"><span><small>{t("No key yet: one appears as each machine connects.")}</small></span></div>}
      {keys?.map((key) => (
        <div key={key.id} className="settings-row device-row key-row">
          <span>
            <span className="device-head">{key.name}</span>
            <small>{keyLine(key, now)}</small>
          </span>
          <button type="button" className="btn danger sm" onClick={() => revoke(key)}>
            {t("Revoke")}
          </button>
        </div>
      ))}
    </>
  );
}

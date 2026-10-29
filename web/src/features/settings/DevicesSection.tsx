// The signed-in devices, a section of Settings: each with a name of its own (a click on it edits), what it is and when
// it was last seen, a Revoke behind a confirm; under the list, every other device signed out at once, then the
// agents' keys (AgentKeys.tsx).

import { useState, type KeyboardEvent } from "react";
import { actions } from "@/app/store";
import { useQuery } from "@/lib/query";
import { errMsg, plural } from "@/lib/text";
import AgentKeys from "./AgentKeys";
import ViewerKeys from "./ViewerKeys";
import { fetchDevices, renameDevice, revokeDevice, revokeOtherDevices, type Device } from "./api";
import { deviceLine, revokeQuestion } from "./devices";
import { t } from "@/lib/i18n";

export default function DevicesSection() {
  const { data: devices, error, reload, mutate } = useQuery<Device[]>(() => fetchDevices(), []);
  const [editing, setEditing] = useState<string | null>(null);
  const now = Date.now();
  const fail = (e: unknown) => actions.notify(errMsg(e), { error: true });

  const rename = (device: Device, name: string) => {
    setEditing(null);
    if (name.trim() === device.name) return;
    mutate((list) => list?.map((d) => (d.id === device.id ? { ...d, name: name.trim() || d.kind } : d)) ?? null);
    renameDevice(device.id, name).then(reload, (e) => {
      fail(e);
      reload();
    });
  };
  const revoke = (device: Device) => {
    const { text, label } = revokeQuestion(device);
    actions.ask(text, label, () =>
      void revokeDevice(device.id).then(({ self }) => {
        if (self) location.reload();
        else {
          actions.notify(t("{device} signed out", { device: device.name }), { brief: true });
          reload();
        }
      }, fail),
    );
  };
  const revokeOthers = () =>
    actions.ask(t("Sign out every other device? Each needs the token to sign in again."), "Sign out others", () =>
      void revokeOtherDevices().then(({ revoked }) => {
        actions.notify(t("{devices} signed out", { devices: plural(revoked, "device") }), { brief: true });
        reload();
      }, fail),
    );
  const onKey = (device: Device) => (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") rename(device, e.currentTarget.value);
    if (e.key === "Escape") setEditing(null);
  };

  return (
    <section className="settings-section">
      <h3>{t("Devices")}</h3>
      {error && <div className="settings-row device-row"><span><small>{error}</small></span></div>}
      {devices?.map((device) => (
        <div key={device.id} className="settings-row device-row">
          <span>
            <span className="device-head">
              {editing === device.id ? (
                <input className="sm device-name-field" defaultValue={device.name} autoFocus aria-label={t("Device name")} onKeyDown={onKey(device)} onBlur={(e) => rename(device, e.currentTarget.value)} />
              ) : (
                <button type="button" className="linkish device-name" title={t("Rename this device")} onClick={() => setEditing(device.id)}>
                  {device.name}
                </button>
              )}
              {device.this && <span className="badge quiet">{t("this device")}</span>}
              {device.viewer && <span className="badge quiet">{t("viewer")}</span>}
            </span>
            <small>{deviceLine(device, now)}</small>
          </span>
          <button type="button" className="btn danger sm" onClick={() => revoke(device)}>
            {device.this ? t("Sign out") : t("Revoke")}
          </button>
        </div>
      ))}
      {devices && devices.length > 1 && (
        <div className="settings-row device-row devices-foot">
          <span>
            <small>Lost a phone? Every other device is signed out at once; this one stays.</small>
          </span>
          <button type="button" className="btn sm" onClick={revokeOthers}>
            {t("Sign out every other device")}
          </button>
        </div>
      )}
      <AgentKeys />
      <ViewerKeys />
    </section>
  );
}

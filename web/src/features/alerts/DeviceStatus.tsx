// The alerts panel's word on this very device: which browser on which system, whether the site may notify, whether
// this cockpit knows the browser's subscription and what it last handed the push service, what the browser last
// received, and where the system may still keep a notification from appearing.

import { useEffect, useState } from "react";
import { Notice } from "@/ui";
import { t } from "@/lib/i18n";
import { blockedHelp, browserOf, deliveryWords, receivedWords, systemHelp, systemOf, type Received } from "./device";
import type { PushState } from "./push";
import { fetchDevice, onPushed, readLastPush, type DeviceOnServer } from "./received";

export default function DeviceStatus({ state, version }: { state: PushState; version: number }) {
  const browser = browserOf(navigator.userAgent);
  const system = systemOf(navigator.userAgent, navigator.maxTouchPoints);
  const [onServer, setOnServer] = useState<DeviceOnServer | null>(null);
  const [received, setReceived] = useState<Received | null>(null);
  useEffect(() => {
    setOnServer(null);
    if (state.endpoint) void fetchDevice(state.endpoint).then(setOnServer, () => setOnServer(null));
  }, [state.endpoint, version]);
  useEffect(() => {
    void readLastPush().then(setReceived);
    return onPushed((next) => {
      setReceived(next);
      if (state.endpoint) void fetchDevice(state.endpoint).then(setOnServer, () => {});
    });
  }, [state.endpoint]);

  const subscription = !state.subscribed
    ? t("This browser is not subscribed to this cockpit: the switches below shape only what shows on this page. Turn on for notifications with the page closed.")
    : onServer && !onServer.known
      ? t("This browser holds a subscription this cockpit does not know — one made for another address. Turn off and on again.")
      : t("This browser is subscribed to this cockpit.");
  return (
    <div className="alerts-device">
      <div className="alerts-device-head">
        <b>{t("{browser} on {system}", { browser: browser === "browser" ? t("This browser") : browser, system: system === "system" ? t("this system") : system })}</b>
        <span className={`alerts-device-permission is-${state.permission}`}>
          {state.permission === "granted" ? t("notifications allowed") : state.permission === "denied" ? t("notifications blocked") : state.permission === "default" ? t("not asked yet") : t("no notifications here")}
        </span>
      </div>
      {state.permission === "denied" && <Notice tone="warn">{blockedHelp(browser)}</Notice>}
      <ul className="alerts-device-lines">
        <li>{subscription}</li>
        {state.subscribed && onServer?.known && <li>{deliveryWords(onServer.last)}</li>}
        <li>{receivedWords(received)}</li>
        <li>{t("While this page is in front, every alert below shows in its corner as well.")}</li>
      </ul>
      {state.permission === "granted" && <p className="modal-hint">{systemHelp(browser, system)}</p>}
    </div>
  );
}

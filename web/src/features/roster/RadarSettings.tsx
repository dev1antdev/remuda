// The sidebar's settings: the order, what a row is labelled with, the idle tiers, whether shells are listed — the fields
// the Settings dialog shows.

import { useEffect, useState } from "react";
import { Icon, Select } from "@/ui";
import type { RadarPrefs } from "./radar";

export function RadarFields({ prefs, onPrefs }: { prefs: RadarPrefs; onPrefs: (p: RadarPrefs) => void }) {
  return (
    <>
      <label className="settings-row">
        <span>Order</span>
        <Select
          value={prefs.order}
          onChange={(order) => onPrefs({ ...prefs, order })}
          options={[
            { value: "active", label: "grouped by activity", icon: <Icon name="arrow-up-down" size={14} /> },
            { value: "recent", label: "flat, by activity", icon: <Icon name="list" size={14} /> },
            { value: "herdr", label: "herdr's own order", icon: <Icon name="layers" size={14} /> },
          ]}
        />
      </label>
      <label className="settings-row">
        <span>Row label</span>
        <Select
          value={prefs.rowLabel}
          onChange={(rowLabel) => onPrefs({ ...prefs, rowLabel })}
          options={[
            { value: "title", label: "the session's title" },
            { value: "tab", label: "the tab's name" },
            { value: "both", label: "tab · title" },
          ]}
        />
      </label>
      <label className="settings-row">
        <span>
          Fresh for
          <small>minutes an idle agent still counts as fresh</small>
        </span>
        <MinutesInput value={prefs.freshMinutes} onChange={(freshMinutes) => onPrefs({ ...prefs, freshMinutes })} />
      </label>
      <label className="settings-row">
        <span>
          Stale after
          <small>minutes idle before a row dims</small>
        </span>
        <MinutesInput value={prefs.staleMinutes} onChange={(staleMinutes) => onPrefs({ ...prefs, staleMinutes })} />
      </label>
      <label className="settings-row">
        <span>List shell panes too</span>
        <input type="checkbox" checked={prefs.shells} onChange={(e) => onPrefs({ ...prefs, shells: e.target.checked })} />
      </label>
    </>
  );
}

// MinutesInput keeps its text local so the field can be cleared and retyped; a whole positive number saves as typed
function MinutesInput({ value, onChange }: { value: number; onChange: (n: number) => void }) {
  const [text, setText] = useState(String(value));
  useEffect(() => setText(String(value)), [value]);
  return (
    <input
      type="number"
      className="sm settings-minutes"
      min={1}
      inputMode="numeric"
      value={text}
      onChange={(e) => {
        setText(e.target.value);
        const n = Number(e.target.value);
        if (e.target.value !== "" && Number.isInteger(n) && n > 0) onChange(n);
      }}
      onBlur={() => setText(String(value))}
    />
  );
}

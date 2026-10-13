// The sidebar's settings popover: the order, what a row is labelled with, the idle tiers, whether shells are listed.

import { useEffect, useState } from "react";
import { Icon, Select } from "@/ui";
import type { RadarPrefs } from "./radar";

export function RadarSettings({ at, prefs, onPrefs }: { at: { x: number; y: number }; prefs: RadarPrefs; onPrefs: (p: RadarPrefs) => void }) {
  return (
    <div className="row-menu radar-settings" role="dialog" aria-label="Sidebar settings" style={{ left: at.x, top: at.y }}>
      <label>
        Order
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
      <label>
        Row label
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
      <label>
        Fresh for (min)
        <MinutesInput value={prefs.freshMinutes} onChange={(freshMinutes) => onPrefs({ ...prefs, freshMinutes })} />
      </label>
      <label>
        Stale after (min)
        <MinutesInput value={prefs.staleMinutes} onChange={(staleMinutes) => onPrefs({ ...prefs, staleMinutes })} />
      </label>
      <label className="row">
        <input type="checkbox" checked={prefs.shells} onChange={(e) => onPrefs({ ...prefs, shells: e.target.checked })} /> list shell panes too
      </label>
    </div>
  );
}

// MinutesInput keeps its text local so the field can be cleared and retyped; a whole positive number saves as typed
function MinutesInput({ value, onChange }: { value: number; onChange: (n: number) => void }) {
  const [text, setText] = useState(String(value));
  useEffect(() => setText(String(value)), [value]);
  return (
    <input
      type="number"
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

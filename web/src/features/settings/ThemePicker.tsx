// A row of theme cards in Settings › Look: each a small picture of the theme — its base, a sidebar in its mantle, its text
// and dim text, three accents — with its name under it; a radio group, so the arrows move the pick as the platform's
// radios do and only the picked card takes the tab stop.

import { useRef, type KeyboardEvent } from "react";
import { FLAVOR_NAMES, swatches, type Accent, type Flavor } from "@/lib/theme";
import { previewHues, stepChoice } from "./themeCards";

export function ThemePicker<F extends Flavor>({ label, flavors, value, accent, onPick }: { label: string; flavors: readonly F[]; value: F; accent: Accent; onPick: (flavor: F) => void }) {
  const group = useRef<HTMLDivElement>(null);
  const hues = previewHues(accent);
  const step = (e: KeyboardEvent) => {
    const next = stepChoice(flavors, value, e.key);
    if (!next) return;
    e.preventDefault();
    onPick(next);
    group.current?.querySelector<HTMLButtonElement>(`[data-flavor="${next}"]`)?.focus();
  };
  return (
    <div className="theme-cards" role="radiogroup" aria-label={label} ref={group} onKeyDown={step}>
      {flavors.map((flavor) => {
        const c = swatches(flavor);
        const on = flavor === value;
        return (
          <button key={flavor} type="button" role="radio" aria-checked={on} tabIndex={on ? 0 : -1} data-flavor={flavor} className={`btn theme-card${on ? " on" : ""}`} onClick={() => onPick(flavor)}>
            <span className="theme-card-face" style={{ background: c.base, borderColor: c.surface1 }} aria-hidden="true">
              <span className="theme-card-side" style={{ background: c.mantle }} />
              <span className="theme-card-body">
                <span className="theme-card-line" style={{ background: c.text }} />
                <span className="theme-card-line short" style={{ background: c.subtext0 }} />
                <span className="theme-card-dots">
                  {hues.map((hue) => (
                    <span key={hue} style={{ background: c[hue] }} />
                  ))}
                </span>
              </span>
            </span>
            <span className="theme-card-name">{FLAVOR_NAMES[flavor]}</span>
          </button>
        );
      })}
    </div>
  );
}

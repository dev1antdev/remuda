// The status bar's plan limits: each agent whose windows remuda reads, its logo and "5h 38% · wk 54%" in the tone
// remuda gives a window's use; hovering one shows its card — each window's bar with a tick at the share of its time
// gone and the verdict on the pace, the forecast, the reset, how old the reading is — and a click opens the Usage panel.

import type { Machine } from "@/features/roster/api";
import { useSharing } from "@/features/roster/useSharing";
import { accountWindows, agentName, forecastTone, forecastWords, limitTone, resetTime, type AccountWindow } from "@/features/usage/limits";
import { useNow } from "@/lib/clock";
import { t } from "@/lib/i18n";
import { ago } from "@/lib/time";
import { Logo, Tip } from "@/ui";
import { byAgent, elapsedShare, segmentText, shownShare, stale, tickWords, timeGone, timeLeft, verdict, verdictWords, windowLabel, type AgentLimits } from "./pace";

export default function Limits({ machines, remaining, onOpen }: { machines: Machine[]; remaining: boolean; onOpen: () => void }) {
  const now = useNow(30_000);
  const groups = byAgent(accountWindows(machines, now));
  return (
    <div className="status-limits">
      {groups.length > 0 && <span className="status-quiet">{remaining ? t("Remaining") : t("Used")}</span>}
      {groups.length === 0 && (
        <Tip text={t("No plan limits read yet: Codex reports its own after its next answer, Claude Code's through its status line once remuda reads it.")} side="top">
          <button type="button" className="btn ghost sm status-item" onClick={onOpen}>
            {t("no plan limits read")}
          </button>
        </Tip>
      )}
      {groups.map((group) => (
        <Tip key={group.agent} text={<LimitsCard group={group} now={now} remaining={remaining} />} side="top" className="tip-card">
          <button type="button" className={`btn ghost sm status-item status-agent${stale(group.at, now) ? " is-stale" : ""}`} onClick={onOpen} aria-label={t("{agent}'s plan limits: open the Usage panel", { agent: agentName(group.agent) })}>
            <Logo agent={group.agent} />
            {group.windows.map((w, i) => (
              <span key={w.name} className="status-window">
                {i > 0 && <span className="status-sep">·</span>}
                <span className={`status-num${toneClass(limitTone(w.used))}`}>{segmentText(w, remaining)}</span>
              </span>
            ))}
          </button>
        </Tip>
      ))}
    </div>
  );
}

const toneClass = (tone: string) => (tone ? ` is-${tone}` : "");

// LimitsCard is one agent's every window in full, the hover card of its segment
function LimitsCard({ group, now, remaining }: { group: AgentLimits; now: number; remaining: boolean }) {
  const { redact } = useSharing();
  const old = stale(group.at, now);
  return (
    <div className="limits-card">
      <div className="limits-card-head">
        <Logo agent={group.agent} />
        <strong>{agentName(group.agent)}</strong>
        <span className={`limits-card-age${old ? " is-old" : ""}`}>{old ? t("last reading {ago}", { ago: ago(group.at, now) }) : t("read {ago}", { ago: ago(group.at, now) })}</span>
      </div>
      <div className={`limits-card-rows${old ? " is-stale" : ""}`}>
        {group.windows.map((w) => (
          <WindowRow key={w.name} w={w} now={now} remaining={remaining} />
        ))}
      </div>
      <div className="limits-card-foot">{t("Read on {machine}. A click opens the Usage panel.", { machine: redact(group.windows[0]?.machineLabel ?? "") })}</div>
    </div>
  );
}

function WindowRow({ w, now, remaining }: { w: AccountWindow; now: number; remaining: boolean }) {
  const share = elapsedShare(w, now);
  const tick = share === null ? null : remaining ? 1 - share : share;
  const tone = limitTone(w.used);
  const forecast = forecastWords(w, now);
  const shown = shownShare(w.used, remaining);
  return (
    <div className="limits-row">
      <div className="limits-row-main">
        <span className="limits-row-name">{windowLabel(w.name)}</span>
        <span className={`limits-bar${toneClass(tone)}`} role="img" aria-label={share === null ? `${shown} %` : tickWords(w.used, share, remaining)}>
          <span className="limits-bar-fill" style={{ width: `${shown}%` }} />
          {tick !== null && <span className="limits-bar-tick" style={{ left: `${tick * 100}%` }} />}
        </span>
        <span className={`limits-row-pct${toneClass(tone)}`}>{remaining ? t("{n} % left", { n: Math.max(0, Math.ceil(100 - w.used)) }) : t("{n} %", { n: Math.floor(w.used) })}</span>
        <span className="limits-row-left">{timeLeft(w.resets_at, now)}</span>
      </div>
      {share !== null && (
        <div className="limits-row-pace">
          <span className={`limits-verdict is-${verdict(w.used, share)}`}>{verdictWords(verdict(w.used, share))}</span>
          <span className="limits-row-why"> · {timeGone(share, remaining)}</span>
        </div>
      )}
      <div className="limits-row-sub">
        {t("resets {time}", { time: resetTime(w.resets_at, now) })}
        {forecast && <span className={`limits-row-forecast${toneClass(forecastTone(w))}`}> · {forecast}</span>}
      </div>
    </div>
  );
}

// The notice: one block, five tones; and the state dot.

import type { ReactNode } from "react";
import Icon, { type IconName } from "@/ui/icons";

export type Tone = "info" | "success" | "warn" | "error" | "hint";

export function Notice({ tone = "info", icon, children, action, onDismiss, className }: {
  tone?: Tone;
  icon?: IconName;
  children: ReactNode;
  action?: ReactNode;
  onDismiss?: () => void;
  className?: string;
}) {
  return (
    <div className={`notice ${tone}${className ? ` ${className}` : ""}`} role={tone === "error" ? "alert" : undefined}>
      {icon && <Icon name={icon} size={14} className="notice-icon" />}
      <span className="notice-text">{children}</span>
      {action}
      {onDismiss && (
        <button type="button" className="icon-btn sm" onClick={onDismiss} title="Dismiss" aria-label="Dismiss">
          <Icon name="x" size={12} />
        </button>
      )}
    </div>
  );
}

export function StatusDot({ status, title }: { status: string; title?: string }) {
  return <span className={`dot dot-${status}`} title={title} />;
}

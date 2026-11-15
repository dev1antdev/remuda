// What a place says while it has nothing to show: an icon on a soft tile, one line, and where it helps a hint and a
// button. `small` is its size in a narrow place — a rail, a wall tile, a panel's list.

import type { ReactNode } from "react";
import Icon, { type IconName } from "@/ui/icons";

export default function Empty({ icon, title, hint, action, small, className }: {
  icon: IconName;
  title: ReactNode;
  hint?: ReactNode;
  action?: ReactNode;
  small?: boolean;
  className?: string;
}) {
  return (
    <div className={`empty-state${small ? " small" : ""}${className ? ` ${className}` : ""}`}>
      <span className="empty-icon">
        <Icon name={icon} size={small ? 16 : 20} />
      </span>
      <p className="empty-title">{title}</p>
      {hint && <p className="empty-hint">{hint}</p>}
      {action}
    </div>
  );
}

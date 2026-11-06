// One cell of a Markdown table in a chat with a copy button in its corner: shown on hover with a mouse, after a long
// press on a phone. The browser's own table selection turns into whole rows the moment a drag leaves the cell, so the
// button is the sure way to take a wrapped cell's words.

import { useEffect, useRef, useState, type ReactNode } from "react";
import Icon from "@/ui/icons";
import Tip from "@/ui/Tip";
import { copyText } from "@/lib/host";
import { t } from "@/lib/i18n";
import { longPress } from "@/ui/press";
import { cellText } from "@/ui/cellText";

const SHOWN_AFTER_PRESS = 4000;
const COPIED_FOR = 1200;

export default function TableCell({ as, source, children }: { as: "td" | "th"; source: string; children: ReactNode }) {
  const [pressed, setPressed] = useState(false);
  const [copied, setCopied] = useState(false);
  const timers = useRef<{ press?: ReturnType<typeof setTimeout>; copied?: ReturnType<typeof setTimeout> }>({});
  useEffect(
    () => () => {
      clearTimeout(timers.current.press);
      clearTimeout(timers.current.copied);
    },
    [],
  );
  const [press] = useState(() =>
    longPress(() => {
      setPressed(true);
      clearTimeout(timers.current.press);
      timers.current.press = setTimeout(() => setPressed(false), SHOWN_AFTER_PRESS);
    }),
  );
  const copy = () => {
    void copyText(cellText(source)).then((ok) => {
      if (!ok) return;
      setCopied(true);
      clearTimeout(timers.current.copied);
      timers.current.copied = setTimeout(() => setCopied(false), COPIED_FOR);
    });
  };
  const Tag = as;
  return (
    <Tag className={`md-cell${pressed ? " pressed" : ""}`} {...press}>
      {children}
      <Tip text={copied ? t("Copied") : t("Copy this cell")}>
        <button type="button" className="icon-btn sm md-cell-copy" aria-label={t("Copy this cell")} onClick={copy}>
          <Icon name={copied ? "check" : "copy"} size={12} />
        </button>
      </Tip>
    </Tag>
  );
}

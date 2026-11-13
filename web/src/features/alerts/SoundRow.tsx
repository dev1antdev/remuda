// The alerts panel's Sound row: what rings with an alert shown on this page — nothing, the chime for every alert, or a
// bell when an agent needs you and a rising run when one finishes — and a button that plays each one, which is also
// the click a browser waits for before the page may make a sound.

import Select from "@/ui/Select";
import Icon from "@/ui/icons";
import { t } from "@/lib/i18n";
import { preview, wakeSounds } from "./player";
import type { Sound, SoundChoice } from "./sounds";
import { useSoundChoice } from "./useSoundChoice";

export default function SoundRow() {
  const [choice, setChoice] = useSoundChoice();
  const sounds: { sound: Sound; label: string }[] = [
    { sound: "chime", label: t("Chime") },
    { sound: "bell", label: t("Bell") },
    { sound: "run", label: t("Rising run") },
  ];
  return (
    <>
      <label className="alerts-opt">
        <span>
          <b>{t("Sound")}</b> {t("with an alert shown on the page")}
          <small>{t("A bell when an agent needs you, a short rising run when one finishes, the chime for anything else — at most one in 1.5 s, none in the quiet hours.")}</small>
        </span>
        <Select<SoundChoice>
          value={choice}
          onChange={(next) => {
            if (next !== "off") wakeSounds(); // the pick is the gesture: the page may ring from now on
            setChoice(next);
          }}
          options={[
            { value: "off", label: t("no sound") },
            { value: "chime", label: t("a chime") },
            { value: "bell", label: t("a bell and a run") },
          ]}
        />
      </label>
      <div className="alerts-quiet alerts-sounds">
        {sounds.map(({ sound, label }) => (
          <button key={sound} type="button" className="btn sm" data-sound={sound} onClick={() => preview(sound, choice !== "off")}>
            <Icon name="play" size={12} />
            {label}
          </button>
        ))}
      </div>
    </>
  );
}

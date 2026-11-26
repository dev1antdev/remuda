// Settings, in two panes: the list of sections and the one picked beside it. This device keeps how the page looks, how
// the sidebar reads and the page's language; the cockpit keeps, for every device, the sleep and resume switches, the
// Jira site, the backups, the agents' timelapses and who is signed in. The dialog opens on
// the section it was asked for, else on the one this device showed last.

import { useState } from "react";
import BackupSection from "./BackupSection";
import DevicesSection from "./DevicesSection";
import GeneralSection from "./GeneralSection";
import LookSection from "./LookSection";
import SettingsNav, { tabId } from "./SettingsNav";
import TimelapseSection from "./TimelapseSection";
import { FIRST_SECTION, SECTION_PREF, sectionGroups, shownSection, type SectionId } from "./sections";
import JiraSection from "@/features/jira/JiraSection";
import { DEFAULT_RADAR, type RadarPrefs } from "@/features/roster/radar";
import { RadarFields } from "@/features/roster/RadarSettings";
import { useRoster } from "@/features/roster/useRoster";
import { actions } from "@/app/store";
import Usage from "@/features/usage/Usage";
import Schedules from "@/features/schedules/Schedules";
import ReposPanel from "@/features/repos/ReposPanel";
import ServerPanel from "@/features/roster/ServerPanel";
import SleepSection from "@/features/sleep/SleepSection";
import { t } from "@/lib/i18n";
import { usePref } from "@/lib/prefs";
import { Dialog } from "@/ui";

export default function SettingsDialog({ section, onClose }: { section?: SectionId; onClose: () => void }) {
  const { viewOnly, machines, server } = useRoster();
  const [kept, keep] = usePref<string>(SECTION_PREF, FIRST_SECTION);
  const [asked, setAsked] = useState<string>(section ?? kept);
  const groups = sectionGroups(viewOnly, !!server);
  const shown = shownSection(groups, asked);
  const pick = (id: SectionId) => {
    setAsked(id);
    keep(id);
  };
  return (
    <Dialog title={t("Settings")} icon="sliders" size="lg" onClose={onClose} className="settings-dialog">
      <div className="settings-body">
        <SettingsNav groups={groups} shown={shown} onPick={pick} />
        <div key={shown} id="settings-pane" className="settings-pane" role="tabpanel" aria-labelledby={tabId(shown)}>
          <Pane shown={shown} onClose={onClose} machines={machines} server={server} />
        </div>
      </div>
    </Dialog>
  );
}

function Pane({ shown, onClose, machines, server }: { shown: SectionId; onClose: () => void; machines: import("@/features/roster/api").Machine[]; server?: import("@/features/roster/api").Machine }) {
  switch (shown) {
    case "look":
      return <LookSection />;
    case "sidebar":
      return <SidebarSection />;
    case "general":
      return <GeneralSection onClose={onClose} />;
    case "usage":
      return (
        <Usage
          machines={machines}
          embedded
          onClose={onClose}
          onOpenSession={(machine, _project, session, dir) => actions.openViewer({ machine, dir, session })}
        />
      );
    case "automations":
      return <Schedules machines={machines} embedded onClose={onClose} ask={actions.ask} />;
    case "repos":
      return <ReposPanel embedded onClose={onClose} />;
    case "server":
      return server ? <ServerPanel server={server} embedded onClose={onClose} /> : null;
    case "sleep":
      return <SleepSection />;
    case "jira":
      return <JiraSection />;
    case "backups":
      return <BackupSection />;
    case "timelapse":
      return <TimelapseSection />;
    case "devices":
      return <DevicesSection />;
  }
}

function SidebarSection() {
  const [radar, setRadar] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);
  return (
    <section className="settings-section sidebar-settings">
      <h3>{t("Sidebar")}</h3>
      <RadarFields prefs={radar} onPrefs={setRadar} />
    </section>
  );
}

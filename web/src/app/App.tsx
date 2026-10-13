// The page: the roster read once, and the shell drawn from it and the app's own state.

import { RosterProvider } from "@/features/roster/useRoster";
import Shell from "./Shell";

export default function App() {
  return (
    <RosterProvider>
      <Shell />
    </RosterProvider>
  );
}

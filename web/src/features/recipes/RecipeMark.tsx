// RecipeMark is the mark before a recipe's name: the agent's logo, or the terminal for a plain shell, as the agent Select draws them.

import { Icon, Logo } from "@/ui";

export default function RecipeMark({ agent }: { agent: string }) {
  return agent === "" || agent === "shell" ? <Icon name="terminal" size={14} /> : <Logo agent={agent} />;
}

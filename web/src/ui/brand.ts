// Each vendor's own published hue and name, for the logo and the radar rows.

// BRAND is each vendor's own published hue; green and red stay out, they mean done and blocked
export const BRAND: Record<string, string> = {
  claude: "#d97757",
  gemini: "#4285f4",
  kimi: "#1783ff",
  deepseek: "#4d6bfe",
  qwen: "#615ced",
  kiro: "#9046ff",
  cline: "#586876", // #323b43 lightened: as published it dies on a dark panel
  kilo: "#9a9808", // #f8f676 darkened: as published it dies on a light one
  kimchi: "#ff521d",
  muse: "#0082fb",
  crush: "#ff388b",
};

// What to call an agent whose own title never says.
export const VENDOR_NAME: Record<string, string> = {
  claude: "Claude Code",
  codex: "Codex",
  opencode: "OpenCode",
  omp: "Oh My Pi",
  cline: "Cline",
  mastracode: "Mastra",
  kimi: "Kimi",
  kilo: "Kilo",
  maki: "Maki",
  pi: "Pi",
  hermes: "Hermes",
  cursor: "Cursor",
  copilot: "Copilot",
  deepseek: "DeepSeek",
  gemini: "Gemini",
  gpt: "GPT",
  qwen: "Qwen",
  grok: "grok",
  agy: "Antigravity",
  kiro: "Kiro",
  amp: "Amp",
  devin: "Devin",
  qodercli: "Qoder",
  glm: "GLM",
  kimchi: "Kimchi",
  muse: "Muse",
  crush: "Crush",
};

// brandColor is the hue a working row and its mark take: the vendor's, else OTHER_HUE for a harness without one
export const OTHER_HUE = "#c78a1f";
export const brandColor = (agent: string): string => BRAND[agent] ?? OTHER_HUE;

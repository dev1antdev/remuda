# Agent marks

The SVG marks in this folder are the vendor logos used in the sidebar's
agent rows. They come from [hhdebb/herdr-radar](https://github.com/hhdebb/herdr-radar)
(MIT; itself a fork of qintmb/herdr-icon-agent-ui, MIT), which took them from
each vendor's published icon set or from [lobehub/lobe-icons](https://github.com/lobehub/lobe-icons)
(MIT) and normalized them to bare `<path>` geometry. The MIT licences cover
that packaging; the marks identify third-party products and imply no
affiliation or endorsement.

| Mark | Source or owner |
|---|---|
| claude | Anthropic `cwc-workshops` (Apache-2.0) |
| codex | OpenAI `codex` (Apache-2.0) |
| opencode | anomalyco `opencode` (MIT) |
| omp | can1357 `oh-my-pi` (MIT) |
| cline | Cline (MIT) |
| mastracode | MastraCode (MIT) |
| kimi | Moonshot AI Kimi Code CLI (Apache-2.0) |
| kilo | Kilo Code CLI (Apache-2.0) |
| maki | Maki (MIT) |
| pi | Pi coding agent |
| hermes | Hermes Agent |
| cursor | Cursor (proprietary) |
| copilot | GitHub Copilot (proprietary) |
| deepseek | DeepSeek (proprietary) |
| gemini | Google Gemini (proprietary) |
| gpt | OpenAI (proprietary) |
| qwen | Alibaba Qwen (proprietary) |
| grok | xAI Grok (proprietary) |
| agy | Google Antigravity (proprietary) |
| kiro | AWS Kiro (proprietary) |
| glm | Z.ai / GLM (proprietary) |
| amp | Amp (proprietary) |
| devin | Cognition Devin (proprietary) |
| qodercli | Alibaba Qoder (proprietary) |
| kimchi | CAST AI Kimchi (proprietary) |
| muse | Meta Platforms / Muse Code (proprietary) |
| crush | Charmbracelet, Inc. / Crush |

`npm run marks` regenerates `src/marks.ts` from these files.

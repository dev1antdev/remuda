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

## Colours

The page's colours are twenty published palettes, each laid out in
Catppuccin's named slots, as values in `src/lib/themes/` and `src/styles.css`;
where a palette has fewer than fourteen hues an accent is the nearest of its
own or a blend of two, and a few text colours are moved only as far as reading
needs (lib/theme.test.ts says which bar each meets). The terminal colours are
each palette's own ports'.

| Theme | Palette, author | Licence |
|---|---|---|
| Catppuccin Latte, Frappé, Macchiato, Mocha | [Catppuccin](https://github.com/catppuccin/catppuccin) | MIT, Copyright (c) 2021 Catppuccin |
| Dracula | [Dracula Theme](https://draculatheme.com/spec), Zeno Rocha | MIT, Copyright (c) 2023 Dracula Theme |
| Nord | [Nord](https://github.com/nordtheme/nord), Sven Greb | MIT, Copyright (c) 2016-present Sven Greb |
| Gruvbox Dark, Gruvbox Light | [gruvbox](https://github.com/morhetz/gruvbox), Pavel Pertsev (morhetz) | MIT/X11 |
| Tokyo Night, Tokyo Night Day | [tokyonight.nvim](https://github.com/folke/tokyonight.nvim), Folke Lemaitre, after [Tokyo Night](https://github.com/enkia/tokyo-night-vscode-theme) by enkia (MIT) | Apache-2.0 (its terminal ports are published as MIT) |
| Rosé Pine, Rosé Pine Dawn | [Rosé Pine](https://github.com/rose-pine/neovim) | MIT, Copyright (c) 2023 Rosé Pine |
| One Dark, One Light | [Atom's One Dark and One Light syntax themes](https://github.com/atom/atom/tree/master/packages) | MIT, Copyright (c) 2011-2022 GitHub Inc. |
| Kanagawa | [kanagawa.nvim](https://github.com/rebelot/kanagawa.nvim), Tommaso Laurenzi | MIT, Copyright (c) 2021 Tommaso Laurenzi |
| Solarized Light | [Solarized](https://github.com/altercation/solarized), Ethan Schoonover | MIT, Copyright (c) 2011 Ethan Schoonover |
| GitHub Light | [Primer primitives](https://github.com/primer/primitives), GitHub | MIT, Copyright (c) 2018 GitHub Inc. |
| Everforest Light | [Everforest](https://github.com/sainnhe/everforest), sainnhe | MIT, Copyright (c) 2019 sainnhe |
| Ayu Light | [ayu](https://github.com/ayu-theme/ayu-colors), Konstantin Pschera | MIT, Copyright (c) Konstantin Pschera |
| Flexoki Light | [Flexoki](https://stephango.com/flexoki), Steph Ango | MIT, Copyright (c) 2023 Steph Ango |

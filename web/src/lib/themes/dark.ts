// The ten dark themes in Catppuccin's 26 slots: each palette's own background and foreground ramp as crust … text, its own
// hues as the fourteen accents (a blend of two of them where it has fewer), and the terminal sixteen of its own ports
// where it has them (Catppuccin's, One Dark's are worked out from the slots). The stylesheet's palette blocks carry the
// same values (lib/theme.test.ts keeps them equal and every one readable); web/marks/NOTICE.md credits each palette.

import type { Palette } from "./palette";

export const DARK_PALETTES = {
  frappe: {
    name: "Catppuccin Frappé",
    swatches: {
      crust: "#232634", mantle: "#292c3c", base: "#303446", surface0: "#414559", surface1: "#51576d", surface2: "#626880",
      overlay0: "#737994", overlay1: "#838ba7", overlay2: "#949cbb", subtext0: "#a5adce", subtext1: "#b5bfe2", text: "#c6d0f5",
      rosewater: "#f2d5cf", flamingo: "#eebebe", pink: "#f4b8e4", mauve: "#ca9ee6", red: "#e78284", maroon: "#ea999c", peach: "#ef9f76",
      yellow: "#e5c890", green: "#a6d189", teal: "#81c8be", sky: "#99d1db", sapphire: "#85c1dc", blue: "#8caaee", lavender: "#babbf1",
    },
  },
  macchiato: {
    name: "Catppuccin Macchiato",
    swatches: {
      crust: "#181926", mantle: "#1e2030", base: "#24273a", surface0: "#363a4f", surface1: "#494d64", surface2: "#5b6078",
      overlay0: "#6e738d", overlay1: "#8087a2", overlay2: "#939ab7", subtext0: "#a5adcb", subtext1: "#b8c0e0", text: "#cad3f5",
      rosewater: "#f4dbd6", flamingo: "#f0c6c6", pink: "#f5bde6", mauve: "#c6a0f6", red: "#ed8796", maroon: "#ee99a0", peach: "#f5a97f",
      yellow: "#eed49f", green: "#a6da95", teal: "#8bd5ca", sky: "#91d7e3", sapphire: "#7dc4e4", blue: "#8aadf4", lavender: "#b7bdf8",
    },
  },
  mocha: {
    name: "Catppuccin Mocha",
    swatches: {
      crust: "#11111b", mantle: "#181825", base: "#1e1e2e", surface0: "#313244", surface1: "#45475a", surface2: "#585b70",
      overlay0: "#6c7086", overlay1: "#7f849c", overlay2: "#9399b2", subtext0: "#a6adc8", subtext1: "#bac2de", text: "#cdd6f4",
      rosewater: "#f5e0dc", flamingo: "#f2cdcd", pink: "#f5c2e7", mauve: "#cba6f7", red: "#f38ba8", maroon: "#eba0ac", peach: "#fab387",
      yellow: "#f9e2af", green: "#a6e3a1", teal: "#94e2d5", sky: "#89dceb", sapphire: "#74c7ec", blue: "#89b4fa", lavender: "#b4befe",
    },
  },
  dracula: {
    name: "Dracula",
    swatches: {
      crust: "#191a21", mantle: "#21222c", base: "#282a36", surface0: "#343746", surface1: "#424450", surface2: "#44475a",
      overlay0: "#535d7f", overlay1: "#6272a4", overlay2: "#8894b8", subtext0: "#adb5cb", subtext1: "#d3d7df", text: "#f8f8f2",
      rosewater: "#fbc5e0", flamingo: "#ff678e", pink: "#ff79c6", mauve: "#bd93f9", red: "#ff5555", maroon: "#ff6e6e", peach: "#ffb86c",
      yellow: "#f1fa8c", green: "#50fa7b", teal: "#6ef2bc", sky: "#8be9fd", sapphire: "#98d4fc", blue: "#a4befb", lavender: "#d6acff",
    },
    terminal: { cursor: "#f8f8f2", colors: ["#21222c", "#ff5555", "#50fa7b", "#f1fa8c", "#bd93f9", "#ff79c6", "#8be9fd", "#f8f8f2", "#6272a4", "#ff6e6e", "#69ff94", "#ffffa5", "#d6acff", "#ff92df", "#a4ffff", "#ffffff"] },
  },
  nord: {
    name: "Nord",
    swatches: {
      crust: "#20242d", mantle: "#272c36", base: "#2e3440", surface0: "#3b4252", surface1: "#434c5e", surface2: "#4c566a",
      overlay0: "#616e88", overlay1: "#767f90", overlay2: "#8b93a3", subtext0: "#a0a8b6", subtext1: "#bcc3d0", text: "#d8dee9",
      rosewater: "#dcbdb6", flamingo: "#c8746d", pink: "#b87e96", mauve: "#b48ead", red: "#bf616a", maroon: "#c77c85", peach: "#d08770",
      yellow: "#ebcb8b", green: "#a3be8c", teal: "#8fbcbb", sky: "#88c0d0", sapphire: "#85b1c9", blue: "#81a1c1", lavender: "#9b98b7",
    },
    terminal: { cursor: "#d8dee9", colors: ["#3b4252", "#bf616a", "#a3be8c", "#ebcb8b", "#81a1c1", "#b48ead", "#88c0d0", "#e5e9f0", "#4c566a", "#bf616a", "#a3be8c", "#ebcb8b", "#81a1c1", "#b48ead", "#8fbcbb", "#eceff4"] },
  },
  "gruvbox-dark": {
    name: "Gruvbox Dark",
    swatches: {
      crust: "#1d2021", mantle: "#232425", base: "#282828", surface0: "#3c3836", surface1: "#504945", surface2: "#665c54",
      overlay0: "#7c6f64", overlay1: "#928374", overlay2: "#a89984", subtext0: "#bdae93", subtext1: "#d5c4a1", text: "#ebdbb2",
      rosewater: "#dfb1a7", flamingo: "#e76868", pink: "#dd7781", mauve: "#d3869b", red: "#fb4934", maroon: "#f1584e", peach: "#fe8019",
      yellow: "#fabd2f", green: "#b8bb26", teal: "#8ec07c", sky: "#89b38a", sapphire: "#86ac91", blue: "#83a598", lavender: "#ab969a",
    },
    terminal: { cursor: "#ebdbb2", colors: ["#282828", "#cc241d", "#98971a", "#d79921", "#458588", "#b16286", "#689d6a", "#a89984", "#928374", "#fb4934", "#b8bb26", "#fabd2f", "#83a598", "#d3869b", "#8ec07c", "#ebdbb2"] },
  },
  "tokyo-night": {
    name: "Tokyo Night",
    swatches: {
      crust: "#0c0e14", mantle: "#16161e", base: "#1a1b26", surface0: "#292e42", surface1: "#3b4261", surface2: "#414868",
      overlay0: "#565f89", overlay1: "#737aa2", overlay2: "#8e96bc", subtext0: "#a9b1d6", subtext1: "#b5bee6", text: "#c0caf5",
      rosewater: "#d9a4c7", flamingo: "#f98281", pink: "#d988c3", mauve: "#bb9af7", red: "#f7768e", maroon: "#db4b4b", peach: "#ff9e64",
      yellow: "#e0af68", green: "#9ece6a", teal: "#73daca", sky: "#7dcfff", sapphire: "#2ac3de", blue: "#7aa2f7", lavender: "#9d7cd8",
    },
    terminal: { cursor: "#c0caf5", colors: ["#15161e", "#f7768e", "#9ece6a", "#e0af68", "#7aa2f7", "#bb9af7", "#7dcfff", "#a9b1d6", "#414868", "#ff899d", "#9fe044", "#faba4a", "#8db0ff", "#c7a9ff", "#a4daff", "#c0caf5"] },
  },
  "rose-pine": {
    name: "Rosé Pine",
    swatches: {
      crust: "#110f17", mantle: "#16141f", base: "#191724", surface0: "#26233a", surface1: "#403d52", surface2: "#524f67",
      overlay0: "#605d77", overlay1: "#6e6a86", overlay2: "#7f7b98", subtext0: "#908caa", subtext1: "#b8b5cf", text: "#e0def4",
      rosewater: "#ebbcba", flamingo: "#eb96a6", pink: "#dd83b0", mauve: "#c4a7e7", red: "#eb6f92", maroon: "#eb869e", peach: "#f19c83",
      yellow: "#f6c177", green: "#95b1ac", teal: "#99c0c2", sky: "#9ccfd8", sapphire: "#7cb4c2", blue: "#67a2b4", lavender: "#b0bbe0",
    },
    terminal: { cursor: "#524f67", colors: ["#26233a", "#eb6f92", "#31748f", "#f6c177", "#9ccfd8", "#c4a7e7", "#ebbcba", "#e0def4", "#6e6a86", "#eb6f92", "#31748f", "#f6c177", "#9ccfd8", "#c4a7e7", "#ebbcba", "#e0def4"] },
  },
  "one-dark": {
    name: "One Dark",
    // moved for contrast: text, mono-1 #abb2bf, is brightened to read at 7:1 on the base
    swatches: {
      crust: "#181a1f", mantle: "#21252b", base: "#282c34", surface0: "#2c313a", surface1: "#3e4451", surface2: "#4b5263",
      overlay0: "#5c6370", overlay1: "#828997", overlay2: "#9097a4", subtext0: "#979eab", subtext1: "#a1a8b5", text: "#b2b8c4",
      rosewater: "#c68f9a", flamingo: "#da6f8f", pink: "#d372a9", mauve: "#c678dd", red: "#e06c75", maroon: "#be5046", peach: "#d19a66",
      yellow: "#e5c07b", green: "#98c379", teal: "#6dbba8", sky: "#56b6c2", sapphire: "#5cb3d9", blue: "#61afef", lavender: "#9494e6",
    },
  },
  kanagawa: {
    name: "Kanagawa",
    swatches: {
      crust: "#16161d", mantle: "#181820", base: "#1f1f28", surface0: "#2a2a37", surface1: "#363646", surface2: "#54546d",
      overlay0: "#63636b", overlay1: "#727169", overlay2: "#908d78", subtext0: "#a6a082", subtext1: "#c8c093", text: "#dcd7ba",
      rosewater: "#cd9f96", flamingo: "#db7388", pink: "#d27e99", mauve: "#957fb8", red: "#ff5d62", maroon: "#e46876", peach: "#ffa066",
      yellow: "#e6c384", green: "#98bb6c", teal: "#7aa89f", sky: "#a3d4d5", sapphire: "#7fb4ca", blue: "#7e9cd8", lavender: "#9cabca",
    },
    terminal: { cursor: "#c8c093", colors: ["#16161d", "#c34043", "#76946a", "#c0a36e", "#7e9cd8", "#957fb8", "#6a9589", "#c8c093", "#727169", "#e82424", "#98bb6c", "#e6c384", "#7fb4ca", "#938aa9", "#7aa89f", "#dcd7ba"] },
  },
} satisfies Record<string, Palette>;

// The ten light themes in Catppuccin's 26 slots: each palette's own paper and foreground ramp as crust … text, its own
// hues as the fourteen accents (a blend of two of them where it has fewer), and the terminal sixteen of its own ports
// where it has them (Catppuccin's, One Light's are worked out from the slots). The stylesheet's palette blocks carry the
// same values (lib/theme.test.ts keeps them equal and every one readable); web/marks/NOTICE.md credits each palette.

import type { Palette } from "./palette";

export const LIGHT_PALETTES = {
  latte: {
    name: "Catppuccin Latte",
    // moved for contrast: overlay0 to subtext0 take the next darker of Latte's own greys, text #4c4f69 is darkened to read at 7:1 on the mantle
    swatches: {
      crust: "#dce0e8", mantle: "#e6e9ef", base: "#eff1f5", surface0: "#ccd0da", surface1: "#bcc0cc", surface2: "#acb0be",
      overlay0: "#8c8fa1", overlay1: "#7c7f93", overlay2: "#6c6f85", subtext0: "#5c5f77", subtext1: "#52556e", text: "#484b64",
      rosewater: "#dc8a78", flamingo: "#dd7878", pink: "#ea76cb", mauve: "#8839ef", red: "#d20f39", maroon: "#e64553", peach: "#fe640b",
      yellow: "#df8e1d", green: "#40a02b", teal: "#179299", sky: "#04a5e5", sapphire: "#209fb5", blue: "#1e66f5", lavender: "#7287fd",
    },
  },
  "solarized-light": {
    name: "Solarized Light",
    // the text is base02 and the dim text base01: its body ink base00 reads at 4.2:1 on base3, so it is the faint one here
    swatches: {
      crust: "#eee8d5", mantle: "#f6efdc", base: "#fdf6e3", surface0: "#d3d3c5", surface1: "#b7bdb6", surface2: "#93a1a1",
      overlay0: "#839496", overlay1: "#657b83", overlay2: "#5f757c", subtext0: "#586e75", subtext1: "#30525c", text: "#073642",
      rosewater: "#cf414c", flamingo: "#d83459", pink: "#d33682", mauve: "#6c71c4", red: "#dc322f", maroon: "#d43f23", peach: "#cb4b16",
      yellow: "#b58900", green: "#859900", teal: "#2aa198", sky: "#2896b5", sapphire: "#2791c4", blue: "#268bd2", lavender: "#497ecb",
    },
    terminal: { cursor: "#586e75", colors: ["#073642", "#dc322f", "#859900", "#b58900", "#268bd2", "#d33682", "#2aa198", "#eee8d5", "#002b36", "#cb4b16", "#586e75", "#657b83", "#839496", "#6c71c4", "#93a1a1", "#fdf6e3"] },
  },
  "gruvbox-light": {
    name: "Gruvbox Light",
    swatches: {
      crust: "#ebdbb2", mantle: "#f2e5bc", base: "#fbf1c7", surface0: "#d5c4a1", surface1: "#bdae93", surface2: "#a89984",
      overlay0: "#928374", overlay1: "#7c6f64", overlay2: "#71665c", subtext0: "#665c54", subtext1: "#504945", text: "#3c3836",
      rosewater: "#c4604a", flamingo: "#bf4352", pink: "#b16286", mauve: "#8f3f71", red: "#9d0006", maroon: "#cc241d", peach: "#af3a03",
      yellow: "#b57614", green: "#79740e", teal: "#427b58", sky: "#458588", sapphire: "#267680", blue: "#076678", lavender: "#4b5375",
    },
    terminal: { cursor: "#3c3836", colors: ["#fbf1c7", "#cc241d", "#98971a", "#d79921", "#458588", "#b16286", "#689d6a", "#7c6f64", "#928374", "#9d0006", "#79740e", "#b57614", "#076678", "#8f3f71", "#427b58", "#3c3836"] },
  },
  "rose-pine-dawn": {
    name: "Rosé Pine Dawn",
    swatches: {
      crust: "#e9e2dd", mantle: "#f2e9e1", base: "#faf4ed", surface0: "#dfdad9", surface1: "#cecacd", surface2: "#b8b4bd",
      overlay0: "#9893a5", overlay1: "#797593", overlay2: "#6a6684", subtext0: "#625e7d", subtext1: "#555170", text: "#464261",
      rosewater: "#d7827e", flamingo: "#c6737c", pink: "#a76b8a", mauve: "#907aa9", red: "#b4637a", maroon: "#bf6c7b", peach: "#d28354",
      yellow: "#ea9d34", green: "#6d8f89", teal: "#629294", sky: "#56949f", sapphire: "#3f7f91", blue: "#286983", lavender: "#66739a",
    },
    terminal: { cursor: "#cecacd", colors: ["#f2e9e1", "#b4637a", "#286983", "#ea9d34", "#56949f", "#907aa9", "#d7827e", "#575279", "#9893a5", "#b4637a", "#286983", "#ea9d34", "#56949f", "#907aa9", "#d7827e", "#575279"] },
  },
  "tokyo-night-day": {
    name: "Tokyo Night Day",
    // moved for contrast: text #3760bf and fg_dark #6172b0 (subtext0) are darkened to read at 7:1 and 4.5:1
    swatches: {
      crust: "#c1c9df", mantle: "#d0d5e3", base: "#e1e2e7", surface0: "#c4c8da", surface1: "#a8aecb", surface2: "#a1a6c5",
      overlay0: "#848cb5", overlay1: "#68709a", overlay2: "#626991", subtext0: "#536297", subtext1: "#3b5089", text: "#233d7a",
      rosewater: "#da3e3d", flamingo: "#c64343", pink: "#d20065", mauve: "#9854f1", red: "#f52a65", maroon: "#de3754", peach: "#b15c00",
      yellow: "#8c6c3e", green: "#587539", teal: "#118c74", sky: "#007197", sapphire: "#188092", blue: "#2e7de9", lavender: "#7847bd",
    },
    terminal: { cursor: "#3760bf", colors: ["#b4b5b9", "#f52a65", "#587539", "#8c6c3e", "#2e7de9", "#9854f1", "#007197", "#6172b0", "#a1a6c5", "#ff4774", "#5c8524", "#a27629", "#358aff", "#a463ff", "#007ea8", "#3760bf"] },
  },
  "github-light": {
    name: "GitHub Light",
    swatches: {
      crust: "#eaeef2", mantle: "#f6f8fa", base: "#ffffff", surface0: "#d0d7de", surface1: "#c0c8d0", surface2: "#afb8c1",
      overlay0: "#8c959f", overlay1: "#7d8690", overlay2: "#6e7781", subtext0: "#656d76", subtext1: "#424a53", text: "#1f2328",
      rosewater: "#c4432b", flamingo: "#c23e5a", pink: "#bf3989", mauve: "#8250df", red: "#cf222e", maroon: "#a40e26", peach: "#bc4c00",
      yellow: "#9a6700", green: "#1a7f37", teal: "#1b7c83", sky: "#3192aa", sapphire: "#0550ae", blue: "#0969da", lavender: "#465ddd",
    },
    terminal: { cursor: "#0969da", colors: ["#24292f", "#cf222e", "#116329", "#4d2d00", "#0969da", "#8250df", "#1b7c83", "#6e7781", "#57606a", "#a40e26", "#1a7f37", "#633c01", "#218bff", "#a475f9", "#3192aa", "#8c959f"] },
  },
  "one-light": {
    name: "One Light",
    swatches: {
      crust: "#e2e2e3", mantle: "#eaeaeb", base: "#fafafa", surface0: "#dbdbdc", surface1: "#c6c6c7", surface2: "#a0a1a7",
      overlay0: "#92949b", overlay1: "#85878f", overlay2: "#777983", subtext0: "#696c77", subtext1: "#51535d", text: "#383a42",
      rosewater: "#da632c", flamingo: "#d73446", pink: "#b41e7d", mauve: "#a626a4", red: "#ca1243", maroon: "#e45649", peach: "#cb7701",
      yellow: "#b76b01", green: "#50a14f", teal: "#219090", sky: "#0184bc", sapphire: "#217ed7", blue: "#4078f2", lavender: "#6957d3",
    },
  },
  "everforest-light": {
    name: "Everforest Light",
    // moved for contrast: text, fg #5c6a72 (kept as subtext0), is darkened to read at 7:1 on bg0 and bg1
    swatches: {
      crust: "#efebd4", mantle: "#f4f0d9", base: "#fdf6e3", surface0: "#e6e2cc", surface1: "#e0dcc7", surface2: "#bdc3af",
      overlay0: "#a6b0a0", overlay1: "#829181", overlay2: "#708089", subtext0: "#5c6a72", subtext1: "#525e65", text: "#475258",
      rosewater: "#f7693c", flamingo: "#f15b71", pink: "#df69ba", mauve: "#a578be", red: "#f85552", maroon: "#f45862", peach: "#f57d26",
      yellow: "#dfa000", green: "#8da101", teal: "#35a77c", sky: "#389ea1", sapphire: "#3999b3", blue: "#3a94c5", lavender: "#7c83c1",
    },
    terminal: { cursor: "#5c6a72", colors: ["#5c6a72", "#f85552", "#8da101", "#dfa000", "#3a94c5", "#df69ba", "#35a77c", "#e6e2cc", "#5c6a72", "#f85552", "#8da101", "#dfa000", "#3a94c5", "#df69ba", "#35a77c", "#e6e2cc"] },
  },
  "ayu-light": {
    name: "Ayu Light",
    // moved for contrast: text, the editor's #5c6166 (kept as subtext1), is deepened toward its own ink #020a12 to read at 7:1
    swatches: {
      crust: "#ebeef0", mantle: "#f8f9fa", base: "#fcfcfc", surface0: "#dcdee0", surface1: "#c9cbcd", surface2: "#adaeb1",
      overlay0: "#9ca0a5", overlay1: "#8a9199", overlay2: "#7c838a", subtext0: "#6e747a", subtext1: "#5c6166", text: "#50565b",
      rosewater: "#f2a191", flamingo: "#f18981", pink: "#d17595", mauve: "#a37acc", red: "#f07171", maroon: "#f3775e", peach: "#fa8532",
      yellow: "#eba400", green: "#86b300", teal: "#4cbf99", sky: "#55b4d4", sapphire: "#3cacdd", blue: "#22a4e6", lavender: "#638fd9",
    },
    terminal: { cursor: "#f29718", colors: ["#000000", "#f06b6c", "#6cbf43", "#e7a100", "#21a1e2", "#a176cb", "#4abc96", "#c7c7c7", "#686868", "#f07171", "#86b300", "#eba400", "#22a4e6", "#a37acc", "#4cbf99", "#d1d1d1"] },
  },
  "flexoki-light": {
    name: "Flexoki Light",
    swatches: {
      crust: "#e6e4d9", mantle: "#f2f0e5", base: "#fffcf0", surface0: "#dad8ce", surface1: "#cecdc3", surface2: "#b7b5ac",
      overlay0: "#9f9d96", overlay1: "#878580", overlay2: "#6f6e69", subtext0: "#6f6e69", subtext1: "#575653", text: "#100f0f",
      rosewater: "#b6411f", flamingo: "#a8304c", pink: "#a02f6f", mauve: "#5e409d", red: "#af3029", maroon: "#c03e35", peach: "#bc5215",
      yellow: "#ad8301", green: "#66800b", teal: "#24837b", sky: "#3171b2", sapphire: "#227191", blue: "#205ea6", lavender: "#3f4fa2",
    },
    terminal: { cursor: "#100f0f", colors: ["#100f0f", "#d14d41", "#879a39", "#d0a215", "#4385be", "#ce5d97", "#3aa99f", "#fffcf0", "#6f6e69", "#af3029", "#66800b", "#ad8301", "#205ea6", "#a02f6f", "#24837b", "#f2f0e5"] },
  },
} satisfies Record<string, Palette>;

package code

// EditorTheme is how code-server wears one of the page's themes: the Open VSX extension that carries it ("" for one
// VS Code has built in) and the exact name workbench.colorTheme takes.
type EditorTheme struct {
	Extension string
	Name      string
}

const catppuccin = "Catppuccin.catppuccin-vsc"

// editorThemes are the page's twenty themes in the editor, each by its own extension on Open VSX where there is one;
// Tokyo Night Day wears its project's own light theme, and Flexoki, with none there, VS Code's built-in light one.
var editorThemes = map[string]EditorTheme{
	"frappe":           {catppuccin, "Catppuccin Frappé"},
	"macchiato":        {catppuccin, "Catppuccin Macchiato"},
	"mocha":            {catppuccin, "Catppuccin Mocha"},
	"dracula":          {"dracula-theme.theme-dracula", "Dracula Theme"},
	"nord":             {"arcticicestudio.nord-visual-studio-code", "Nord"},
	"gruvbox-dark":     {"jdinhlife.gruvbox", "Gruvbox Dark Medium"},
	"tokyo-night":      {"enkia.tokyo-night", "Tokyo Night"},
	"rose-pine":        {"mvllow.rose-pine", "Rosé Pine"},
	"one-dark":         {"zhuangtongfa.material-theme", "One Dark Pro"},
	"kanagawa":         {"metaphore.kanagawa-vscode-color-theme", "Kanagawa Wave"},
	"latte":            {catppuccin, "Catppuccin Latte"},
	"solarized-light":  {"", "Solarized Light"},
	"gruvbox-light":    {"jdinhlife.gruvbox", "Gruvbox Light Medium"},
	"rose-pine-dawn":   {"mvllow.rose-pine", "Rosé Pine Dawn"},
	"tokyo-night-day":  {"enkia.tokyo-night", "Tokyo Night Light"},
	"github-light":     {"GitHub.github-vscode-theme", "GitHub Light Default"},
	"one-light":        {"akamud.vscode-theme-onelight", "Atom One Light"},
	"everforest-light": {"sainnhe.everforest", "Everforest Light"},
	"ayu-light":        {"teabyii.ayu", "Ayu Light Bordered"},
	"flexoki-light":    {"", "Default Light Modern"},
}

// KnownTheme says whether a word names one of the page's themes, or the bare "dark" or "light" a page from before sent.
func KnownTheme(theme string) bool {
	_, ok := editorThemes[theme]
	return ok || theme == "dark" || theme == "light"
}

// EditorThemeOf is the editor's theme for one of the page's: Frappé for the bare "dark" and Latte for "light", as the
// page from before meant them, and VS Code's own dark one for a word it does not know.
func EditorThemeOf(theme string) EditorTheme {
	switch theme {
	case "dark":
		theme = "frappe"
	case "light":
		theme = "latte"
	}
	if editor, ok := editorThemes[theme]; ok {
		return editor
	}
	return EditorTheme{Name: "Default Dark Modern"}
}

// ThemeName is the editor's theme name for one of the page's themes, as workbench.colorTheme takes it.
func ThemeName(theme string) string { return EditorThemeOf(theme).Name }

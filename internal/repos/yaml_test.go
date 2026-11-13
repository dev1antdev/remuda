package repos

import "testing"

func TestTheSetupScriptIsReadOffTheTeamsFile(t *testing.T) {
	cases := []struct{ name, text, want string }{
		{"a block", "name: app\nscripts:\n  setup: |\n    pnpm i\n    ln -s $HERDR_ROOT_PATH/.env .env\n  run: pnpm dev\nother: 1\n", "pnpm i\nln -s $HERDR_ROOT_PATH/.env .env"},
		{"one line", "scripts:\n  setup: pnpm i # deps\n", "pnpm i # deps"},
		{"quoted", "scripts:\n  setup: \"make deps\"\n", "make deps"},
		{"a folded block with a blank line", "scripts:\n  setup: >-\n    a\n\n    b\n", "a\n\nb"},
		{"windows line ends", "scripts:\r\n  setup: |\r\n    npm ci\r\n", "npm ci"},
		{"no scripts key", "setup: pnpm i\n", ""},
		{"setup under another key", "scripts:\n  run: x\nsetup:\n  - y\n", ""},
	}
	for _, c := range cases {
		if got := YAMLScript(c.text, "setup"); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

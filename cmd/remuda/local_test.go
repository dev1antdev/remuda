package main

import "testing"

func TestYesIsOnlyYes(t *testing.T) {
	for answer, want := range map[string]bool{"y\n": true, "Y\n": true, " yes \n": true, "\n": false, "": false, "n\n": false, "yep\n": false, "да\n": false} {
		if got := yes(answer); got != want {
			t.Errorf("yes(%q) = %v, want %v", answer, got, want)
		}
	}
}

// TestOpenScriptKeepsTheAddressAString checks that nothing the address holds becomes script.
func TestOpenScriptKeepsTheAddressAString(t *testing.T) {
	for page, want := range map[string]string{
		"http://127.0.0.1:7317/?token=abc":   `open location "http://127.0.0.1:7317/?token=abc"` + "\n",
		`http://x/?token=a"b`:                `open location "http://x/?token=a\"b"` + "\n",
		`http://x/" & (do shell script "x")`: `open location "http://x/\" & (do shell script \"x\")"` + "\n",
		`http://x/\`:                         `open location "http://x/\\"` + "\n",
	} {
		if got := openScript(page); got != want {
			t.Errorf("openScript(%q) = %q, want %q", page, got, want)
		}
	}
}

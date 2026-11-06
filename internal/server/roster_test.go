package server

import "testing"

func TestATabNamedByDigitsIsHerdrs(t *testing.T) {
	for label, named := range map[string]bool{"": false, "1": false, "12": false, " 2 ": false, "claude": true, "tab 2": true, "2a": true} {
		if got := tabNamed(label); got != named {
			t.Errorf("tabNamed(%q) = %v, want %v", label, got, named)
		}
	}
}

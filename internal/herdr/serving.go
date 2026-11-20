package herdr

import (
	"fmt"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// The herdr a machine moved off when its remuda went to remuda-core: its server may still run there, holding the
// agents it had. Two scripts over the machine's shell ask and stop it — through the machine's own herdr_bin and
// remote_socket when it has them, else herdr found as the probe finds it, on its own socket (HERDR_SOCKET_PATH, else
// its default): a machine on the core keeps no herdr of its own.

// findHerdr sets h to herdr's binary and up to whether its server answers, or ends the script with "none" where herdr
// is not installed.
func findHerdr(bin, socket string) string {
	find := script.HerdrBin("h")
	if bin != "" {
		find = "h=" + script.Quote(bin)
	}
	if socket != "" {
		find += "\nexport HERDR_SOCKET_PATH=" + script.Quote(socket)
	}
	return script.PATH + "\n" + find + `
[ -n "$h" ] || { echo none; exit 0; }
up() { "$h" status server 2>/dev/null | grep -q "status: running"; }
`
}

// Serving says whether herdr's server answers on the host; false where herdr is not installed. bin and socket are the
// machine's own herdr_bin and remote_socket, "" for none.
func Serving(host shell.Shell, bin, socket string) (bool, error) {
	out, err := shell.Text(host, findHerdr(bin, socket)+"if up; then echo running; else echo stopped; fi\n", 20*time.Second)
	switch lastLine(out) {
	case "running":
		return true, nil
	case "stopped", "none":
		return false, nil
	}
	return false, fmt.Errorf("cannot tell whether herdr's server runs there: %s", said(out, err))
}

// StopServing stops herdr's server on the host, and every agent still running in it with it, waiting up to ten seconds
// for it to go; herdr stays installed.
func StopServing(host shell.Shell, bin, socket string) error {
	out, err := shell.Text(host, findHerdr(bin, socket)+`if up; then "$h" server stop 2>&1; fi
i=0; while up; do [ $i -ge 20 ] && { echo running; exit 0; }; sleep 0.5; i=$((i+1)); done; echo stopped
`, 45*time.Second)
	switch lastLine(out) {
	case "stopped", "none":
		return nil
	case "running":
		return fmt.Errorf("herdr's server still answers after its stop: %s", said(out, nil))
	}
	return fmt.Errorf("herdr's server did not stop: %s", said(out, err))
}

func lastLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// said is what a script answered, for an error: its output, else the shell's failure.
func said(out string, err error) string {
	if text := strings.TrimSpace(out); text != "" {
		return text
	}
	if err != nil {
		return err.Error()
	}
	return "no answer"
}

package herdrollback

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

// findHerdr sets h to herdr's binary, or ends the script with "none" where herdr is not installed, and defines state:
// s is running or stopped only on herdr's own word ("status: running", "status: not running"), else unknown, with what
// herdr said in said — a binary that would not run is no server stopped.
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
state() {
  said=$("$h" status server 2>&1)
  case "$said" in
  *"status: running"*) s=running ;;
  *"status: not running"*) s=stopped ;;
  *) s=unknown ;;
  esac
}
`
}

// tell ends a script with the state, what herdr said before it when that is unknown.
const tell = `[ "$s" = unknown ] && printf '%s\n' "$said"; echo "$s"
`

// Serving says whether herdr's server answers on the host; false where herdr is not installed, an error when herdr
// would not say. bin and socket are the herdr remuda reached there, "" for herdr's own.
func Serving(host shell.Shell, bin, socket string) (bool, error) {
	out, err := shell.Text(host, findHerdr(bin, socket)+"state\n"+tell, 20*time.Second)
	switch lastLine(out) {
	case "running":
		return true, nil
	case "stopped", "none":
		return false, nil
	}
	return false, fmt.Errorf("cannot tell whether herdr's server runs there: %s", said(out, err))
}

// StopServing stops herdr's server on the host, and every agent still running in it with it, waiting up to ten seconds
// for herdr to say it is down; herdr stays installed. Anything short of herdr's word that it stopped is an error.
func StopServing(host shell.Shell, bin, socket string) error {
	out, err := shell.Text(host, findHerdr(bin, socket)+`state
[ "$s" = running ] && "$h" server stop 2>&1
i=0
while state; [ "$s" = running ] && [ $i -lt 20 ]; do sleep 0.5; i=$((i+1)); done
`+tell, 45*time.Second)
	switch lastLine(out) {
	case "stopped", "none":
		return nil
	case "running":
		return fmt.Errorf("herdr's server still answers after its stop: %s", said(out, nil))
	}
	return fmt.Errorf("cannot tell whether herdr's server stopped: %s", said(out, err))
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

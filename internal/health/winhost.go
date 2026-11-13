package health

import (
	"fmt"
	"strconv"
	"strings"
)

// A Windows machine's body as Git Bash reads it there: no uptime command, so the uptime and load come from MSYS's
// /proc; the edition and build from the registry, which stand for the board; memory from /proc/meminfo, whose free
// memory is Windows' available memory. No temperature, clock or throttling: Windows hands none out without WMI.

// WindowsHostScript prints the lines only a Windows machine answers; elsewhere it prints nothing.
const WindowsHostScript = `case "$(uname -s)" in MINGW*|MSYS*|CYGWIN*)
  k='HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion'
  echo "winver=$(reg query "$k" //v ProductName 2>/dev/null | sed -n 's/.*REG_SZ *//p' | head -1)|$(reg query "$k" //v CurrentBuild 2>/dev/null | sed -n 's/.*REG_SZ *//p' | head -1)"
  echo "winup=$(cut -d' ' -f1 /proc/uptime 2>/dev/null) $(cut -d' ' -f1 /proc/loadavg 2>/dev/null)" ;;
esac
`

// windowsName is the system as uname says it under Git Bash, MINGW64_NT-10.0-26100, in Windows' words.
func windowsName(uname string) (string, bool) {
	for _, prefix := range []string{"MINGW64_NT-", "MINGW32_NT-", "MSYS_NT-", "CYGWIN_NT-"} {
		if rest, ok := strings.CutPrefix(uname, prefix); ok {
			version, _, _ := strings.Cut(rest, " ")
			return "Windows " + strings.Replace(version, "-", ".", 1), true
		}
	}
	return "", false
}

// windowsEdition is the registry's product name and build as the machine's model: Windows 11 still names itself
// "Windows 10" there, and only its build (22000 on) tells.
func windowsEdition(v string) string {
	name, build, _ := strings.Cut(v, "|")
	name, build = strings.TrimSpace(name), strings.TrimSpace(build)
	if n, err := strconv.Atoi(build); err == nil && n >= 22000 && strings.HasPrefix(name, "Windows 10") {
		name = "Windows 11" + strings.TrimPrefix(name, "Windows 10")
	}
	switch {
	case name == "":
		return ""
	case build == "":
		return name
	}
	return name + " (build " + build + ")"
}

// parseWindowsUp reads "208.78 1.26": the seconds since boot and the minute's load, as MSYS reckons them.
func parseWindowsUp(r *Report, v string) {
	f := strings.Fields(v)
	if len(f) >= 1 {
		if seconds, err := strconv.ParseFloat(f[0], 64); err == nil && seconds > 0 {
			r.Uptime = uptimeWords(int(seconds))
		}
	}
	if len(f) >= 2 {
		r.Load1, _ = strconv.ParseFloat(f[1], 64)
	}
}

// uptimeWords says a time since boot the way BSD's uptime does: "3 days, 2:14", "5:03", "12 mins".
func uptimeWords(seconds int) string {
	days, hours, minutes := seconds/86400, seconds%86400/3600, seconds%3600/60
	clock := fmt.Sprintf("%d:%02d", hours, minutes)
	if hours == 0 {
		clock = fmt.Sprintf("%d mins", minutes)
	}
	switch {
	case days == 1:
		return "1 day, " + clock
	case days > 1:
		return fmt.Sprintf("%d days, %s", days, clock)
	}
	return clock
}

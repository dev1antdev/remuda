package herdr

import "github.com/dev1antdev/remuda/internal/script"

// StartWindowsServerScript starts herdr's server on a Windows machine whose ssh shell is Git Bash, unless one runs,
// and waits for it. A server started from the ssh session itself would hold the session open and die with it, so
// PowerShell starts it hidden, outside the session; its own log is herdr-server.log beside the socket.
func StartWindowsServerScript(bin, socket string) string {
	q := script.Quote(bin)
	s := script.Quote(socket)
	up := `HERDR_SOCKET_PATH=` + s + ` ` + q + ` status server 2>/dev/null | grep -q "status: running"`
	return `if ` + up + `; then exit 0; fi
unset HERDR_ENV HERDR_PANE_ID TS_AUTHKEY; export HERDR_SOCKET_PATH=` + s + `
w=$(cygpath -w ` + q + ` | sed "s/'/''/g")
powershell.exe -NoProfile -NonInteractive -Command "Start-Process -WindowStyle Hidden -WorkingDirectory \$env:USERPROFILE -FilePath '$w' -ArgumentList 'server'" < /dev/null
i=0; while [ $i -lt 30 ]; do if ` + up + `; then exit 0; fi; sleep 0.5; i=$((i+1)); done; exit 1
`
}

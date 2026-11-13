package main

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// bridgeCmd is `remuda bridge [socket]`: stdin to the herdr server on this machine and its answers to stdout, until
// either side ends. A cockpit reaches a Windows machine's herdr this way, one exec over ssh per request and one that
// stays for the event stream: herdr listens on a named pipe there, which neither an ssh forward nor nc can reach.
// The socket is herdr's own resolution when not given: $HERDR_SOCKET_PATH, else the default session's.
func bridgeCmd(args []string) {
	socket := herdr.OwnSocket()
	if len(args) > 0 && args[0] != "" {
		socket = args[0]
	}
	conn, err := herdr.DialLocal(socket, 10*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, "remuda bridge:", err)
		os.Exit(1)
	}
	if err := bridge(conn, os.Stdin, os.Stdout); err != nil {
		os.Exit(1)
	}
}

// bridge copies in to herdr and herdr to out until herdr closes the connection or in ends: the cockpit never closes
// its side before it is done reading, so an end of in means nobody reads any more.
func bridge(conn io.ReadWriteCloser, in io.Reader, out io.Writer) error {
	var once sync.Once
	closeConn := func() { once.Do(func() { conn.Close() }) }
	go func() {
		io.Copy(conn, in)
		closeConn()
	}()
	_, err := io.Copy(out, conn)
	closeConn()
	return err
}

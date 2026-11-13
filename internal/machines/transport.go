package machines

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/sshx"
)

// How this remuda reaches herdr on a remote machine.

// rpcClient picks the RPC transport: a forwarded unix socket, else one `nc -U` exec per request over the master
// (some sshds refuse streamlocal forwarding with "unknown channel type"; herdr answers once per connection anyway).
func rpcClient(conn *sshx.Conn, sock string) (*herdr.Client, error) {
	var errs []string
	if local, err := conn.ForwardSocket(sock); err == nil {
		client := herdr.NewClient(local)
		if _, _, err := client.Ping(); err == nil {
			return client, nil
		} else {
			errs = append(errs, "forward: "+err.Error())
		}
	} else {
		errs = append(errs, err.Error())
	}

	bridge := "nc -U " + script.Quote(sock)
	client := herdr.NewClient(sock)
	client.Dial = func(timeout time.Duration) (net.Conn, error) {
		return conn.DialExec(bridge, timeout)
	}
	if _, _, err := client.Ping(); err == nil {
		return client, nil
	} else {
		errs = append(errs, "nc -U: "+err.Error())
	}
	return nil, fmt.Errorf("%s", strings.Join(errs, "; "))
}

// bridgeClient reaches herdr on a Windows machine: its socket is a named pipe there, which no ssh forward and no nc
// can open, so every request is one `remuda.exe bridge` exec over the master, and the event stream one that stays.
func bridgeClient(conn *sshx.Conn, remudaBin, sock string) (*herdr.Client, error) {
	command := BridgeCommand(remudaBin, sock)
	client := herdr.NewClient(sock)
	client.Dial = func(timeout time.Duration) (net.Conn, error) {
		return conn.DialExec(command, timeout)
	}
	if _, _, err := client.Ping(); err != nil {
		return nil, fmt.Errorf("remuda bridge: %v", err)
	}
	return client, nil
}

// BridgeCommand is the exec that carries one connection to herdr on a Windows machine, as its Git Bash reads it.
func BridgeCommand(remudaBin, sock string) string {
	return script.Quote(remudaBin) + " bridge " + script.Quote(sock)
}

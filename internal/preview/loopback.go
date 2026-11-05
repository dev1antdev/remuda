package preview

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// errInUse is a port of the cockpit's host something already answers on.
var errInUse = errors.New("something on this host answers on that port")

// LoopbackListen opens a preview on a port of the host -addr listens on, addressed by the name the page was asked at.
// A port something on the host already answers on — on either loopback — is never taken: a Mac lets a listener on
// 127.0.0.1 bind over another process's wildcard one, which would take the port from the host's own server (a preview
// of the local machine's dev server would even proxy into itself).
func LoopbackListen(addr string) func(port int, host string) (net.Listener, string, error) {
	bind, _, err := net.SplitHostPort(addr)
	if err != nil {
		bind = "127.0.0.1"
	}
	return func(port int, host string) (net.Listener, string, error) {
		if answers(port) {
			return nil, "", errInUse
		}
		ln, err := net.Listen("tcp", net.JoinHostPort(bind, strconv.Itoa(port)))
		if err != nil {
			return nil, "", err
		}
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		return ln, fmt.Sprintf("http://%s:%d", host, port), nil
	}
}

// answers says whether a server on this host takes a connection on the port, over IPv4's loopback or IPv6's
func answers(port int) bool {
	for _, loopback := range []string{"127.0.0.1", "::1"} {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(loopback, strconv.Itoa(port)), 300*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
	}
	return false
}

package herdr

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// DialLocal opens a connection to the herdr server on this machine. On Windows herdr's socket is a named pipe named
// after the socket's path (\\.\pipe\C:\Users\me\AppData\Roaming\herdr\herdr.sock, as a real herdr 0.9.5 listens).
// It is opened for overlapped I/O: os.NewFile then puts it on the runtime's completion port, so a read waiting for
// herdr's answer never holds back the write of the request, and the deadlines work.
func DialLocal(socket string, timeout time.Duration) (net.Conn, error) {
	name := PipeName(socket)
	path, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	const pipeBusy = syscall.Errno(231) // every instance of the pipe is taken this moment
	deadline := time.Now().Add(timeout)
	for {
		handle, err := syscall.CreateFile(path, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_OVERLAPPED, 0)
		if err == nil {
			return pipeConn{os.NewFile(uintptr(handle), name)}, nil
		}
		if !errors.Is(err, pipeBusy) || time.Now().After(deadline) {
			if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
				return nil, fmt.Errorf("no herdr server answers at %s", name)
			}
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// PipeName is the named pipe herdr listens on for a socket path.
func PipeName(socket string) string { return `\\.\pipe\` + socket }

// localSocket is herdr's default socket on this machine: under %APPDATA%\herdr.
func localSocket() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, "AppData", "Roaming")
	}
	return filepath.Join(dir, "herdr", "herdr.sock")
}

// pipeConn is the pipe as a net.Conn.
type pipeConn struct{ *os.File }

func (p pipeConn) LocalAddr() net.Addr  { return pipeAddr(p.Name()) }
func (p pipeConn) RemoteAddr() net.Addr { return pipeAddr(p.Name()) }

type pipeAddr string

func (a pipeAddr) Network() string { return "pipe" }
func (a pipeAddr) String() string  { return string(a) }

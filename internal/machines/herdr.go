package machines

import "time"

// MovedHerdr is the herdr remuda reached on this machine before the move to remuda-core: this one's -herdr-bin and
// -socket; a remote's herdr_bin, and the socket remuda dials there (herdrSocket: its remote_socket, else herdr's
// default under %APPDATA% or the home, never one the ssh environment names), once a connect has read the machine;
// "" where it had none, for herdr's own lookup and socket.
func (mc *Machine) MovedHerdr() (bin, socket string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if !mc.local {
		return mc.Cfg.HerdrBin, mc.socket
	}
	return mc.herdrBin, mc.socket
}

// mayStartServer allows one `herdr server` start per two minutes, noting the one it allows.
func (m *Machine) mayStartServer() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if time.Since(m.serverStart) < 2*time.Minute {
		return false
	}
	m.serverStart = time.Now()
	return true
}

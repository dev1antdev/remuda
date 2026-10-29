// Package wake sends the magic packet that wakes a sleeping machine on the same network: six 0xFF bytes and then its
// MAC address sixteen times, as UDP to every broadcast address this machine has. It goes out from this remuda only —
// a remote never relays it — and nothing comes back: the machine's own reconnect notices when it is up.
package wake

import (
	"fmt"
	"net"
	"strings"
)

// Port is where the packet goes; the discard port is the convention, no one listens and nothing has to.
const Port = 9

// ParseMAC reads a 48-bit address written aa:bb:cc:dd:ee:ff, aa-bb-cc-dd-ee-ff or aabb.ccdd.eeff and answers it in
// the colon form, lower case.
func ParseMAC(s string) (string, error) {
	s = strings.TrimSpace(s)
	hw, err := net.ParseMAC(s)
	if err != nil || len(hw) != 6 {
		return "", fmt.Errorf("%q is not a MAC address like aa:bb:cc:dd:ee:ff", s)
	}
	return strings.ToLower(hw.String()), nil
}

// Packet is the magic packet for the address: 6 × 0xFF, then the address 16 times.
func Packet(mac string) ([]byte, error) {
	canonical, err := ParseMAC(mac)
	if err != nil {
		return nil, err
	}
	hw, _ := net.ParseMAC(canonical)
	packet := make([]byte, 0, 6+16*6)
	for i := 0; i < 6; i++ {
		packet = append(packet, 0xFF)
	}
	for i := 0; i < 16; i++ {
		packet = append(packet, hw...)
	}
	return packet, nil
}

// Broadcast is the directed broadcast address of an IPv4 network, nil for anything else.
func Broadcast(n *net.IPNet) net.IP {
	ip := n.IP.To4()
	if ip == nil || len(n.Mask) != 4 {
		return nil
	}
	out := make(net.IP, 4)
	for i := range out {
		out[i] = ip[i] | ^n.Mask[i]
	}
	return out
}

// Targets lists where the packet goes: the broadcast address of every interface that is up, broadcast-capable and
// not the loopback, and the limited broadcast last.
func Targets() ([]net.IP, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []net.IP
	seen := map[string]bool{}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagBroadcast == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			n, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			if b := Broadcast(n); b != nil && !seen[b.String()] {
				seen[b.String()] = true
				out = append(out, b)
			}
		}
	}
	return append(out, net.IPv4bcast), nil
}

// Send sends the packet to every target and answers how many went out; an error only when none did.
func Send(mac string) (int, error) {
	packet, err := Packet(mac)
	if err != nil {
		return 0, err
	}
	targets, err := Targets()
	if err != nil {
		return 0, err
	}
	sent := 0
	var last error
	for _, ip := range targets {
		conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: ip, Port: Port})
		if err != nil {
			last = err
			continue
		}
		_, err = conn.Write(packet)
		conn.Close()
		if err != nil {
			last = err
			continue
		}
		sent++
	}
	if sent == 0 && last != nil {
		return 0, fmt.Errorf("the packet went nowhere: %w", last)
	}
	return sent, nil
}

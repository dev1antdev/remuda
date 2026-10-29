package wake

import (
	"bytes"
	"net"
	"testing"
)

func TestParseMACReadsTheThreeSpellings(t *testing.T) {
	for _, in := range []string{"AA:BB:CC:DD:EE:FF", "aa-bb-cc-dd-ee-ff", "aabb.ccdd.eeff", "  aa:bb:cc:dd:ee:ff\n"} {
		got, err := ParseMAC(in)
		if err != nil || got != "aa:bb:cc:dd:ee:ff" {
			t.Errorf("ParseMAC(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "aa:bb:cc:dd:ee", "aa:bb:cc:dd:ee:ff:00:11", "hello", "aa:bb:cc:dd:ee:gg"} {
		if _, err := ParseMAC(bad); err == nil {
			t.Errorf("ParseMAC(%q) accepted", bad)
		}
	}
}

func TestPacketIsSixFFsThenTheAddressSixteenTimes(t *testing.T) {
	packet, err := Packet("01:02:03:04:05:06")
	if err != nil {
		t.Fatal(err)
	}
	if len(packet) != 102 || !bytes.Equal(packet[:6], bytes.Repeat([]byte{0xFF}, 6)) {
		t.Fatalf("packet = % x", packet)
	}
	mac := []byte{1, 2, 3, 4, 5, 6}
	for i := 0; i < 16; i++ {
		if !bytes.Equal(packet[6+i*6:12+i*6], mac) {
			t.Fatalf("copy %d of the address is wrong: % x", i, packet)
		}
	}
	if _, err := Packet("nope"); err == nil {
		t.Fatal("a bad address made a packet")
	}
}

func TestBroadcastOfANetwork(t *testing.T) {
	_, n, _ := net.ParseCIDR("192.168.1.37/24")
	if got := Broadcast(n); got.String() != "192.168.1.255" {
		t.Fatalf("broadcast = %s", got)
	}
	_, n, _ = net.ParseCIDR("10.0.0.5/30")
	if got := Broadcast(n); got.String() != "10.0.0.7" {
		t.Fatalf("broadcast = %s", got)
	}
	_, n6, _ := net.ParseCIDR("fe80::1/64")
	if Broadcast(n6) != nil {
		t.Fatal("an IPv6 network has no broadcast")
	}
}

// The packet really leaves this machine: a UDP broadcast to an address nobody has, harmless on any network.
func TestSendReachesAtLeastTheLimitedBroadcast(t *testing.T) {
	targets, err := Targets()
	if err != nil || len(targets) == 0 {
		t.Skipf("no targets here: %v", err)
	}
	sent, err := Send("02:00:00:00:00:01")
	if err != nil && sent == 0 {
		t.Skipf("the host refuses broadcasts: %v", err)
	}
	if sent < 1 {
		t.Fatalf("sent = %d", sent)
	}
}

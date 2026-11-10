package qr2

import (
	"encoding/binary"
	"net"
	"testing"
	"wwfc/ffclimit"
)

func qr2Packet(kind byte, sessionID uint32) []byte {
	buf := make([]byte, 21)
	buf[0] = kind
	binary.BigEndian.PutUint32(buf[1:5], sessionID)
	return buf
}

// NET-09: a flood of forged datagrams from the address of an authenticated session (same IP, its own port or
// another one, wrong session ID) must not drop that session's heartbeats and keepalives.
func TestForgedFloodKeepsAuthenticatedSession(t *testing.T) {
	ffclimit.Reset()
	defer ffclimit.Reset()
	victim := net.UDPAddr{IP: net.IPv4(203, 0, 113, 7), Port: 50000}
	other := net.UDPAddr{IP: victim.IP, Port: 50001}
	const sid = 0x11223344

	markAuthenticatedSource(victim, sid)
	defer forgetAuthenticatedSource(victim)

	dropped := 0
	for i := 0; i < 5000; i++ {
		if !allowPacket(victim, qr2Packet(KeepAliveRequest, 0xdeadbeef)) {
			dropped++
		}
		if !allowPacket(other, qr2Packet(HeartbeatRequest, 0xdeadbeef)) {
			dropped++
		}
	}
	if dropped == 0 {
		t.Fatal("the forged flood was never limited")
	}
	for _, kind := range []byte{HeartbeatRequest, KeepAliveRequest} {
		if !allowPacket(victim, qr2Packet(kind, sid)) {
			t.Errorf("packet type %d of the authenticated session dropped after the forged flood", kind)
		}
	}
	// Once the session is gone its address is limited like any other.
	forgetAuthenticatedSource(victim)
	if allowPacket(victim, qr2Packet(HeartbeatRequest, sid)) {
		t.Error("removed session still exempt")
	}
}

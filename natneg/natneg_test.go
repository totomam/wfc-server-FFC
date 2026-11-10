package natneg

import (
	"net"
	"testing"
	"time"
)

// fakeConn is a PacketConn that drops everything written to it.
type fakeConn struct{}

func (fakeConn) ReadFrom(p []byte) (int, net.Addr, error)  { return 0, nil, net.ErrClosed }
func (fakeConn) WriteTo(p []byte, a net.Addr) (int, error) { return len(p), nil }
func (fakeConn) Close() error                              { return nil }
func (fakeConn) LocalAddr() net.Addr                       { return &net.UDPAddr{} }
func (fakeConn) SetDeadline(t time.Time) error             { return nil }
func (fakeConn) SetReadDeadline(t time.Time) error         { return nil }
func (fakeConn) SetWriteDeadline(t time.Time) error        { return nil }

var testAddr = &net.UDPAddr{IP: net.IPv4(192, 0, 2, 1), Port: 27901}

// feed runs one datagram through handleConnection the way StartServer does
// (a 1024-byte read buffer sliced to the datagram size).
func feed(t testing.TB, pkt []byte) {
	natnegConn = fakeConn{}
	buf := make([]byte, 1024)
	buf = buf[:copy(buf, pkt)]
	waitGroup.Add(1)
	handleConnection(fakeConn{}, testAddr, buf)
}

func hdr(cmd byte, cookie byte) []byte {
	return []byte{0xfd, 0xfc, 0x1e, 0x66, 0x6a, 0xb2, 0x03, cmd, 0x00, 0x00, 0x00, cookie}
}

// TestNatnegKnownCrashes feeds the three inputs from the 2026-11-10 audit (O1-1).
func TestNatnegKnownCrashes(t *testing.T) {
	cases := []struct {
		name string
		pkts [][]byte
	}{
		// NN_CONNECT_ACK with a 0-byte body: buffer[1] out of range.
		{"connect-ack-empty", [][]byte{hdr(NNConnectReply, 0x10)}},
		// 14-byte NN_REPORT: buffer[:9] out of range.
		{"report-14-bytes", [][]byte{append(hdr(NNReportRequest, 0x11), 0x00, 0x00)}},
		// Init that registers client 0 with no server IP (useGamePort=1, natneg port),
		// then the audit's report packet: empty ServerIP reaches qr2.ProcessNATNEGReport.
		{"report-empty-serverip", [][]byte{
			append(hdr(NNInitRequest, 0x01), 0x01, 0x00, 0x01, 10, 0, 0, 1, 0x00, 0x00, 'g', 0x00),
			{0xfd, 0xfc, 0x1e, 0x66, 0x6a, 0xb2, 0x03, 0x0d, 0x00, 0x00, 0x00, 0x01, 0x30, 0x00, 0x30},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic escaped handleConnection: %v", r)
				}
			}()
			for _, p := range c.pkts {
				feed(t, p)
			}
		})
	}
}

// TestNatnegHandlersNoPanic calls the handlers directly (no recover in the
// way) to pin the length/IP checks themselves.
func TestNatnegHandlersNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handler panicked: %v", r)
		}
	}()
	s := &NATNEGSession{Open: true, Version: 3, Cookie: 0x20, Clients: map[byte]*NATNEGClient{}}
	s.handleConnectReply(fakeConn{}, testAddr, []byte{}, "test", 3)
	s.handleConnectReply(fakeConn{}, testAddr, []byte{0x00}, "test", 3)
	s.handleReport(fakeConn{}, testAddr, []byte{0x00, 0x00}[:2:2], "test", 3)
	s.Clients[0] = &NATNEGClient{Index: 0, ConnectingIndex: 0, Result: map[byte]byte{}, NegotiateIP: testAddr.String()}
	s.handleReport(fakeConn{}, testAddr, []byte{0x30, 0x00, 0x30, 0, 0, 0, 0, 0, 0}, "test", 3)
	// Connecting index points at a client that does not exist.
	s.Clients[1] = &NATNEGClient{Index: 1, ConnectingIndex: 7, Result: map[byte]byte{}, ServerIP: testAddr.String()}
	s.handleReport(fakeConn{}, testAddr, []byte{0x00, 0x01, 0x01, 0, 0, 0, 0, 0, 0}, "test", 3)
}

func FuzzNatneg(f *testing.F) {
	f.Add(hdr(NNConnectReply, 0x30))
	f.Add(append(hdr(NNReportRequest, 0x31), 0x00, 0x00))
	f.Add([]byte{0xfd, 0xfc, 0x1e, 0x66, 0x6a, 0xb2, 0x03, 0x0d, 0x00, 0x00, 0x00, 0x01, 0x30, 0x00, 0x30})
	f.Add(append(hdr(NNInitRequest, 0x01), 0x01, 0x00, 0x01, 10, 0, 0, 1, 0x00, 0x00, 'g', 0x00))
	f.Add(append(hdr(NNPreInitRequest, 0x32), 0, 0, 0, 0, 0, 0))
	f.Fuzz(func(t *testing.T, data []byte) {
		// handleConnection must contain every panic itself.
		feed(t, data)
		// The handlers must also be panic-free on their own.
		if len(data) >= 12 {
			s := &NATNEGSession{Open: true, Version: data[6], Clients: map[byte]*NATNEGClient{
				0: {Index: 0, ConnectingIndex: 0, Result: map[byte]byte{}},
			}}
			body := data[12:len(data):len(data)]
			s.handleConnectReply(fakeConn{}, testAddr, body, "fuzz", data[6])
			s.handleReport(fakeConn{}, testAddr, body, "fuzz", data[6])
		}
	})
}

package natneg

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"wwfc/logging"
	"wwfc/qr2"

	"github.com/logrusorgru/aurora/v3"
)

func (session *NATNEGSession) handleReport(conn net.PacketConn, addr net.Addr, buffer []byte, _moduleName string, version byte) {
	// The reply echoes the first 9 bytes of the report.
	if len(buffer) < 9 {
		logging.Error(_moduleName, "Invalid packet size")
		return
	}

	response := createPacketHeader(version, NNReportReply, session.Cookie)
	response = append(response, buffer[:9]...)
	response[14] = 0
	_, _ = conn.WriteTo(response, addr)

	// portType := buffer[0]
	clientIndex := buffer[1]
	result := buffer[2]
	// natType := buffer[3]
	// mappingScheme := buffer[7]
	// gameName, err := common.GetString(buffer[11:])

	moduleName := "NATNEG:" + fmt.Sprintf("%08x/", session.Cookie) + addr.String()
	logging.Notice(moduleName, "Report from", aurora.BrightCyan(clientIndex), "result:", aurora.Cyan(result))

	if client, exists := session.Clients[clientIndex]; exists {
		connecting, connExists := session.Clients[client.ConnectingIndex]
		if !connExists || connecting == nil {
			logging.Error(moduleName, "Report for unknown connecting index", aurora.Cyan(client.ConnectingIndex))
			return
		}
		client.Result[client.ConnectingIndex] = result
		client.ConnectingIndex = clientIndex
		client.ConnectAck = false

		if otherResult, hasResult := connecting.Result[clientIndex]; hasResult {
			if otherResult != 1 {
				result = otherResult
			}
			if !isValidIPv4Port(client.ServerIP) || !isValidIPv4Port(connecting.ServerIP) {
				logging.Error(moduleName, "Report with invalid server IP", aurora.Cyan(client.ServerIP), aurora.Cyan(connecting.ServerIP))
			} else {
				qr2.ProcessNATNEGReport(result, client.ServerIP, connecting.ServerIP)
			}
		}
	}

	// Send remaining requests
	session.sendConnectRequests(moduleName)
}

// isValidIPv4Port reports whether s is "a.b.c.d:port", the only form
// common.IPFormatToInt accepts without panicking.
func isValidIPv4Port(s string) bool {
	host, port, err := net.SplitHostPort(s)
	if err != nil {
		return false
	}
	if ip := net.ParseIP(host); ip == nil || ip.To4() == nil || strings.Contains(host, ":") {
		return false
	}
	p, err := strconv.Atoi(port)
	return err == nil && p >= 0 && p <= 0xffff
}

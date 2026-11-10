package nas

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"wwfc/common"
	"wwfc/logging"

	"github.com/logrusorgru/aurora/v3"
)

// maxNASLineBytes bounds one HTTP request or header line read by filterDuplicateHost (NET-16).
const maxNASLineBytes = 8192

var errNASLineTooLong = errors.New("nas: request line too long")

// readNASLine reads one line of at most the reader's buffer size (ReadSlice never grows the buffer).
func readNASLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) {
		return "", errNASLineTooLong
	}
	return string(line), err
}

type httpListener struct {
	net.Listener
}

type nasInConn struct {
	net.Conn
	in io.Reader
}

func (c nasInConn) Read(b []byte) (int, error) { return c.in.Read(b) }

func (c nasInConn) Close() error {
	err := c.Conn.Close()
	if closer, ok := c.in.(io.Closer); ok {
		_ = closer.Close()
	}
	return err
}

func (l *httpListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()
	go func() {
		r := bufio.NewReaderSize(conn, maxNASLineBytes)
		if err := filterDuplicateHost(pw, r); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		_, err := io.Copy(pw, r)
		_ = pw.CloseWithError(err)
	}()
	return &nasInConn{
		Conn: conn,
		in:   pr,
	}, nil
}

func listenAndServe() {
	logging.Notice("NAS", "Starting HTTP server on", aurora.BrightCyan(server.Addr))

	l, err := net.Listen("tcp", server.Addr)
	common.ShouldNotError(err)
	listener := &httpListener{Listener: l}

	defer func() {
		common.ShouldNotError(listener.Close())
	}()

	err = server.Serve(listener)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}

type tlsListener struct {
	net.Listener
}

func (l *tlsListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	tlsConn := tlsConnection{
		Conn: conn,
	}

	pr, pw := io.Pipe()
	go func() {
		r := bufio.NewReaderSize(&tlsConn, maxNASLineBytes)
		if err := filterDuplicateHost(pw, r); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		_, err := io.Copy(pw, r)
		_ = pw.CloseWithError(err)
	}()
	return &nasInConn{
		Conn: &tlsConn,
		in:   pr,
	}, nil
}

func listenAndServeTLS() {
	logging.Notice("NAS", "Starting HTTPS server on", aurora.BrightCyan(tlsServer.Addr))

	l, err := net.Listen("tcp", tlsServer.Addr)
	common.ShouldNotError(err)
	listener := &tlsListener{Listener: l}

	defer func() {
		common.ShouldNotError(listener.Close())
	}()

	err = tlsServer.Serve(listener)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}

// filterDuplicateHost wraps a net.Conn and filters out duplicate Host headers from the HTTP request,
// making the invalid requests sent by DWC acceptable to the standard library's HTTP server.
// Every line is bounded by the reader's buffer (maxNASLineBytes): a longer line closes the connection
// instead of being buffered (NET-16).
func filterDuplicateHost(w *io.PipeWriter, r *bufio.Reader) error {
	// Read the first line of the HTTP request
	line, err := readNASLine(r)
	if errors.Is(err, errNASLineTooLong) {
		return err
	}
	_, errPipe := w.Write([]byte(line))
	if err != nil || errPipe != nil {
		return errPipe
	}

	// Is this an HTTP request we handle?
	if !strings.HasSuffix(line, "HTTP/1.1\r\n") {
		return nil
	}

	// Iterate through the HTTP headers and remove any duplicate Host headers
	hostSeen := false
	for errPipe == nil {
		line, err = readNASLine(r)
		if errors.Is(err, errNASLineTooLong) {
			return err
		}
		if err != nil || line == "\r\n" || line == "\n" {
			_, errPipe = w.Write([]byte(line))
			break
		}

		if strings.HasPrefix(strings.ToLower(line), "host:") {
			if hostSeen {
				continue
			}
			hostSeen = true
		}
		_, errPipe = w.Write([]byte(line))
	}
	return errPipe
}

package gpcm

import (
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"wwfc/common"
)

// fakeFrontend stands in for the frontend RPC server so that SendPacket and
// CloseConnection can run in unit tests; it records what would be sent.
type fakeFrontend struct{}

var (
	frontendOnce sync.Once
	frontendMu   sync.Mutex
	frontendSent = map[uint64][]string{}
)

func (fakeFrontend) SendPacket(args common.RPCFrontendPacket, _ *struct{}) error {
	frontendMu.Lock()
	defer frontendMu.Unlock()
	frontendSent[args.Index] = append(frontendSent[args.Index], string(args.Data))
	return nil
}

func (fakeFrontend) CloseConnection(args common.RPCFrontendPacket, _ *struct{}) error {
	return nil
}

func startFakeFrontend(t *testing.T) {
	t.Helper()
	frontendOnce.Do(func() {
		srv := rpc.NewServer()
		if err := srv.RegisterName("RPCFrontendPacket", fakeFrontend{}); err != nil {
			t.Fatal(err)
		}
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		go srv.Accept(l)

		dir := t.TempDir()
		cfg := "<Config><backendFrontendAddress>" + l.Addr().String() + "</backendFrontendAddress><logOutput>StdOut</logOutput></Config>"
		if err := os.WriteFile(filepath.Join(dir, "config.xml"), []byte(cfg), 0o600); err != nil {
			t.Fatal(err)
		}
		wd, _ := os.Getwd()
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		common.GetConfig() // loaded once, kept in memory
		_ = os.Chdir(wd)
	})
}

// takeSent returns and clears the packets sent to a connection index.
func takeSent(index uint64) []string {
	frontendMu.Lock()
	defer frontendMu.Unlock()
	s := frontendSent[index]
	delete(frontendSent, index)
	return s
}

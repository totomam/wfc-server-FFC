package nas

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"time"
	"wwfc/common"
)

// NET-10: Nd (dls) requests carry `token` = the SVCLOC servicetoken (ROM: DWCi_Svl_ParseHttp reads "servicetoken",
// setupHttp posts it as "token" on every Nd request, the modded result/lbname/hub/table actions included). svcloc
// svc 9000 now issues a token bound to the userid (HMAC, per-process key, dlsTokenTTL), and the per-user actions
// (result, lbname, hub/news mail) reject a userid without its token. Limit: SVCLOC itself is authenticated by the userid
// only (the frozen client holds no other secret), see docs/sessions/fix03/NET-10.md in ffc-decomp.

const dlsTokenTTL = 24 * time.Hour

var (
	dlsTokenKey = func() []byte {
		k := make([]byte, 32)
		if _, err := rand.Read(k); err != nil {
			panic(err)
		}
		return k
	}()
	dlsTokenNow = time.Now
)

func dlsTokenMAC(userID uint64, exp uint32) []byte {
	var b [12]byte
	binary.BigEndian.PutUint64(b[:8], userID)
	binary.BigEndian.PutUint32(b[8:], exp)
	m := hmac.New(sha256.New, dlsTokenKey)
	m.Write(b[:])
	return m.Sum(nil)[:16]
}

// dlsIssueToken: 56 hex chars (userid 16, expiry 8, mac 32). Hex is also valid DWC-base64 (length % 4 == 0), so the
// request parses whether the client re-encodes the token or posts it as received.
func dlsIssueToken(userID uint64) string {
	exp := uint32(dlsTokenNow().Add(dlsTokenTTL).Unix())
	var b [12]byte
	binary.BigEndian.PutUint64(b[:8], userID)
	binary.BigEndian.PutUint32(b[8:], exp)
	return hex.EncodeToString(b[:]) + hex.EncodeToString(dlsTokenMAC(userID, exp))
}

func dlsTokenValid(tok string, userID uint64) bool {
	raw, err := hex.DecodeString(tok)
	if err != nil || len(raw) != 28 {
		return false
	}
	uid, exp := binary.BigEndian.Uint64(raw[:8]), binary.BigEndian.Uint32(raw[8:12])
	return uid == userID && int64(exp) > dlsTokenNow().Unix() && hmac.Equal(raw[12:], dlsTokenMAC(uid, exp))
}

// dlsUser: the request's userid, only if its token was issued for it (as decoded by parseAuthRequest, or re-encoded:
// a client that posts the token raw had it "decoded" into other bytes).
func dlsUser(fields map[string][]byte) (uint64, bool) {
	userID, err := strconv.ParseUint(string(fields["userid"]), 10, 64)
	if err != nil || userID == 0 {
		return 0, false
	}
	tok := fields["token"]
	if dlsTokenValid(string(tok), userID) || dlsTokenValid(common.Base64DwcEncoding.EncodeToString(tok), userID) {
		return userID, true
	}
	return 0, false
}

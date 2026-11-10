package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"sync"
	"time"
)

// SecretHeader carries the admin API secret (NET-14). GET requests take it only from this header (the old
// ?secret= query form leaked it into access logs, proxies and shell history); POST bodies may still carry "secret".
const SecretHeader = "X-API-Secret"

// Failed-auth rate limit (NET-14): more than authFailMax failures from one IP within authFailWindow -> 429 until
// the window ends, without checking the secret.
var (
	authFailMax    = 10
	authFailWindow = time.Minute
	authFailMu     sync.Mutex
	authFails      = map[string]*authFailEntry{}
)

type authFailEntry struct {
	count int
	reset time.Time
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func authLimited(ip string) bool {
	authFailMu.Lock()
	defer authFailMu.Unlock()
	e := authFails[ip]
	if e == nil {
		return false
	}
	if time.Now().After(e.reset) {
		delete(authFails, ip)
		return false
	}
	return e.count >= authFailMax
}

func authFailed(ip string) {
	authFailMu.Lock()
	defer authFailMu.Unlock()
	now := time.Now()
	if len(authFails) > 4096 {
		for k, e := range authFails {
			if now.After(e.reset) {
				delete(authFails, k)
			}
		}
	}
	e := authFails[ip]
	if e == nil || now.After(e.reset) {
		e = &authFailEntry{reset: now.Add(authFailWindow)}
		authFails[ip] = e
	}
	e.count++
}

// checkAuth authenticates r (header secret, else the body's) and writes 429/401 on refusal.
func checkAuth(w http.ResponseWriter, r *http.Request, authInfo AuthInfo, requiredRole Role) bool {
	if requiredRole == RoleNone {
		return true
	}
	ip := clientIP(r)
	if authLimited(ip) {
		w.Header().Set("Retry-After", strconv.Itoa(int(authFailWindow/time.Second)))
		replyError(w, http.StatusTooManyRequests, APIErrorFailedAuthentication)
		return false
	}
	if h := r.Header.Get(SecretHeader); h != "" {
		authInfo.Secret = h
	}
	if !authenticate(authInfo, requiredRole) {
		authFailed(ip)
		replyError(w, http.StatusUnauthorized, APIErrorFailedAuthentication)
		return false
	}
	return true
}

// maxPostBodySize caps every admin POST body read by parsePost (NET-15).
const maxPostBodySize = 64 << 10

type APIErrorString string

const (
	APIErrorFailedAuthentication APIErrorString = "failed_authentication"
	APIErrorInvalidQuery         APIErrorString = "invalid_query"
	APIErrorInvalidProfileID     APIErrorString = "invalid_profile_id"
	APIErrorInvalidReason        APIErrorString = "invalid_reason"
	APIErrorInvalidBanLength     APIErrorString = "invalid_ban_length"
	APIErrorBanFailed            APIErrorString = "ban_failed"
	APIErrorUnbanFailed          APIErrorString = "unban_failed"
	APIErrorBanNotFound          APIErrorString = "ban_not_found"
)

type APIError struct {
	Error string `json:"error"`
}

type Role string

// Values currently just made up
const (
	RoleNone      Role = "none" // Not signed in
	RoleUser      Role = "user"
	RoleAdmin     Role = "admin"
	RoleModerator Role = "moderator"
)

type AuthInfo struct {
	Secret string `json:"secret"`
}

var (
	errOptionsRequest  = errors.New("OPTIONS request")
	errIncorrectMethod = errors.New("incorrect HTTP method")
	errNoAuthInfo      = errors.New("request struct does not contain AuthInfo fields")
	errAuthFailed      = errors.New("authentication failed")
)

func parseGet(r *http.Request, w http.ResponseWriter, requiredRole Role) (query url.Values, err error) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	switch r.Method {
	case http.MethodGet:
		break

	case http.MethodOptions:
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, "+SecretHeader)
		w.WriteHeader(http.StatusNoContent)
		return nil, errOptionsRequest

	default:
		w.Header().Set("Allow", "GET, OPTIONS")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return nil, errIncorrectMethod
	}

	query, err = url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return nil, err
	}

	if requiredRole == RoleNone {
		return query, nil
	}

	if !checkAuth(w, r, AuthInfo{}, requiredRole) {
		return nil, errAuthFailed
	}
	return query, nil
}

func parsePost(r *http.Request, w http.ResponseWriter, parsed any, requiredRole Role) error {
	return parsePostLimit(r, w, parsed, requiredRole, maxPostBodySize)
}

// parsePostLimit is parsePost with a handler-specific body cap (the boot upload carries a 175.5 KB window).
func parsePostLimit(r *http.Request, w http.ResponseWriter, parsed any, requiredRole Role, limit int64) error {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	switch r.Method {
	case http.MethodPost:
		break

	case http.MethodOptions:
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, "+SecretHeader)
		w.WriteHeader(http.StatusNoContent)
		return errOptionsRequest

	default:
		w.Header().Set("Allow", "POST, OPTIONS")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return errIncorrectMethod
	}

	// The secret is inside the JSON body, so the size cap has to come before authentication.
	jsonData, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
		return err
	}

	err = json.Unmarshal(jsonData, parsed)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return err
	}

	if requiredRole == RoleNone {
		return nil
	}

	authInfo, ok := reflect.ValueOf(parsed).Elem().FieldByName("AuthInfo").Interface().(AuthInfo)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return errNoAuthInfo
	}
	if !checkAuth(w, r, authInfo, requiredRole) {
		return errAuthFailed
	}
	return nil
}

// authenticate refuses everything while apiSecret is empty (NET-33) and compares in constant time (NET-14).
func authenticate(authInfo AuthInfo, requiredRole Role) bool {
	if requiredRole == RoleNone {
		return true
	}
	if apiSecret == "" || authInfo.Secret == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(authInfo.Secret), []byte(apiSecret)) == 1
}

func replyError(w http.ResponseWriter, statusCode int, errMsg APIErrorString) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	jsonData := []byte(`{"error":"` + string(errMsg) + `"}`)
	w.Header().Set("Content-Length", strconv.Itoa(len(jsonData)))
	_, _ = w.Write(jsonData)
}

func replyOK(w http.ResponseWriter, data any) {
	if data == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	var jsonData []byte
	if reflect.ValueOf(data).Kind() == reflect.String {
		// Assume it's already JSON
		jsonData = []byte(data.(string))
	} else {
		var err error
		jsonData, err = json.Marshal(data)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Length", strconv.Itoa(len(jsonData)))
	_, _ = w.Write(jsonData)
}

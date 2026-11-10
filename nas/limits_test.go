package nas

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wwfc/common"
)

// endlessLine yields `size` bytes of 'A' with no newline and counts what was consumed.
type endlessLine struct{ size, read int }

func (e *endlessLine) Read(p []byte) (int, error) {
	if e.read >= e.size {
		return 0, io.EOF
	}
	n := len(p)
	if n > e.size-e.read {
		n = e.size - e.read
	}
	for i := range p[:n] {
		p[i] = 'A'
	}
	e.read += n
	return n, nil
}

// NET-16: a 1 MiB first line with no newline is refused after the line limit, without buffering it.
func TestFilterDuplicateHostLongLine(t *testing.T) {
	for _, header := range []bool{false, true} {
		var prefix string
		if header {
			prefix = "POST /ac HTTP/1.1\r\n"
		}
		src := &endlessLine{size: 1 << 20}
		pr, pw := io.Pipe()
		forwarded := make(chan int, 1)
		go func() {
			n, _ := io.Copy(io.Discard, pr)
			forwarded <- int(n)
		}()
		r := bufio.NewReaderSize(io.MultiReader(strings.NewReader(prefix), src), maxNASLineBytes)
		err := filterDuplicateHost(pw, r)
		_ = pw.CloseWithError(err)
		n := <-forwarded
		if err == nil {
			t.Errorf("header=%v: no error for a %d-byte line", header, src.size)
		}
		if src.read > 2*maxNASLineBytes {
			t.Errorf("header=%v: consumed %d bytes of the long line, limit %d", header, src.read, maxNASLineBytes)
		}
		if n > len(prefix)+maxNASLineBytes {
			t.Errorf("header=%v: forwarded %d bytes", header, n)
		}
	}
}

// Normal DWC requests (duplicate Host) still pass the filter.
func TestFilterDuplicateHostNormal(t *testing.T) {
	in := "POST /ac HTTP/1.1\r\nHost: nas.example\r\nHost: nas.example\r\nContent-Length: 0\r\n\r\nbody"
	pr, pw := io.Pipe()
	out := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(pr)
		out <- string(b)
	}()
	r := bufio.NewReaderSize(strings.NewReader(in), maxNASLineBytes)
	err := filterDuplicateHost(pw, r)
	if err == nil {
		_, err = io.Copy(pw, r)
	}
	_ = pw.CloseWithError(err)
	want := "POST /ac HTTP/1.1\r\nHost: nas.example\r\nContent-Length: 0\r\n\r\nbody"
	if got := <-out; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// NET-16: a 200k-tab `words` value is answered in under 100 ms.
func TestProfanityManyWords(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 0, 5000)
	for i := 0; i < 5000; i++ {
		lines = append(lines, "badword"+strings.Repeat("x", i%50))
	}
	path := filepath.Join(dir, "profanity.txt")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := profanityFilePath
	profanityFilePath = path
	defer func() { profanityFilePath = old }()
	if err := CacheProfanityFile(); err != nil {
		t.Fatal(err)
	}

	enc := func(s string) string { return common.Base64DwcEncoding.EncodeToString([]byte(s)) }
	form := url.Values{}
	form.Set("unitcd", enc("0"))
	form.Set("wenc", enc("UTF-8"))
	form.Set("words", enc(strings.Repeat("a\t", 200000)))
	req := httptest.NewRequest(http.MethodPost, "/pr", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	start := time.Now()
	handleAuthProfanityEndpoint(rec, req)
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Errorf("200k-tab words took %v (status %d)", d, rec.Code)
	}
}

// IsBadWord still matches case-insensitively and through the symbol equivalences.
func TestIsBadWordMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profanity.txt")
	if err := os.WriteFile(path, []byte("Badword\n\nother\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := profanityFilePath
	profanityFilePath = path
	defer func() { profanityFilePath = old }()
	for word, want := range map[string]bool{"badword": true, "BADWORD": true, "b4dw0rd": true, "OTHER": true, "fine": false, "": false} {
		if got, err := IsBadWord(word); got != want || err != nil {
			t.Errorf("IsBadWord(%q) = %v, %v; want %v", word, got, err, want)
		}
	}
}

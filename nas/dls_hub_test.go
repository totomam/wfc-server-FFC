package nas

import (
	"testing"
	"wwfc/ffcbonus"
)

func TestHubPages(t *testing.T) {
	gen := []ffcbonus.NewsItem{{Name: "97", Body: []byte("ME")}, {Name: "98", Body: []byte("TOP")}, {Name: "196", Body: []byte("LAST")},
		{Name: "165", Body: []byte("MAIL 1/1")}}
	files := []dlcEntry{{name: "70", data: nil}, {name: "3"}}
	if p := hubPages("top", gen, nil, "VDEE"); len(p) != 1 || p[0] != "TOP" {
		t.Fatal(p)
	}
	if p := hubPages("me", gen, nil, "VDEE"); len(p) != 2 || p[0] != "ME" || p[1] != "LAST" {
		t.Fatal(p)
	}
	old := dlcDir
	defer func() { dlcDir = old }()
	dlcDir = t.TempDir()
	if p := hubPages("mail", gen, files, "VDEE"); len(p) != 1 || p[0] != "MAIL 1/1" { // file 70 missing on disk
		t.Fatal(p)
	}
	if p := hubPages("mail", nil, nil, "VDEE"); len(p) != 1 || p[0] == "" {
		t.Fatal("empty page", p)
	}
	if hubPages("x", gen, nil, "VDEE") != nil {
		t.Fatal("unknown section")
	}
	if _, ok := dlsActions["hub"]; !ok {
		t.Fatal("hub not registered")
	}
}

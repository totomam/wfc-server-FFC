package database

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"wwfc/filter"
)

func testGenerateFilter(t *testing.T, expression string) {
	tree, err := filter.Parse(expression)
	if err != nil {
		t.Error(err)
		return
	}

	fmt.Printf("tree: %s\n", tree.String())

	query, err := createSqlFilter(nil, tree)
	if err != nil {
		t.Error(err)
		return
	}

	fmt.Printf("query: %s\n", query)
}

func TestSakeFilter(t *testing.T) {
	testGenerateFilter(t, "ownerid = 1")
	testGenerateFilter(t, "course = 12 and gameid = 1687 and time < 195")
	testGenerateFilter(t, "wiiid = 8880667695734424 and num_ratings = 0")

	// From bandbrosEUds
	testGenerateFilter(t, "SUBSTRING(country, 5, 1) & 32 > 0")
	testGenerateFilter(t, "uniqueid = '7ACeoVFlNDUwNTAxGRkCCv///////////////////////////////w=='")

	// Random complex filter I made up
	testGenerateFilter(t, "gameid = 1687 and (test = 'aaa' or (DROP = 100000) and ((((UPDATE != 4))))) or (1 = 2 + 7  &   SELECT - 9)")
}

func sqlFor(t *testing.T, expression string) (query string, err error) {
	t.Helper()
	tree, perr := filter.Parse(expression)
	if perr != nil {
		t.Fatal(perr)
	}
	return createSqlFilter(nil, tree)
}

func TestSakeFilterVirtualFields(t *testing.T) {
	q, err := sqlFor(t, "num_ratings > 0 and average_rating >= 100")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"FROM sake_ratings", "COUNT(*)", "AVG(sr.rating)"} {
		if !strings.Contains(q, want) {
			t.Errorf("query lacks %q: %s", want, q)
		}
	}
	// stored JSON must not be used for the virtual fields
	if strings.Contains(q, "fields->'num_ratings'") || strings.Contains(q, "fields->'average_rating'") {
		t.Errorf("virtual field read from stored JSON: %s", q)
	}
	for _, expr := range []string{"update_time > 5", "my_rating = 1"} {
		if _, err := sqlFor(t, expr); !errors.Is(err, ErrSakeFilterUnsupported) {
			t.Errorf("%q: want ErrSakeFilterUnsupported, got %v", expr, err)
		}
	}
}

func TestSakeFilterQuoting(t *testing.T) {
	q, err := sqlFor(t, "team_name = 'abc' and player_level >= 5")
	if err != nil {
		t.Fatal(err)
	}
	if e := (&expression{}); e.filterPushArg("a'b") != "'a''b'" {
		t.Errorf("quote not doubled: %s", e.filterPushArg("a'b"))
	}
	if !strings.Contains(q, "fields->'player_level'->>'value'") {
		t.Errorf("field access missing: %s", q)
	}
}

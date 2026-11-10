package filter

import "testing"

// TestEvalRepanic: a filter that makes eval panic with an error value (not a
// string) must come back as an error, not re-panic out of Eval (O1-2).
func TestEvalRepanic(t *testing.T) {
	tree, err := Parse("maxplayers > foo")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Eval panicked: %v", r)
		}
	}()
	contexts := []map[string]string{
		{"maxplayers": "4"},
		{"maxplayers": "4", "foo": "bar"},
		{},
	}
	for _, ctx := range contexts {
		_, err := Eval(tree, ctx, "dental2ds")
		if err == nil {
			t.Errorf("Eval(%v) returned no error", ctx)
		}
	}
}

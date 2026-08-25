package config

import (
	"encoding/json"
	"testing"
)

// TestDefaultSendsNoNullFields pins the shape this structure crosses to the UI in.
//
// The Host tab merges the saved configuration over its own defaults. A field
// that arrives as null therefore replaces a default the UI depends on, rather
// than leaving it alone — and in JavaScript the failure surfaces far from here,
// as a call on nothing. A nil slice or map in Go marshals to null, so the rule
// is that this structure never emits one.
func TestDefaultSendsNoNullFields(t *testing.T) {
	b, err := json.Marshal(Default())
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		t.Fatal(err)
	}
	for name, value := range fields {
		if string(value) == "null" {
			t.Errorf("%q is null; omit it or give it an empty value, or a consumer merging "+
				"this over its own defaults loses that default\n  got: %s", name, b)
		}
	}
}

package match

import "encoding/json"

// maxFields is the largest number of entries kept in Failure.Fields.
const maxFields = 20

// Field is one concrete leaf value behind a failure: the field of an object
// or array element a condition looked at, and what it held. It lets a
// consumer show briefly which fields are wrong without the full message.
type Field struct {
	// Path is the full path to the leaf, e.g. containers.#(state=="running")#.2.name.
	Path string `json:"path"`
	// JSONPath is the location inside a parsed JSON document (json.assert,
	// json.schema); empty otherwise.
	JSONPath string `json:"json_path,omitempty"`
	// Operator is the leaf operator, e.g. "in", "length gt", "exists".
	Operator string `json:"operator"`
	// Actual is the value at the leaf, cut like Failure.Actual; nil when
	// the path is missing.
	Actual any `json:"actual"`
	// Missing is true when the path is absent (not an explicit null).
	Missing bool `json:"missing,omitempty"`
}

// MarshalJSON writes an empty list of fields as [] rather than null.
func (f Failure) MarshalJSON() ([]byte, error) {
	type plain Failure
	p := plain(f)
	if p.Fields == nil {
		p.Fields = []Field{}
	}
	return json.Marshal(p)
}

// leafField describes the value v at the recorder's location.
func (r recorder) leafField(op string, v value) Field {
	f := Field{Path: r.path, JSONPath: r.jsonPath, Operator: r.prefix + op, Missing: !v.exists}
	if v.exists {
		f.Actual = cut(v.v)
	}
	return f
}

// hold notes a leaf condition that held, when the recorder collects them
// (inside "not", whose failure is made of conditions that held).
func (r recorder) hold(op string, v value) {
	if r.held != nil {
		*r.held = append(*r.held, r.leafField(op, v))
	}
}

// fieldsOf returns the fields of the failures, in order.
func fieldsOf(failures []Failure) []Field {
	var out []Field
	for _, f := range failures {
		out = append(out, f.Fields...)
	}
	return out
}

// merge removes repeated fields (same path, JSON path and operator) and
// keeps at most maxFields; nil when there are none.
func merge(fields []Field) []Field {
	type key struct{ path, jsonPath, op string }
	seen := map[key]struct{}{}
	var out []Field
	for _, f := range fields {
		k := key{f.Path, f.JSONPath, f.Operator}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, f)
		if len(out) == maxFields {
			break
		}
	}
	return out
}

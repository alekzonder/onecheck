package module

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// DecodeParams decodes the request params into out and rejects unknown keys,
// so that a typo in a rule is reported instead of silently ignored.
func DecodeParams(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}
	return nil
}

// WantsOr reports whether the request asks for the field. When the request
// names no fields at all it returns cheap: collect inexpensive fields only.
func (r Request) WantsOr(field string, cheap bool) bool {
	if len(r.Fields) == 0 {
		return cheap
	}
	return r.Wants(field)
}

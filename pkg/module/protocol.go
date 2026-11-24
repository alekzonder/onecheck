// Package module is the SDK for onecheck check modules.
//
// A module is a standalone executable named onecheck-<name>. onecheck starts
// it once per check and exchanges JSON over stdin/stdout. A module only
// collects field values; every comparison is done by onecheck itself.
package module

import "encoding/json"

// ProtocolVersion is the version of the JSON protocol implemented by this SDK.
const ProtocolVersion = 1

// FieldType describes the kind of value a module field holds.
type FieldType string

// Field types. Bytes and Duration are numbers (bytes and seconds) for which
// onecheck accepts unit literals such as "10MB" and "1h" in conditions.
const (
	TypeString   FieldType = "string"
	TypeNumber   FieldType = "number"
	TypeBool     FieldType = "bool"
	TypeObject   FieldType = "object"
	TypeArray    FieldType = "array"
	TypeBytes    FieldType = "bytes"
	TypeDuration FieldType = "duration"
)

// FieldInfo documents one field a module can return.
type FieldInfo struct {
	Type        FieldType `json:"type"`
	Description string    `json:"description"`
	// Output marks a field as diagnostic output (a command's stdout, a
	// response body): onecheck asks for it on every check and keeps its
	// tail in the result, whether or not a condition uses it.
	Output bool `json:"output,omitempty"`
	// Elements gives, for an array field whose elements are objects, the
	// types of the keys of an element. Keys typed bytes or duration accept
	// unit literals in conditions, like fields of those types.
	Elements map[string]FieldType `json:"elements,omitempty"`
}

// Description is the answer to "onecheck-<name> describe".
type Description struct {
	Protocol     int                  `json:"protocol"`
	Name         string               `json:"name"`
	Version      string               `json:"version"`
	Description  string               `json:"description"`
	ParamsSchema json.RawMessage      `json:"params_schema,omitempty"`
	Fields       map[string]FieldInfo `json:"fields"`
}

// Request is sent on stdin of "onecheck-<name> check".
type Request struct {
	Protocol int             `json:"protocol"`
	Rule     string          `json:"rule"`
	Params   json.RawMessage `json:"params"`
	// Fields lists the fields the rule's conditions use, plus the module's
	// output fields when onecheck saves output. A module may skip
	// collecting anything else. Empty means "collect everything cheap".
	Fields       []string `json:"fields"`
	MaxFieldSize int64    `json:"max_field_size"`
	TimeoutMS    int64    `json:"timeout_ms"`
}

// Wants reports whether the request asks for the given field.
func (r Request) Wants(field string) bool {
	for _, f := range r.Fields {
		if f == field {
			return true
		}
	}
	return false
}

// Error reports that a module could not collect its data.
type Error struct {
	Message string `json:"message"`
}

// Response is written to stdout by "onecheck-<name> check".
type Response struct {
	Fields    map[string]any `json:"fields"`
	Truncated []string       `json:"truncated"`
	Error     *Error         `json:"error"`
}

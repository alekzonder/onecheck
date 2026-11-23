package match

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type jsonAssert struct {
	path string
	node node
}

// jsonNode parses a string value as JSON and checks the parsed document.
type jsonNode struct {
	asserts []jsonAssert
	schema  *jsonschema.Schema
}

func (n jsonNode) missingAware() bool { return false }

func (n jsonNode) eval(r recorder, v value) bool {
	if !v.exists {
		r.fail("json", "valid JSON", v, "")
		return false
	}
	text, ok := v.v.(string)
	if !ok {
		r.add(Failure{Operator: "json", Actual: cut(v.v), Message: fmt.Sprintf(
			"%s: value is not valid JSON: got a %s, want a string holding JSON", r.where(), typeName(v.v))})
		return false
	}
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(text))
	if err != nil {
		r.add(Failure{Operator: "json", Actual: cut(text), Message: fmt.Sprintf(
			"%s: value is not valid JSON: %v", r.where(), err)})
		return false
	}
	ok = true
	data := []byte(text)
	for _, a := range n.asserts {
		sub := recorder{path: r.path, jsonPath: a.path, out: r.out}
		if !a.node.eval(sub, lookup(data, a.path)) {
			ok = false
		}
	}
	if n.schema != nil && !n.validate(r, doc) {
		ok = false
	}
	return ok
}

func (n jsonNode) validate(r recorder, doc any) bool {
	err := n.schema.Validate(doc)
	if err == nil {
		return true
	}
	var verr *jsonschema.ValidationError
	if !errors.As(err, &verr) {
		r.add(Failure{Operator: "schema", Message: fmt.Sprintf("%s: schema validation failed: %v", r.where(), err)})
		return false
	}
	for _, unit := range leaves(verr.BasicOutput()) {
		loc := unit.InstanceLocation
		if loc == "" {
			loc = "/"
		}
		r.add(Failure{Operator: "schema", JSONPath: loc, Message: fmt.Sprintf(
			"%s json %s: schema violation: %s", r.path, loc, unit.Error.String())})
	}
	return false
}

// leaves returns the output units that carry an actual error message.
func leaves(u *jsonschema.OutputUnit) []jsonschema.OutputUnit {
	var out []jsonschema.OutputUnit
	var walk func(u jsonschema.OutputUnit)
	walk = func(u jsonschema.OutputUnit) {
		if len(u.Errors) == 0 {
			if u.Error != nil {
				out = append(out, u)
			}
			return
		}
		for _, child := range u.Errors {
			walk(child)
		}
	}
	walk(*u)
	return out
}

func compileJSON(arg any) (node, error) {
	spec, ok := arg.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("json: needs a mapping with assert and/or schema")
	}
	n := jsonNode{}
	for key, v := range spec {
		switch key {
		case "assert":
			asserts, err := compileAsserts(v)
			if err != nil {
				return nil, err
			}
			n.asserts = asserts
		case "schema":
			schema, err := compileSchema(v)
			if err != nil {
				return nil, err
			}
			n.schema = schema
		default:
			return nil, fmt.Errorf("json: unknown key %q (expected assert, schema)", key)
		}
	}
	if len(n.asserts) == 0 && n.schema == nil {
		return nil, fmt.Errorf("json: needs at least one of assert, schema")
	}
	return n, nil
}

func compileAsserts(v any) ([]jsonAssert, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("json.assert: needs a list of {path, <operators>}")
	}
	asserts := make([]jsonAssert, 0, len(list))
	for i, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("json.assert[%d]: needs a mapping with path and operators", i)
		}
		path, ok := m["path"].(string)
		if !ok || path == "" {
			return nil, fmt.Errorf("json.assert[%d]: path is required", i)
		}
		ops := make(map[string]any, len(m))
		for k, val := range m {
			if k != "path" {
				ops[k] = val
			}
		}
		n, err := compileCond(ops, "")
		if err != nil {
			return nil, fmt.Errorf("json.assert[%d] (%s): %w", i, path, err)
		}
		asserts = append(asserts, jsonAssert{path: path, node: n})
	}
	return asserts, nil
}

func compileSchema(v any) (*jsonschema.Schema, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("json.schema: %w", err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("json.schema: %w", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("schema.json", doc); err != nil {
		return nil, fmt.Errorf("json.schema: %w", err)
	}
	schema, err := c.Compile("schema.json")
	if err != nil {
		return nil, fmt.Errorf("json.schema: %w", err)
	}
	return schema, nil
}

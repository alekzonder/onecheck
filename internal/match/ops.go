package match

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/alekzonder/onecheck/internal/units"
	"github.com/alekzonder/onecheck/pkg/module"
)

// builder compiles the argument of one leaf operator.
type builder func(name string, arg any, ft module.FieldType) (node, error)

var leafOps map[string]builder

func init() {
	leafOps = map[string]builder{
		"equals":       buildEquals(false),
		"not_equals":   buildEquals(true),
		"contains":     buildContains(false),
		"not_contains": buildContains(true),
		"starts_with":  buildStringOp(strings.HasPrefix),
		"ends_with":    buildStringOp(strings.HasSuffix),
		"matches":      buildMatches(false),
		"not_matches":  buildMatches(true),
		"gt":           buildCompare(func(a, b float64) bool { return a > b }),
		"gte":          buildCompare(func(a, b float64) bool { return a >= b }),
		"lt":           buildCompare(func(a, b float64) bool { return a < b }),
		"lte":          buildCompare(func(a, b float64) bool { return a <= b }),
		"in":           buildIn(false),
		"not_in":       buildIn(true),
		"type":         buildType,
	}
}

// unitArg converts a unit literal ("10MB", "1h") into a number when the
// condition is on a field typed bytes or duration.
func unitArg(arg any, ft module.FieldType) (any, error) {
	s, isString := arg.(string)
	if !isString {
		return arg, nil
	}
	switch ft {
	case module.TypeBytes:
		return units.ParseBytes(s)
	case module.TypeDuration:
		return units.ParseDuration(s)
	}
	return arg, nil
}

func buildEquals(negate bool) builder {
	return func(name string, arg any, ft module.FieldType) (node, error) {
		want, err := unitArg(arg, ft)
		if err != nil {
			return nil, err
		}
		return leaf{op: name, expected: arg, fn: func(v any) (bool, string) {
			return reflect.DeepEqual(v, want) != negate, ""
		}}, nil
	}
}

func buildContains(negate bool) builder {
	return func(name string, arg any, _ module.FieldType) (node, error) {
		return leaf{op: name, expected: arg, fn: func(v any) (bool, string) {
			found, note := contains(v, arg)
			if note != "" {
				return false, note
			}
			return found != negate, ""
		}}, nil
	}
}

func contains(v, arg any) (found bool, note string) {
	switch x := v.(type) {
	case string:
		s, ok := arg.(string)
		if !ok {
			return false, "a string can only contain a string"
		}
		return strings.Contains(x, s), ""
	case []any:
		for _, item := range x {
			if reflect.DeepEqual(item, arg) {
				return true, ""
			}
		}
		return false, ""
	case map[string]any:
		key, ok := arg.(string)
		if !ok {
			return false, "an object can only contain a string key"
		}
		_, found := x[key]
		return found, ""
	}
	return false, "not applicable to a " + typeName(v)
}

func buildStringOp(fn func(s, arg string) bool) builder {
	return func(name string, arg any, _ module.FieldType) (node, error) {
		want, ok := arg.(string)
		if !ok {
			return nil, fmt.Errorf("needs a string, got %s", toJSON(arg))
		}
		return leaf{op: name, expected: arg, fn: func(v any) (bool, string) {
			s, ok := v.(string)
			if !ok {
				return false, "not applicable to a " + typeName(v)
			}
			return fn(s, want), ""
		}}, nil
	}
}

func buildMatches(negate bool) builder {
	return func(name string, arg any, _ module.FieldType) (node, error) {
		pattern, ok := arg.(string)
		if !ok {
			return nil, fmt.Errorf("needs a regular expression string, got %s", toJSON(arg))
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regular expression: %w", err)
		}
		return leaf{op: name, expected: arg, fn: func(v any) (bool, string) {
			s, ok := v.(string)
			if !ok {
				return false, "not applicable to a " + typeName(v)
			}
			return re.MatchString(s) != negate, ""
		}}, nil
	}
}

func buildCompare(cmp func(a, b float64) bool) builder {
	return func(name string, arg any, ft module.FieldType) (node, error) {
		conv, err := unitArg(arg, ft)
		if err != nil {
			return nil, err
		}
		want, ok := conv.(float64)
		if !ok {
			return nil, fmt.Errorf("needs a number, got %s", toJSON(arg))
		}
		return leaf{op: name, expected: arg, fn: func(v any) (bool, string) {
			n, ok := v.(float64)
			if !ok {
				return false, "not applicable to a " + typeName(v)
			}
			return cmp(n, want), ""
		}}, nil
	}
}

func buildIn(negate bool) builder {
	return func(name string, arg any, ft module.FieldType) (node, error) {
		list, ok := arg.([]any)
		if !ok {
			return nil, fmt.Errorf("needs a list, got %s", toJSON(arg))
		}
		want := make([]any, len(list))
		for i, item := range list {
			conv, err := unitArg(item, ft)
			if err != nil {
				return nil, err
			}
			want[i] = conv
		}
		return leaf{op: name, expected: arg, fn: func(v any) (bool, string) {
			for _, item := range want {
				if reflect.DeepEqual(v, item) {
					return !negate, ""
				}
			}
			return negate, ""
		}}, nil
	}
}

var typeNames = map[string]bool{"string": true, "number": true, "bool": true, "object": true, "array": true, "null": true}

func buildType(name string, arg any, _ module.FieldType) (node, error) {
	want, ok := arg.(string)
	if !ok || !typeNames[want] {
		return nil, fmt.Errorf("needs one of string, number, bool, object, array, null; got %s", toJSON(arg))
	}
	return leaf{op: name, expected: arg, fn: func(v any) (bool, string) {
		got := typeName(v)
		if got != want {
			return false, "it is a " + got
		}
		return true, ""
	}}, nil
}

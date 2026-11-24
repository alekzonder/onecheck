// Package glob compiles shell patterns (fnmatch style) into regular
// expressions matching a whole string.
//
// Syntax: * any run of characters (including /), ? one character, [abc],
// [a-z] and [!abc] character classes. Everything else is literal.
package glob

import (
	"fmt"
	"regexp"
	"strings"
)

// Has reports whether s contains pattern characters, i.e. is a pattern and
// not a plain name.
func Has(s string) bool { return strings.ContainsAny(s, "*?[") }

// Compile turns a shell pattern into an anchored, case-sensitive regexp.
func Compile(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString(`(?s)^`)
	runes := []rune(pattern)
	for i := 0; i < len(runes); i++ {
		switch r := runes[i]; r {
		case '*':
			b.WriteString(`.*`)
		case '?':
			b.WriteString(`.`)
		case '[':
			end, class, err := parseClass(runes, i)
			if err != nil {
				return nil, fmt.Errorf("invalid pattern %q: %w", pattern, err)
			}
			b.WriteString(class)
			i = end
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString(`$`)
	return regexp.Compile(b.String())
}

// parseClass reads a character class starting at runes[start] == '[' and
// returns the index of its closing ']' and the class as regexp syntax.
func parseClass(runes []rune, start int) (int, string, error) {
	var b strings.Builder
	b.WriteByte('[')
	i := start + 1
	if i < len(runes) && runes[i] == '!' {
		b.WriteByte('^')
		i++
	}
	first := true
	for ; i < len(runes); i++ {
		r := runes[i]
		if r == ']' && !first {
			b.WriteByte(']')
			return i, b.String(), nil
		}
		first = false
		switch r {
		case '\\', '[', ']', '^':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return 0, "", fmt.Errorf("unclosed [")
}

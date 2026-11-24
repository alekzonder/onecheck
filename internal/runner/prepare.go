package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/alekzonder/onecheck/internal/config"
	"github.com/alekzonder/onecheck/internal/match"
	"github.com/alekzonder/onecheck/internal/modules"
)

// loadedModule is a described module, or the reason it cannot be used.
type loadedModule struct {
	info *modules.Info
	// outputs are the fields whose tail is saved with every result.
	outputs []string
	schema  *jsonschema.Schema
	types   match.FieldTypes
	// elements are the key types of the elements of array fields.
	elements match.ElementTypes
	err      error
}

// moduleSet describes each module at most once per run.
type moduleSet struct {
	dir   string
	cache map[string]*loadedModule
}

func newModuleSet(dir string) *moduleSet {
	return &moduleSet{dir: dir, cache: map[string]*loadedModule{}}
}

func (s *moduleSet) get(ctx context.Context, name string) *loadedModule {
	if m, ok := s.cache[name]; ok {
		return m
	}
	m := s.load(ctx, name)
	s.cache[name] = m
	return m
}

func (s *moduleSet) load(ctx context.Context, name string) *loadedModule {
	path := modules.Path(s.dir, name)
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return &loadedModule{err: fmt.Errorf("module %q is not installed in %s (see: onecheck module install --help)", name, s.dir)}
	}
	info, err := modules.Describe(ctx, path)
	if err != nil {
		return &loadedModule{err: fmt.Errorf("module %q is not usable: %w", name, err)}
	}
	if err := info.CompatError(); err != nil {
		return &loadedModule{err: err}
	}
	m := &loadedModule{info: info, types: match.FieldTypes{}, elements: match.ElementTypes{}}
	for field, fi := range info.Fields {
		m.types[field] = fi.Type
		if len(fi.Elements) > 0 {
			m.elements[field] = fi.Elements
		}
		if fi.Output {
			m.outputs = append(m.outputs, field)
		}
	}
	sort.Strings(m.outputs)
	if len(info.ParamsSchema) > 0 {
		m.schema, err = compileSchema(info.ParamsSchema)
		if err != nil {
			return &loadedModule{err: fmt.Errorf("module %q has an invalid params_schema: %w", name, err)}
		}
	}
	return m
}

func compileSchema(raw json.RawMessage) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("params.json", doc); err != nil {
		return nil, err
	}
	return c.Compile("params.json")
}

// prepared is a rule ready to be executed, or the reason it cannot be.
type prepared struct {
	rule   config.Rule
	mod    *loadedModule
	check  *match.Check
	params json.RawMessage
	err    error
}

// prepare validates one rule against its module. Every problem it finds is a
// rule-level error: it makes this rule unusable and nothing else. An
// unusable rule allows no output to be shown: its event_output was not, or
// could not be, checked against the module.
func prepare(ctx context.Context, rule config.Rule, mods *moduleSet) prepared {
	p := prepareRule(ctx, rule, mods)
	if p.err != nil {
		p.rule.EventOutput = nil
	}
	return p
}

func prepareRule(ctx context.Context, rule config.Rule, mods *moduleSet) prepared {
	p := prepared{rule: rule}
	if rule.Err != nil {
		p.err = rule.Err
		return p
	}
	p.mod = mods.get(ctx, rule.Module)
	if p.mod.err != nil {
		p.err = p.mod.err
		return p
	}
	params := rule.Params
	if params == nil {
		params = map[string]any{}
	}
	p.params, p.err = json.Marshal(params)
	if p.err != nil {
		p.err = fmt.Errorf("params are not plain data: %w", p.err)
		return p
	}
	if p.err = validateParams(p.mod.schema, p.params); p.err != nil {
		return p
	}
	if p.err = validateEventOutput(rule, p.mod); p.err != nil {
		return p
	}
	p.check, p.err = match.Compile(rule.Check, p.mod.types, p.mod.elements)
	return p
}

// validateEventOutput checks that event_output names output fields of the
// module: only those are saved in a result and can be shown.
func validateEventOutput(rule config.Rule, mod *loadedModule) error {
	for _, name := range rule.EventOutput {
		if slices.Contains(mod.outputs, name) {
			continue
		}
		if len(mod.outputs) == 0 {
			return fmt.Errorf("event_output: module %q has no output fields", rule.Module)
		}
		return fmt.Errorf("event_output: %q is not an output field of module %q (output fields: %s)",
			name, rule.Module, strings.Join(mod.outputs, ", "))
	}
	return nil
}

func validateParams(schema *jsonschema.Schema, params json.RawMessage) error {
	if schema == nil {
		return nil
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(params))
	if err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}
	err = schema.Validate(doc)
	if err == nil {
		return nil
	}
	var verr *jsonschema.ValidationError
	if !errors.As(err, &verr) {
		return fmt.Errorf("invalid params: %w", err)
	}
	var msgs []string
	collectSchemaErrors(verr, &msgs)
	return fmt.Errorf("invalid params: %s", strings.Join(msgs, "; "))
}

// schemaPrinter words schema violations: "missing property 'path'".
var schemaPrinter = message.NewPrinter(language.English)

func collectSchemaErrors(e *jsonschema.ValidationError, out *[]string) {
	if len(e.Causes) == 0 {
		loc := "/" + strings.Join(e.InstanceLocation, "/")
		msg := e.ErrorKind.LocalizedString(schemaPrinter)
		// A module forbids a param in some combination with "name": false.
		if _, forbidden := e.ErrorKind.(*kind.FalseSchema); forbidden {
			msg = "not allowed together with the other params"
		}
		*out = append(*out, fmt.Sprintf("%s: %s", loc, msg))
		return
	}
	for _, cause := range e.Causes {
		collectSchemaErrors(cause, out)
	}
}

// Validate checks every rule of the config, enabled or not, without running
// anything, and returns all rule-level problems.
func Validate(ctx context.Context, cfg *config.Config) []error {
	mods := newModuleSet(cfg.Paths.Modules)
	var errs []error
	for _, rule := range cfg.Rules {
		if p := prepare(ctx, rule, mods); p.err != nil {
			errs = append(errs, fmt.Errorf("%s: rule %s: %w", rule.Source, rule.Name, p.err))
		}
	}
	return errs
}

package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"

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
	err     error
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
	m := &loadedModule{info: info, types: match.FieldTypes{}}
	for field, fi := range info.Fields {
		m.types[field] = fi.Type
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
// rule-level error: it makes this rule unusable and nothing else.
func prepare(ctx context.Context, rule config.Rule, mods *moduleSet) prepared {
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
	p.check, p.err = match.Compile(rule.Check, p.mod.types)
	return p
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

func collectSchemaErrors(e *jsonschema.ValidationError, out *[]string) {
	if len(e.Causes) == 0 {
		loc := "/" + strings.Join(e.InstanceLocation, "/")
		*out = append(*out, fmt.Sprintf("%s: %v", loc, e.ErrorKind))
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

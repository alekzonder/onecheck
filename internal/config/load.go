package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// fileRules is the part of a config file that holds rules; include files
// may contain nothing else.
type fileRules struct {
	Rules []yaml.Node `yaml:"rules"`
}

// rawRule mirrors the YAML shape of a rule.
type rawRule struct {
	Name        string         `yaml:"name"`
	Module      string         `yaml:"module"`
	Tags        []string       `yaml:"tags"`
	Enabled     *bool          `yaml:"enabled"`
	Timeout     *Duration      `yaml:"timeout"`
	FailAfter   *int           `yaml:"fail_after"`
	PassAfter   *int           `yaml:"pass_after"`
	OutputTail  *Size          `yaml:"output_tail"`
	EventParams *bool          `yaml:"event_params"`
	EventOutput []string       `yaml:"event_output"`
	RemindEvery *Duration      `yaml:"remind_every"`
	Params      map[string]any `yaml:"params"`
	Check       map[string]any `yaml:"check"`
}

// Load reads the config file and its includes. A returned error means the
// whole config is unusable; problems of a single rule are stored in Rule.Err.
func Load(path string) (*Config, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("cannot read config: %w", err)
	}
	cfg := defaultConfig()
	cfg.Path = abs
	main := struct {
		*Config   `yaml:",inline"`
		RuleNodes []yaml.Node `yaml:"rules"`
	}{Config: cfg}
	if err := decodeStrict(data, &main); err != nil {
		return nil, fmt.Errorf("%s: %w", abs, err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", abs, err)
	}
	cfg.resolvePaths()

	sources := map[string]string{}
	if err := cfg.addRules(main.RuleNodes, abs, sources); err != nil {
		return nil, err
	}
	files, err := cfg.includeFiles()
	if err != nil {
		return nil, err
	}
	cfg.Includes = files
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("cannot read include: %w", err)
		}
		var inc fileRules
		if err := decodeStrict(data, &inc); err != nil {
			return nil, fmt.Errorf("%s: %w (include files may only contain \"rules\")", file, err)
		}
		if err := cfg.addRules(inc.Rules, file, sources); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func decodeStrict(data []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func (c *Config) validate() error {
	if c.Name == "" {
		return errors.New(`"name" is required: it separates the results, state and logs of different configs`)
	}
	if !NameRE.MatchString(c.Name) {
		return fmt.Errorf("invalid name %q: use lower-case letters, digits, '_', '.', '-' and start with a letter or digit", c.Name)
	}
	switch c.Log.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log.level: %q is not one of debug, info, warn, error", c.Log.Level)
	}
	if c.Log.Format != "text" && c.Log.Format != "json" {
		return fmt.Errorf("log.format: %q is not one of text, json", c.Log.Format)
	}
	if c.Run.Parallel < 1 {
		return errors.New("run.parallel must be at least 1")
	}
	if c.Run.MaxFieldSize < 1 {
		return errors.New("run.max_field_size must be positive")
	}
	if c.Defaults.FailAfter < 1 || c.Defaults.PassAfter < 1 {
		return errors.New("defaults.fail_after and defaults.pass_after must be at least 1")
	}
	if c.Defaults.Timeout <= 0 {
		return errors.New("defaults.timeout must be positive")
	}
	return nil
}

// resolvePaths makes relative directories relative to the config file.
func (c *Config) resolvePaths() {
	base := filepath.Dir(c.Path)
	for _, p := range []*string{&c.Log.Dir, &c.Paths.Modules, &c.Paths.Results, &c.Paths.Events, &c.Paths.State} {
		if !filepath.IsAbs(*p) {
			*p = filepath.Join(base, *p)
		}
	}
}

func (c *Config) includeFiles() ([]string, error) {
	var files []string
	seen := map[string]bool{c.Path: true}
	for _, pattern := range c.Include {
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(filepath.Dir(c.Path), pattern)
		}
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("include %q: %w", pattern, err)
		}
		sort.Strings(matches)
		for _, m := range matches {
			if info, err := os.Stat(m); err != nil || info.IsDir() || seen[m] {
				continue
			}
			seen[m] = true
			files = append(files, m)
		}
	}
	return files, nil
}

// addRules converts rule nodes from one file and appends them to the config.
func (c *Config) addRules(nodes []yaml.Node, source string, sources map[string]string) error {
	for i := range nodes {
		node := &nodes[i]
		var ident struct {
			Name string `yaml:"name"`
		}
		if err := node.Decode(&ident); err != nil || ident.Name == "" {
			return fmt.Errorf("%s: rule #%d (line %d) has no name", source, i+1, node.Line)
		}
		if prev, dup := sources[ident.Name]; dup {
			return fmt.Errorf("duplicate rule name %q: defined in %s and in %s", ident.Name, prev, source)
		}
		sources[ident.Name] = source
		c.Rules = append(c.Rules, c.buildRule(ident.Name, node, source))
	}
	return nil
}

func (c *Config) buildRule(name string, node *yaml.Node, source string) Rule {
	rule := Rule{
		Name:        name,
		Source:      source,
		Enabled:     true,
		Timeout:     time.Duration(c.Defaults.Timeout),
		FailAfter:   c.Defaults.FailAfter,
		PassAfter:   c.Defaults.PassAfter,
		OutputTail:  int64(c.Defaults.OutputTail),
		EventParams: c.Defaults.EventParams,
		RemindEvery: DefaultRemindEvery,
	}
	if d := c.Defaults.RemindEvery; d != nil {
		rule.RemindEvery, rule.RemindExplicit = time.Duration(*d), true
	}
	var raw rawRule
	// Decoding the node (not its text) keeps anchors defined in other rules
	// usable; unknown keys are then checked by hand.
	err := node.Decode(&raw)
	if err == nil {
		err = unknownRuleKeys(node)
	}
	if err != nil {
		rule.Err = fmt.Errorf("invalid rule: %s", strings.TrimPrefix(err.Error(), "yaml: "))
		return rule
	}
	rule.Module, rule.Tags, rule.Check = raw.Module, raw.Tags, raw.Check
	rule.EventOutput = raw.EventOutput
	if raw.Enabled != nil {
		rule.Enabled = *raw.Enabled
	}
	if raw.Timeout != nil {
		rule.Timeout = time.Duration(*raw.Timeout)
	}
	if raw.FailAfter != nil {
		rule.FailAfter = *raw.FailAfter
	}
	if raw.PassAfter != nil {
		rule.PassAfter = *raw.PassAfter
	}
	if raw.OutputTail != nil {
		rule.OutputTail = int64(*raw.OutputTail)
	}
	if raw.EventParams != nil {
		rule.EventParams = *raw.EventParams
	}
	if raw.RemindEvery != nil {
		rule.RemindEvery, rule.RemindExplicit = time.Duration(*raw.RemindEvery), true
	}
	// expandEnv builds a new value, so the params as written stay intact.
	rule.RawParams = raw.Params
	params, err := expandEnv(raw.Params)
	if err != nil {
		rule.Err = err
		return rule
	}
	rule.Params, _ = params.(map[string]any)
	rule.Err = rule.validate()
	return rule
}

var ruleKeys = map[string]bool{
	"name": true, "module": true, "tags": true, "enabled": true, "timeout": true,
	"fail_after": true, "pass_after": true, "output_tail": true, "event_params": true, "event_output": true, "remind_every": true, "params": true, "check": true,
	"<<": true, // YAML merge key
}

// unknownRuleKeys rejects keys a rule does not have, so typos are reported.
func unknownRuleKeys(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode && node.Alias != nil {
		node = node.Alias
	}
	if node.Kind != yaml.MappingNode {
		return errors.New("a rule must be a mapping")
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if key := node.Content[i].Value; !ruleKeys[key] {
			return fmt.Errorf("line %d: field %s not found in a rule (known: name, module, tags, enabled, timeout, fail_after, pass_after, output_tail, event_params, event_output, remind_every, params, check)",
				node.Content[i].Line, key)
		}
	}
	return nil
}

// moduleNameRE keeps a module name from pointing outside the modules directory.
var moduleNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func (r *Rule) validate() error {
	switch {
	case !NameRE.MatchString(r.Name):
		return fmt.Errorf("invalid rule name %q: use lower-case letters, digits, '_', '.', '-' and start with a letter or digit", r.Name)
	case r.Module == "":
		return errors.New(`"module" is required`)
	case !moduleNameRE.MatchString(r.Module):
		return fmt.Errorf("invalid module name %q: use lower-case letters, digits, '_', '-'", r.Module)
	case r.FailAfter < 1 || r.PassAfter < 1:
		return errors.New("fail_after and pass_after must be at least 1")
	case r.Timeout <= 0:
		return errors.New("timeout must be positive")
	}
	return nil
}

// expandEnv replaces ${VAR} in every string of v with the environment value.
// "$${VAR}" produces a literal "${VAR}"; any other "$" is left alone. An
// undefined variable is an error.
func expandEnv(v any) (any, error) {
	switch x := v.(type) {
	case string:
		return expandString(x)
	case map[string]any:
		if x == nil {
			return x, nil
		}
		out := make(map[string]any, len(x))
		for k, item := range x {
			e, err := expandEnv(item)
			if err != nil {
				return nil, err
			}
			out[k] = e
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			e, err := expandEnv(item)
			if err != nil {
				return nil, err
			}
			out[i] = e
		}
		return out, nil
	}
	return v, nil
}

func expandString(s string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		rest := s[i:]
		switch {
		case strings.HasPrefix(rest, "$${"):
			// Escape: "$${VAR}" is a literal "${VAR}".
			b.WriteString("${")
			i += 2
		case strings.HasPrefix(rest, "${"):
			end := strings.IndexByte(rest, '}')
			if end < 0 {
				return "", fmt.Errorf("unterminated ${ in %q", s)
			}
			name := rest[2:end]
			val, ok := os.LookupEnv(name)
			if !ok {
				return "", fmt.Errorf("environment variable %s is not set (used as ${%s} in params)", name, name)
			}
			b.WriteString(val)
			i += end
		default:
			// Any other "$" is left alone, so shell syntax survives.
			b.WriteByte(s[i])
		}
	}
	return b.String(), nil
}

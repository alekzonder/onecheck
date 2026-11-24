package config

import (
	"time"

	"gopkg.in/yaml.v3"

	"github.com/alekzonder/onecheck/internal/units"
)

// Size is a number of bytes written in config as "10MB".
type Size int64

// UnmarshalYAML parses a size literal.
func (s *Size) UnmarshalYAML(n *yaml.Node) error {
	v, err := units.ParseBytes(n.Value)
	if err != nil {
		return err
	}
	*s = Size(v)
	return nil
}

// Duration is a time span written in config as "30s", "1h" or "7d".
type Duration time.Duration

// UnmarshalYAML parses a duration literal.
func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	v, err := units.ParseDuration(n.Value)
	if err != nil {
		return err
	}
	*d = Duration(v * float64(time.Second))
	return nil
}

// Rotate holds the log rotation rules used by "onecheck logrotate".
type Rotate struct {
	MaxSize  Size     `yaml:"max_size"`
	Keep     int      `yaml:"keep"`
	MaxAge   Duration `yaml:"max_age"`
	Compress bool     `yaml:"compress"`
}

// Log configures logging.
type Log struct {
	Level  string `yaml:"level"`
	Dir    string `yaml:"dir"`
	Format string `yaml:"format"`
	Rotate Rotate `yaml:"rotate"`
}

// Paths holds the base directories used by onecheck.
type Paths struct {
	Modules string `yaml:"modules"`
	Results string `yaml:"results"`
	Events  string `yaml:"events"`
	State   string `yaml:"state"`
}

// Events configures the event queue.
type Events struct {
	TTL Duration `yaml:"ttl"`
}

// Run configures how checks are executed.
type Run struct {
	Parallel     int  `yaml:"parallel"`
	MaxFieldSize Size `yaml:"max_field_size"`
}

// Defaults are applied to rules that do not set the value themselves.
type Defaults struct {
	Timeout   Duration `yaml:"timeout"`
	FailAfter int      `yaml:"fail_after"`
	PassAfter int      `yaml:"pass_after"`
	// OutputTail is how much of each output field of a module (a command's
	// stdout and stderr, a response body) is kept in the result.
	OutputTail Size `yaml:"output_tail"`
	// EventParams says whether events carry the rule's params (as written
	// in the config) and may be shown in full by a notification service.
	EventParams bool `yaml:"event_params"`
	// RemindEvery is how often a rule that stays failed writes a reminder
	// event; 0 turns reminders off. Nil means the config does not set it
	// and DefaultRemindEvery applies.
	RemindEvery *Duration `yaml:"remind_every"`
}

// DefaultRemindEvery is the reminder interval of a config that sets none.
const DefaultRemindEvery = time.Hour

// Modules configures module management.
type Modules struct {
	Repos []string `yaml:"repos"`
}

// Config is a loaded onecheck configuration.
type Config struct {
	Name string `yaml:"name"`
	// Path is the absolute path of the main config file.
	Path    string   `yaml:"-"`
	Log     Log      `yaml:"log"`
	Paths   Paths    `yaml:"paths"`
	Include []string `yaml:"include"`
	// Includes are the include files that were read, in reading order.
	Includes []string `yaml:"-"`
	Events   Events   `yaml:"events"`
	Run      Run      `yaml:"run"`
	Defaults Defaults `yaml:"defaults"`
	Modules  Modules  `yaml:"modules"`
	Rules    []Rule   `yaml:"-"`
}

// Rule is one configured check.
type Rule struct {
	Name      string
	Module    string
	Tags      []string
	Enabled   bool
	Timeout   time.Duration
	FailAfter int
	PassAfter int
	// OutputTail is how many bytes of each output field are saved; 0 = none.
	OutputTail int64
	// Params is the module input, with ${VAR} already expanded.
	Params map[string]any
	// RawParams is the module input as written in the config, before ${VAR}
	// expansion. Only events use it: it is safe to show where Params is not.
	RawParams map[string]any
	// EventParams is false when the rule's details must stay out of events.
	EventParams bool
	// RemindEvery is how often the rule writes a reminder event while its
	// confirmed state is fail; 0 = never.
	RemindEvery time.Duration
	// RemindExplicit is true when RemindEvery was written in the config,
	// on the rule or in defaults, and false for the built-in default.
	RemindExplicit bool
	// Check holds the raw conditions for the matching engine.
	Check map[string]any
	// Source is the file the rule was read from.
	Source string
	// Err is a problem that makes only this rule unusable.
	Err error
}

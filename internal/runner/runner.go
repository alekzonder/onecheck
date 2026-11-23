// Package runner executes the rules of a config: it validates each rule,
// asks its module for field values, evaluates the conditions, and records
// state, events and results.
package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alekzonder/onecheck/internal/config"
	"github.com/alekzonder/onecheck/internal/modules"
	"github.com/alekzonder/onecheck/internal/result"
	"github.com/alekzonder/onecheck/internal/state"
	"github.com/alekzonder/onecheck/internal/store"
	"github.com/alekzonder/onecheck/pkg/module"
)

// Options narrows a run to some rules. Empty means every enabled rule.
type Options struct {
	Rules []string
	Tags  []string
}

func (o Options) filtered() bool { return len(o.Rules) > 0 || len(o.Tags) > 0 }

// Run executes the selected rules and returns their results in config order.
// A returned error means onecheck itself could not do its job (lock, files);
// problems of individual rules are results with status "error".
func Run(ctx context.Context, cfg *config.Config, opts Options, log *slog.Logger) ([]result.Result, error) {
	st, err := store.Open(cfg)
	if err != nil {
		return nil, err
	}
	unlock, err := st.Lock()
	if err != nil {
		return nil, err
	}
	defer unlock()

	rules, err := selectRules(cfg, opts)
	if err != nil {
		return nil, err
	}
	log.Debug("rules selected", "count", len(rules), "of", len(cfg.Rules), "modules_dir", cfg.Paths.Modules,
		"results_dir", cfg.ResultsDir(), "state_dir", cfg.StateDir(), "events_dir", cfg.EventsDir())
	mods := newModuleSet(cfg.Paths.Modules)
	for _, rule := range rules {
		if _, seen := mods.cache[rule.Module]; seen || rule.Err != nil {
			continue
		}
		if m := mods.get(ctx, rule.Module); m.err == nil {
			log.Debug("module loaded", "module", m.info.Name, "version", m.info.Version,
				"protocol", m.info.Protocol, "path", m.info.Path)
		}
	}
	jobs := make([]prepared, len(rules))
	for i, rule := range rules {
		jobs[i] = prepare(ctx, rule, mods)
	}

	results := execute(ctx, cfg, jobs, log)
	if ctx.Err() != nil {
		// Checks killed half-way say nothing about what they were checking.
		// Record nothing: no state change, no event, no result.
		return nil, errors.New("run interrupted, nothing was recorded")
	}
	now := time.Now().UTC()
	for i := range results {
		if err := record(st, cfg, &results[i], now, log); err != nil {
			return results[:i], fmt.Errorf("rule %s: %w", results[i].Rule, err)
		}
	}
	if !opts.filtered() {
		keep := make([]string, len(rules))
		for i, rule := range rules {
			keep[i] = rule.Name
		}
		if err := st.Prune(keep); err != nil {
			return results, fmt.Errorf("cannot remove files of deleted rules: %w", err)
		}
	}
	return results, nil
}

func selectRules(cfg *config.Config, opts Options) ([]config.Rule, error) {
	known := map[string]bool{}
	for _, rule := range cfg.Rules {
		known[rule.Name] = true
	}
	for _, name := range opts.Rules {
		if !known[name] {
			return nil, fmt.Errorf("no rule named %q in %s", name, cfg.Path)
		}
	}
	var out []config.Rule
	for _, rule := range cfg.Rules {
		if rule.Enabled && selected(rule, opts) {
			out = append(out, rule)
		}
	}
	return out, nil
}

func selected(rule config.Rule, opts Options) bool {
	if !opts.filtered() {
		return true
	}
	for _, name := range opts.Rules {
		if name == rule.Name {
			return true
		}
	}
	for _, want := range opts.Tags {
		for _, tag := range rule.Tags {
			if tag == want {
				return true
			}
		}
	}
	return false
}

// execute runs the jobs with at most run.parallel at a time.
func execute(ctx context.Context, cfg *config.Config, jobs []prepared, log *slog.Logger) []result.Result {
	results := make([]result.Result, len(jobs))
	sem := make(chan struct{}, cfg.Run.Parallel)
	var wg sync.WaitGroup
	for i := range jobs {
		sem <- struct{}{}
		if ctx.Err() != nil {
			<-sem
			break
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = runOne(ctx, cfg, jobs[i], log)
		}()
	}
	wg.Wait()
	return results
}

// runOne produces the raw outcome of one rule; state is applied later.
func runOne(ctx context.Context, cfg *config.Config, job prepared, log *slog.Logger) result.Result {
	rule := job.rule
	res := result.Result{
		Config:    cfg.Name,
		Rule:      rule.Name,
		Module:    rule.Module,
		Tags:      rule.Tags,
		FailAfter: rule.FailAfter,
		PassAfter: rule.PassAfter,
		StartedAt: time.Now().UTC(),
		Status:    result.Pass,
	}
	fail := func(err error) result.Result {
		msg := err.Error()
		res.Status, res.Error = result.Error, &msg
		res.DurationMS = time.Since(res.StartedAt).Milliseconds()
		return res
	}
	if job.mod != nil && job.mod.info != nil {
		res.ModuleVersion = job.mod.info.Version
	}
	if job.err != nil {
		return fail(job.err)
	}
	req := module.Request{
		Rule:         rule.Name,
		Params:       job.params,
		Fields:       wantedFields(job),
		MaxFieldSize: int64(cfg.Run.MaxFieldSize),
	}
	log.Debug("collecting fields", "rule", rule.Name, "module", rule.Module, "params", string(job.params),
		"fields", req.Fields, "timeout", rule.Timeout.String())
	var stderr bytes.Buffer
	resp, err := modules.Check(ctx, job.mod.info, req, rule.Timeout, &stderr)
	if stderr.Len() > 0 {
		log.Debug("module stderr", "rule", rule.Name, "module", rule.Module, "stderr", strings.TrimSpace(stderr.String()))
	}
	if err != nil {
		return fail(err)
	}
	res.Output = savedOutput(resp.Fields, job)
	if resp.Error != nil {
		return fail(errors.New(resp.Error.Message))
	}
	log.Debug("fields collected", append([]any{"rule", rule.Name, "module", rule.Module,
		"duration_ms", time.Since(res.StartedAt).Milliseconds()}, fieldAttrs(resp)...)...)
	failures, err := job.check.Eval(resp.Fields, resp.Truncated)
	if err != nil {
		return fail(err)
	}
	res.DurationMS = time.Since(res.StartedAt).Milliseconds()
	if len(failures) > 0 {
		res.Status, res.Failures = result.Fail, failures
	}
	return res
}

// record applies the outcome to the rule's state and writes, in this order,
// the event (if the confirmed state changed), the state and the result. If
// the event cannot be written the state stays untouched, so the transition
// is detected again on the next run instead of being lost.
func record(st *store.Store, cfg *config.Config, res *result.Result, now time.Time, log *slog.Logger) error {
	prev, err := st.LoadState(res.Rule)
	if errors.Is(err, store.ErrCorrupt) {
		log.Warn("state file is corrupt, treating the rule as new", "rule", res.Rule, "error", err)
		prev = nil
	} else if err != nil {
		return err
	}
	from := state.Pass
	if prev != nil {
		from = prev.State
	}
	next, transition := state.Advance(prev, res.OK(), res.FailAfter, res.PassAfter, now)
	res.State = next.State
	res.Consecutive = next.Consecutive(res.OK())
	if transition != "" {
		res.Transition = &transition
		event := result.Event{
			ID:         store.EventID(now, cfg.Name, res.Rule, transition),
			Config:     cfg.Name,
			Rule:       res.Rule,
			Transition: transition,
			From:       from,
			To:         next.State,
			At:         now,
			ExpiresAt:  now.Add(time.Duration(cfg.Events.TTL)),
			Result:     *res,
		}
		if err := st.WriteEvent(event); err != nil {
			return fmt.Errorf("cannot write event: %w", err)
		}
		log.Info("state changed", "rule", res.Rule, "transition", transition, "event", event.ID)
	}
	if err := st.SaveState(res.Rule, next); err != nil {
		return fmt.Errorf("cannot save state: %w", err)
	}
	if err := st.WriteResult(*res); err != nil {
		return fmt.Errorf("cannot write result: %w", err)
	}
	log.Debug("state updated", "rule", res.Rule, "from", from, "to", next.State,
		"consecutive", res.Consecutive, "fail_after", res.FailAfter, "pass_after", res.PassAfter)
	logResult(log, res)
	return nil
}

// wantedFields is what the module is asked for: the fields the conditions
// use and, when output is saved, the module's output fields.
func wantedFields(job prepared) []string {
	fields := job.check.Fields()
	if job.rule.OutputTail <= 0 {
		return fields
	}
	for _, name := range job.mod.outputs {
		if !slices.Contains(fields, name) {
			fields = append(fields, name)
		}
	}
	return fields
}

// savedOutput picks the module's output fields for the result. A string is
// cut to its last output_tail bytes and then starts with "…"; any other
// value is kept whole if its JSON fits into the limit and dropped otherwise,
// because a cut JSON value is useless.
func savedOutput(fields map[string]any, job prepared) map[string]any {
	limit := job.rule.OutputTail
	if limit <= 0 {
		return nil
	}
	out := map[string]any{}
	for _, name := range job.mod.outputs {
		switch value := fields[name].(type) {
		case nil:
		case string:
			if int64(len(value)) > limit {
				value = "…" + strings.ToValidUTF8(value[int64(len(value))-limit:], "")
			}
			out[name] = value
		default:
			if data, err := json.Marshal(value); err == nil && int64(len(data)) <= limit {
				out[name] = value
			}
		}
	}
	return out
}

// logValueLimit is how much of one field value a debug record shows.
const logValueLimit = 256

// fieldAttrs renders the fields a module returned for a debug record, in a
// stable order, with long values cut.
func fieldAttrs(resp *module.Response) []any {
	names := make([]string, 0, len(resp.Fields))
	for name := range resp.Fields {
		names = append(names, name)
	}
	sort.Strings(names)
	attrs := make([]any, 0, len(names)+1)
	for _, name := range names {
		value := resp.Fields[name]
		if text, ok := value.(string); ok && len(text) > logValueLimit {
			value = fmt.Sprintf("%s... (%d bytes)", strings.ToValidUTF8(text[:logValueLimit], ""), len(text))
		}
		attrs = append(attrs, slog.Any(name, value))
	}
	if len(resp.Truncated) > 0 {
		attrs = append(attrs, slog.Any("truncated", resp.Truncated))
	}
	return []any{slog.Group("values", attrs...)}
}

func logResult(log *slog.Logger, res *result.Result) {
	attrs := []any{"rule", res.Rule, "module", res.Module, "status", res.Status, "state", res.State, "duration_ms", res.DurationMS}
	switch res.Status {
	case result.Pass:
		log.Info("check passed", attrs...)
	case result.Fail:
		msgs := make([]string, len(res.Failures))
		for i, f := range res.Failures {
			msgs[i] = f.Message
		}
		log.Info("check failed", append(attrs, "failures", msgs)...)
	default:
		log.Info("check error", append(attrs, "error", *res.Error)...)
	}
}

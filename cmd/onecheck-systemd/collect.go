package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/alekzonder/onecheck/pkg/module"
)

type params struct {
	Unit       string   `json:"unit"`
	User       bool     `json:"user"`
	Properties []string `json:"properties"`
}

// now is the clock; tests replace it.
var now = time.Now

func collect(ctx context.Context, req module.Request) (map[string]any, error) {
	var p params
	if err := module.DecodeParams(req.Params, &p); err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	want := slices.Concat(commonProps, serviceProps, timerProps, p.Properties)
	slices.Sort(want)
	props, err := show(ctx, p.User, p.Unit, slices.Compact(want))
	if err != nil {
		return nil, err
	}
	if props["Id"] == "" {
		return nil, fmt.Errorf("systemctl show %s printed no unit", p.Unit)
	}
	out := unitFields(props, now())
	if len(p.Properties) > 0 {
		raw := map[string]any{}
		for _, name := range p.Properties {
			if v, ok := props[name]; ok {
				raw[name] = propertyValue(v)
			}
		}
		out["properties"] = raw
	}
	if unitType(props["Id"]) == "timer" && props["Unit"] != "" && wantsActivated(req) {
		act, err := show(ctx, p.User, props["Unit"], activatedProps)
		if err != nil {
			return nil, err
		}
		maps.Copy(out, activatedFields(act))
	}
	return out, nil
}

func (p params) validate() error {
	if p.Unit == "" {
		return errors.New("invalid params: unit is required")
	}
	// systemctl show expands patterns and prints every matching unit.
	if strings.ContainsAny(p.Unit, "*?[") {
		return fmt.Errorf("invalid params: unit %q is a pattern; name one unit", p.Unit)
	}
	for _, name := range p.Properties {
		if !propertyName.MatchString(name) {
			return fmt.Errorf("invalid params: property %q is not a systemd property name", name)
		}
	}
	return nil
}

var propertyName = regexp.MustCompile(`^[A-Za-z0-9]+$`)

func wantsActivated(req module.Request) bool {
	for _, f := range []string{"service_result", "service_exit_status", "service_active_state"} {
		if req.WantsOr(f, true) {
			return true
		}
	}
	return false
}

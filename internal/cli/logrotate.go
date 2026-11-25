package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/alekzonder/onecheck/internal/config"
	"github.com/alekzonder/onecheck/internal/rotate"
	"github.com/alekzonder/onecheck/internal/store"
)

const logrotateLong = `Rotate the log of the config and remove expired events.

Logs: when <log.dir>/<config name>/onecheck.log is larger than
log.rotate.max_size it becomes onecheck.log.1 (.gz with compress) and older
files move up. Rotated files beyond log.rotate.keep or older than
log.rotate.max_age are removed.

Events: files in the event queue (paths.events) whose expires_at has passed
are removed. The queue is shared by all configs, so one config's logrotate
cleans it for all of them. An event lives for events.ttl after it was written.

With a directory of configs (--config <dir>, or no --config at all) the log
of every config is rotated and every distinct event queue is cleaned once.

Without flags both are done. Run it from cron, e.g. once a day.`

const logrotateExample = `  onecheck logrotate
  onecheck logrotate --events
  onecheck logrotate -c /etc/onecheck/web.yaml --logs

  # crontab
  17 3 * * *  onecheck logrotate`

func (a *app) logrotateCmd() *cobra.Command {
	var logs, events bool
	cmd := &cobra.Command{
		Use:     "logrotate",
		Short:   "Rotate logs and remove expired events",
		Long:    logrotateLong,
		Example: logrotateExample,
		Args:    cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if !logs && !events {
				logs, events = true, true
			}
			return a.logrotate(logs, events)
		},
	}
	cmd.Flags().BoolVar(&logs, "logs", false, "only rotate logs")
	cmd.Flags().BoolVar(&events, "events", false, "only remove expired events")
	return cmd
}

// logrotate rotates the log of every config and cleans each distinct event
// queue once. In a directory a broken config is reported and the rest is
// still done.
func (a *app) logrotate(logs, events bool) error {
	target, list, err := a.loadConfigs()
	if err != nil {
		return err
	}
	bad := 0
	fail := func(path string, err error) error {
		if err == nil || !target.Dir {
			return err
		}
		fmt.Fprintf(a.stderr, "onecheck: %s\n", configError(path, err))
		bad++
		return nil
	}
	cleaned := map[string]bool{}
	for _, l := range list {
		if l.err != nil {
			bad++
			fmt.Fprintf(a.stderr, "onecheck: %s\n", configError(l.path, l.err))
			continue
		}
		if logs {
			if err := fail(l.path, a.rotateLogs(l.cfg)); err != nil {
				return err
			}
		}
		if events && !cleaned[l.cfg.EventsDir()] {
			cleaned[l.cfg.EventsDir()] = true
			if err := fail(l.path, a.cleanEvents(l.cfg)); err != nil {
				return err
			}
		}
	}
	if bad > 0 {
		return fmt.Errorf("%d of %d configs in %s could not be handled", bad, len(list), target.Path)
	}
	return nil
}

func (a *app) cleanEvents(cfg *config.Config) error {
	removed, err := rotate.Events(cfg.EventsDir(), time.Duration(cfg.Events.TTL), time.Now())
	if err != nil {
		return fmt.Errorf("cannot clean events: %w", err)
	}
	fmt.Fprintf(a.stdout, "events: removed %d expired from %s\n", removed, cfg.EventsDir())
	return nil
}

// rotateLogs rotates under the run lock, so that a log is never compressed
// while a run is still writing to it.
func (a *app) rotateLogs(cfg *config.Config) error {
	st, err := store.Open(cfg)
	if err != nil {
		return err
	}
	unlock, err := st.Lock()
	if errors.Is(err, store.ErrLocked) {
		fmt.Fprintln(a.stdout, "logs: skipped, a run is in progress")
		return nil
	}
	if err != nil {
		return err
	}
	defer unlock()
	rotated, err := rotate.Logs(cfg.LogFile(), cfg.Log.Rotate, time.Now())
	if err != nil {
		return err
	}
	if rotated {
		fmt.Fprintf(a.stdout, "logs: rotated %s\n", cfg.LogFile())
	} else {
		fmt.Fprintf(a.stdout, "logs: nothing to rotate (%s)\n", cfg.LogFile())
	}
	return nil
}

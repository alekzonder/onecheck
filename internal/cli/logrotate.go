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
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}
			if !logs && !events {
				logs, events = true, true
			}
			if logs {
				if err := a.rotateLogs(cfg); err != nil {
					return err
				}
			}
			if events {
				removed, err := rotate.Events(cfg.EventsDir(), time.Duration(cfg.Events.TTL), time.Now())
				if err != nil {
					return fmt.Errorf("cannot clean events: %w", err)
				}
				fmt.Fprintf(a.stdout, "events: removed %d expired from %s\n", removed, cfg.EventsDir())
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&logs, "logs", false, "only rotate logs")
	cmd.Flags().BoolVar(&events, "events", false, "only remove expired events")
	return cmd
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

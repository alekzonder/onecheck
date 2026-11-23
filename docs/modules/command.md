# Module `command`

Runs a command and returns its exit code and output.

## Params

Exactly one of `run` and `argv` is required.

| Key | Description |
|---|---|
| `run` | a command line, executed with `sh -c`; pipes, redirects and variables work |
| `argv` | a list: program and its arguments, executed directly, without a shell |
| `env` | extra environment variables, a mapping of name to value |
| `cwd` | working directory |
| `stdin` | text passed to the command on standard input |

The command inherits onecheck's environment plus `env`. It is killed,
together with everything it started, when the rule's `timeout` expires.

Prefer `argv` when arguments come from elsewhere: nothing is interpreted by a
shell. In `run`, shell variables such as `$HOME`, `$1`, `$$` and `$(...)`
reach the shell untouched; only the `${VAR}` form is expanded by onecheck
from its own environment first (write `$${VAR}` to pass it to the shell
literally).

## Fields

| Field | Type | Description |
|---|---|---|
| `exit_code` | number | exit code of the command; `-1` if it was killed by a signal |
| `stdout` | string | standard output |
| `stderr` | string | standard error |
| `duration` | **duration** | how long the command ran |

A non-zero exit code is a value, not an error: check it with `exit_code`. The
rule ends with status `error` when the command could not be run at all
(program not found, not executable, bad `cwd`) or hit the timeout.

Note that a rule does **not** require exit code `0` unless you say so. Add
`exit_code: 0` to `check`.

The tail of `stdout` and `stderr` (the last `10k` of each by default) is saved
in the rule's result on every run, also when no condition mentions them and
when the command hit the timeout, so a failure can be investigated afterwards.
Change the size with `output_tail`, see
[output-and-results.md](../output-and-results.md#saved-output).

`stdout` and `stderr` are each limited by `run.max_field_size` (default
`10MB`).

Output usually ends with a newline. With `equals` include it (`"active\n"`);
with `matches` use `(?m)` so `$` matches before it: `'(?m)^active$'`.

## Examples

```yaml
rules:
  # exit code only
  - name: nginx-config-valid
    module: command
    params: { argv: [nginx, -t] }
    check:
      exit_code: 0

  # exit code and output
  - name: nginx-active
    module: command
    params: { argv: [systemctl, is-active, nginx] }
    check:
      exit_code: 0
      stdout: { matches: '(?m)^active$' }

  # a shell pipeline producing a number
  - name: not-too-many-processes
    module: command
    params: { run: "ps -e --no-headers | wc -l" }
    check:
      exit_code: 0
      stdout: { matches: '^\s*\d{1,3}\s*$' }      # fewer than 1000

  # JSON output
  - name: queue-short
    module: command
    timeout: 5s
    params: { run: "rabbitmqctl list_queues --formatter json" }
    check:
      exit_code: 0
      stdout:
        json:
          assert:
            - { path: '#(messages>1000)#', length: 0 }

  # nothing alarming on stderr, some output on stdout
  - name: backup-dry-run
    module: command
    params:
      run: "./backup.sh --dry-run"
      cwd: /opt/backup
      env: { BACKUP_TARGET: s3://bucket/prefix }
    check:
      exit_code: { in: [0, 2] }
      stdout: { length: { gt: 0 } }
      stderr: { not_matches: '(?i)error|denied' }
```

# Getting started

## Install

onecheck is one static binary plus one executable per check module.

### With the install script

```sh
curl -fsSL https://raw.githubusercontent.com/alekzonder/onecheck/main/install.sh | sh
```

The script detects the system and CPU, asks GitHub for the latest release,
verifies the download against the release's `checksums.txt`, and installs
onecheck together with the bundled modules (`file`, `http`, `command`, `disk`, `systemd`, `cpu`, `memory`, `docker`) of the
same version.

A specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/alekzonder/onecheck/main/install.sh | sh -s -- --version 0.1.0
```

Every version gets its own directory, and a symlink in a bin directory points
at the chosen one:

| | Linux | macOS |
|---|---|---|
| versions | `/usr/local/lib/onecheck/versions/<version>/onecheck` | `~/.local/share/onecheck/versions/<version>/onecheck` |
| symlink | `/usr/local/bin/onecheck` | `~/.local/bin/onecheck` |
| modules | `/usr/lib/onecheck/modules` | `~/.local/lib/onecheck/modules` |

On Linux the install is system-wide, and the script calls `sudo` when it is
not run as root. On macOS everything stays in the home directory; if
`~/.local/bin` is not in `PATH`, the script says how to add it.

| Option | Description |
|---|---|
| `--version <v>` | version to install (`0.1.0` or `v0.1.0`); default: the latest release. Also `ONECHECK_VERSION` |
| `--root <dir>` | where versions are kept |
| `--bin-dir <dir>` | where the `onecheck` symlink is created |
| `--modules-dir <dir>` | where the bundled modules go |
| `--no-modules` | install onecheck only |

Set `GITHUB_TOKEN` if GitHub's rate limit gets in the way.

To upgrade, downgrade or switch back, run the script again. A version that is
already on disk is not downloaded again, only the symlink is switched, and the
modules are brought to the same version. Old versions are kept until you
delete their directories under `versions/`. `onecheck self-update` refuses to
touch a binary installed this way, because it would overwrite one version's
directory with another version.

The modules directories above are the defaults of `paths.modules`: the Linux
one for root, the macOS one for a regular user. If you run onecheck on Linux
as a regular user, set `paths.modules: /usr/lib/onecheck/modules` in the
config, or install with `--modules-dir ~/.local/lib/onecheck/modules`.

### By hand

1. Download `onecheck_<os>_<arch>` from the
   [releases](https://github.com/alekzonder/onecheck/releases), verify it
   against `checksums.txt`, and put it into `PATH` as `onecheck`:

   ```sh
   install -m 0755 onecheck_linux_amd64 /usr/local/bin/onecheck
   onecheck version
   ```

2. Install the bundled modules (`file`, `http`, `command`, `disk`, `systemd`, `cpu`, `memory`, `docker`):

   ```sh
   onecheck module install alekzonder/onecheck --modules-dir /usr/lib/onecheck/modules
   onecheck module list --modules-dir /usr/lib/onecheck/modules
   ```

   Once a config exists, `--modules-dir` is not needed: the directory is
   taken from `paths.modules`.

Or build everything from source: `make build` puts the binary into
`dist/onecheck` and the modules into `dist/modules/`.

## First config

onecheck looks for its config in this order:

1. `--config <path>` (`-c`)
2. `$ONECHECK_CONFIG`
3. `/etc/onecheck/config.yaml`
4. `$HOME/.config/onecheck/config.yaml`

A minimal config needs a name and some rules:

```yaml
name: main

rules:
  - name: root-disk-marker
    module: file
    params:
      path: /etc/hostname
    check:
      exists: true
      size: { gt: 0 }

  - name: site-up
    module: http
    params:
      url: https://example.com/
    check:
      status: 200
```

Each rule has three parts:

- `module` — which module collects the data;
- `params` — the module's input (what to look at);
- `check` — conditions on the values the module returns.

See [examples/config.yaml](../examples/config.yaml) for a fuller example,
[configuration.md](configuration.md) for every key and [checks.md](checks.md)
for everything `check` can express.

## Validate and run

```sh
onecheck validate          # reports every config problem at once, runs nothing
onecheck paths             # where the config, log, state, results and events are
onecheck                   # run all rules; same as "onecheck run"
onecheck --only-failed     # print only what is wrong
onecheck -o json           # machine-readable output
onecheck --rule site-up    # run one rule
```

The exit code is `0` when every rule passed, `1` when something failed, `2`
when onecheck could not run at all. Details: [output-and-results.md](output-and-results.md).

## Schedule it

onecheck has no scheduler of its own. Run it from cron:

```cron
# /etc/cron.d/onecheck
* * * * *   root  onecheck --only-failed >/dev/null
17 3 * * *  root  onecheck logrotate >/dev/null
```

or from a systemd timer:

```ini
# /etc/systemd/system/onecheck.service
[Unit]
Description=onecheck run

[Service]
Type=oneshot
ExecStart=/usr/local/bin/onecheck --only-failed
# exit code 1 means "a check failed", which is not a unit failure
SuccessExitStatus=1
```

```ini
# /etc/systemd/system/onecheck.timer
[Unit]
Description=Run onecheck every minute

[Timer]
OnCalendar=minutely
AccuracySec=5s

[Install]
WantedBy=timers.target
```

### Checks at different intervals

Use one config per interval, each with its own `name`, and one schedule line
per config:

```cron
* * * * *    root  onecheck -c /etc/onecheck/fast.yaml --only-failed >/dev/null
*/15 * * * * root  onecheck -c /etc/onecheck/slow.yaml --only-failed >/dev/null
```

Configs with different names never block each other: results, state, logs and
the run lock are kept per name. If a run of the *same* config is still in
progress when the next one starts, the new one exits with code `2`.

## What gets written where

With the default paths, for root:

| What | Where |
|---|---|
| latest result of each rule | `/var/lib/onecheck/results/<name>/<rule>.json` |
| state between runs | `/var/lib/onecheck/state/<name>/` |
| events (pass ↔ fail changes) | `/var/lib/onecheck/events/` |
| log | `/var/log/onecheck/<name>/onecheck.log` |
| modules | `/usr/lib/onecheck/modules/` |

For other users everything lives under `$HOME/.local/state/onecheck/` and
`$HOME/.local/lib/onecheck/modules/`. All of it is configurable:
[configuration.md](configuration.md).

---
title: onecheck
description: Simple, scriptable health checks for a server.
sidebar:
  label: Introduction
---

Simple, scriptable health checks for a server. You describe checks in YAML,
cron (or a systemd timer) runs `onecheck`, and onecheck tells you what is
broken: by exit code, on stdout (text or JSON), in per-rule result files, and
through an event queue that records every pass → fail and fail → pass change.

```console
$ onecheck -c /etc/onecheck/config.yaml
PASS   passwd-sane   (file, 2ms)
FAIL   api-health    (http, 124ms)  [transition: fail]
       body json status: expected equals "ok", got "degraded"
PASS   nginx-active  (command, 9ms)

3 rules: 2 passed, 1 failed, 0 errors
$ echo $?
1
```

## How it works

- **Modules collect, onecheck decides.** A check type (`file`, `http`,
  `command`, `disk`, `systemd`, `cpu`, `memory`, `docker`, `ports`, ...) is a
  separate executable, `onecheck-<name>`, living in a modules directory. A
  module only returns values (status code, file size, stdout, ...). All
  comparisons are done by one engine inside onecheck, so every operator works
  the same for every module.
- **Modules are picked up dynamically.** onecheck looks into the modules
  directory on every run. Install a module and the next run can use it.
- **State is tracked for you.** For every rule onecheck remembers the
  confirmed state and how many runs in a row passed or failed. With
  `fail_after: 3` a rule must fail three times in a row before it is
  considered failed. Each confirmed change is written as a JSON event for a
  notification service to pick up.
- **Notifications are a separate tool.** [onenotify](https://alekzonder.github.io/onenotify)
  takes the events and sends them to Telegram, mail or ntfy.

## Quick start

```sh
# 1. install the latest onecheck and the bundled modules
curl -fsSL https://raw.githubusercontent.com/alekzonder/onecheck/main/install.sh | sh

# 2. write a config
mkdir -p /etc/onecheck && cat > /etc/onecheck/config.yaml <<'YAML'
name: main
rules:
  - name: site-up
    module: http
    params: { url: https://example.com/ }
    check:
      status: 200
      body: { contains: "Example Domain" }
YAML

# 3. check the config, then run it
onecheck validate
onecheck
```

[Getting started](/getting-started) walks through installation, the first
config and scheduling with cron or systemd.

## Documentation

| | |
|---|---|
| [Getting started](/getting-started) | install, first config, cron and systemd |
| [Configuration](/configuration) | every config key, includes, defaults |
| [Checks](/checks) | the `check` block: paths, all operators, JSON and JSON Schema |
| Modules | [file](/modules/file), [http](/modules/http), [command](/modules/command), [disk](/modules/disk), [systemd](/modules/systemd), [cpu](/modules/cpu), [memory](/modules/memory), [docker](/modules/docker), [ports](/modules/ports) |
| [State and events](/state-and-events) | `fail_after` / `pass_after`, the event queue contract |
| [Output and results](/output-and-results) | exit codes, text and JSON output, result files |
| [Module management](/module-management) | search, install, update, remove, self-update |
| [Logging and logrotate](/logging-and-logrotate) | log files, levels, rotation, event cleanup |
| [Module protocol](/module-protocol) | write your own module, in any language |
| [Development](/development) | build, test, release |

`onecheck --help` and `onecheck <command> --help` describe every command and
flag. The source is on [GitHub](https://github.com/alekzonder/onecheck).

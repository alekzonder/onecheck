# Module `file`

Inspects one path: a file, a directory or a symlink.

## Params

| Key | Required | Description |
|---|---|---|
| `path` | yes | the path to inspect |

Glob patterns are not supported: one rule looks at one path.

## Fields

| Field | Type | Description |
|---|---|---|
| `exists` | bool | whether the path exists |
| `type` | string | `file`, `dir`, `symlink` or `other` (socket, device, ...) |
| `size` | **bytes** | file size; for a directory the total size of all files below it |
| `age` | **duration** | time since the last modification |
| `mode` | string | permission bits as four octal digits, e.g. `"0640"` |
| `owner` | string | owner user name (the numeric id if it has no name) |
| `group` | string | owner group name (the numeric id if it has no name) |
| `content` | string | content of a regular file |
| `count` | number | number of entries directly in a directory |
| `target` | string | where a symlink points, exactly as stored in the link |

**bytes** and **duration** fields accept unit literals in conditions:
`size: { lt: 10MB }`, `age: { lt: 1h }`. See [checks.md](../checks.md#gt-gte-lt-lte).

Which fields are present:

| The path is | Fields returned |
|---|---|
| missing | `exists: false` only |
| a regular file | everything except `count` and `target` |
| a directory | everything except `content` and `target` |
| a symlink to something that exists | `type: symlink`, `target`, and all other fields describing **what the link points to** |
| a dangling symlink | `exists: false`, `type: symlink`, `target` |

A condition on a field that is not returned fails with "path is missing", so
`size: { gt: 0 }` on a missing file fails as it should.

Only the fields a rule uses are collected. In particular `content` is read,
and a directory's total `size` is computed, only when `check` mentions them.

`content` is limited by `run.max_field_size` (default `10MB`). A condition on
a larger file ends the rule with status `error`; check `size` or raise the
limit.

A path that cannot be inspected at all (permission denied on a parent
directory, unreadable file when `content` is requested) is an `error`, not a
`fail`.

## Examples

```yaml
rules:
  # exists, right owner and permissions
  - name: sshd-config
    module: file
    params: { path: /etc/ssh/sshd_config }
    check:
      exists: true
      type: file
      owner: root
      mode: "0600"

  # must not exist
  - name: no-maintenance-flag
    module: file
    params: { path: /var/www/maintenance.flag }
    check:
      exists: false

  # a backup was written during the last day and is not suspiciously small
  - name: backup-fresh
    module: file
    params: { path: /backup/latest.tar.gz }
    check:
      exists: true
      age: { lt: 25h }
      size: { gt: 100MB }

  # content: substring, regular expression
  - name: nginx-listens-443
    module: file
    params: { path: /etc/nginx/nginx.conf }
    check:
      content:
        contains: "listen 443"
        not_matches: '(?m)^\s*server_tokens\s+on'

  # content as JSON
  - name: app-config
    module: file
    params: { path: /etc/app/config.json }
    check:
      content:
        json:
          assert:
            - { path: db.host, equals: localhost }
            - { path: features, type: array }

  # directory: total size and number of entries
  - name: spool-not-growing
    module: file
    params: { path: /var/spool/app }
    check:
      type: dir
      size: { lt: 1GB }
      count: { lt: 500 }

  # a symlink points where it should
  - name: current-release
    module: file
    params: { path: /srv/app/current }
    check:
      exists: true                       # the target exists
      type: symlink
      target: { starts_with: /srv/app/releases/ }
```

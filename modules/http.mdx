---
title: http module
sidebar:
  label: http
---

Makes one HTTP or HTTPS request.

## Params

| Key | Required | Default | Description |
|---|---|---|---|
| `url` | yes | | `http://` or `https://` URL |
| `method` | no | `GET` | HTTP method |
| `headers` | no | | request headers, a mapping of name to value |
| `body` | no | | request body |
| `follow_redirects` | no | `true` | follow redirects; with `false` the redirect response itself is returned |
| `insecure_skip_verify` | no | `false` | do not verify the server's TLS certificate |

The request is limited by the rule's `timeout`. Proxy settings are taken from
the usual `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY` environment variables.

Secrets can be passed through the environment, see
[Configuration](/configuration#environment-variables-in-params):

```yaml
headers:
  Authorization: "Bearer ${API_TOKEN}"
```

## Fields

| Field | Type | Description |
|---|---|---|
| `status` | number | HTTP status code |
| `headers` | object | response headers; names in **lower case**, repeated headers joined with `, ` |
| `body` | string | response body |
| `duration` | **duration** | time from sending the request until the whole body was read |
| `tls_expires_in` | **duration** | time until the server certificate expires; https only |

**duration** fields accept unit literals: `duration: { lt: 500ms }`,
`tls_expires_in: { gt: 14d }`.

Address a single header as `headers.<name>`, always in lower case:
`headers.content-type`.

A response with any status code is a successful collection: `404` and `500`
are values of `status` to be checked. The rule ends with status `error`, not
`fail`, when no response could be obtained: DNS failure, connection refused,
TLS handshake or certificate error, timeout.

`body` is limited by `run.max_field_size` (default `10MB`); a condition on a
larger body ends the rule with status `error`.

The tail of `body` (the last `10k` by default) and the `headers` are saved in
the rule's result on every run, also when no condition mentions them, so a
failed `status: 200` can be investigated afterwards. Change the size with
`output_tail`, or set `output_tail: 0` for responses that hold secrets, see
[Output and results](/output-and-results#saved-output).

`tls_expires_in` is absent for plain `http://` URLs, so a condition on it
fails there.

## Examples

```yaml
rules:
  # status and substring
  - name: site-up
    module: http
    timeout: 10s
    params: { url: https://example.com/ }
    check:
      status: 200
      body: { contains: "Welcome" }

  # several acceptable statuses, headers, response time
  - name: api-root
    module: http
    params: { url: https://api.example.com/ }
    check:
      status: { in: [200, 204] }
      headers.content-type: { starts_with: application/json }
      headers.strict-transport-security: { exists: true }
      headers.x-powered-by: { exists: false }
      duration: { lt: 1s }

  # JSON body: values, presence of keys, array contents
  - name: api-health
    module: http
    params:
      url: https://api.example.com/health
      headers: { Authorization: "Bearer ${API_TOKEN}" }
    check:
      status: 200
      body:
        json:
          assert:
            - { path: status, equals: ok }
            - { path: version, exists: true }
            - { path: checks.#, gte: 1 }
            - { path: 'checks.#(state!="ok")#', length: 0 }

  # JSON body validated with a JSON Schema
  - name: api-contract
    module: http
    params: { url: https://api.example.com/v1/users/1 }
    check:
      body:
        json:
          schema:
            type: object
            required: [id, email, roles]
            properties:
              id: { type: integer }
              email: { type: string, pattern: "@" }
              roles: { type: array, items: { type: string } }

  # POST with a body
  - name: login-works
    module: http
    params:
      url: https://example.com/api/login
      method: POST
      headers: { Content-Type: application/json }
      body: '{"user":"probe","password":"${PROBE_PASSWORD}"}'
    check:
      status: 200
      body: { json: { assert: [ { path: token, type: string } ] } }

  # a redirect is in place
  - name: http-redirects-to-https
    module: http
    params:
      url: http://example.com/
      follow_redirects: false
    check:
      status: { in: [301, 308] }
      headers.location: { starts_with: "https://" }

  # certificate lifetime
  - name: certificate
    module: http
    params: { url: https://example.com/ }
    check:
      tls_expires_in: { gt: 14d }
```

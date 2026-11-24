# ports module — design

## Intent

Know which ports the host exposes: a port that is not allowed but open is a
failure, and so is an allowed port that is closed. The check must be fast,
run on the host itself and not depend on which firewall is installed.

## Approaches

1. **Scan from outside** — the only full truth, rejected by the user: it is
   a port scan, slow, and needs another machine.
2. **Read firewall rules** (ufw files, nftables ruleset, Docker API) — many
   sources, an emulation of netfilter, silent mistakes. Rejected.
3. **Probe through a temporary network namespace** — the kernel gives the
   verdict, but monitoring would create interfaces on a production host
   every run and needs root. Rejected.
4. **Listening sockets from the kernel** (chosen by the user) — one source
   for the whole system, by protocol and address. A port cannot be reached
   if nothing listens on an outside address. Docker's published ports are
   listening sockets of `docker-proxy`, so they are covered. The invariant
   is "only allowed ports listen on outside addresses, and every required
   one does". It is stricter than a firewall check: a port that only the
   firewall hides is reported as listening.

## Goals

- Bundled module `ports`: listening TCP sockets and unconnected UDP sockets
  from `/proc/net/{tcp,tcp6,udp,udp6}`. No root, no external commands.
- One output shape: `listening` (array) and `count`. Params only narrow.
- Checks are written with the existing `every` / `some` / `path`.

## Non-goals

- Firewall verdicts (ufw, nftables), cloud firewalls.
- macOS: the module needs `/proc`; elsewhere it is an error that says so.
- Sockets bound to a device (`SO_BINDTODEVICE`) are reported by address
  only; raw sockets and SCTP are not reported.

## Anti-goals

- An unreadable `/proc/net/tcp` must never look like "nothing listens".
- A typo in `interface` must not silently pass: unknown interface is an error.

## Behaviour

Params: `proto` (`tcp` | `udp`), `interface` (name).

- Without `interface`: every socket except those bound to a loopback
  address (`127.0.0.0/8`, `::1`).
- With `interface`: sockets bound to an address of that interface, plus
  sockets bound to all addresses (`0.0.0.0`, `::`). `interface: lo` gives
  what is reachable on loopback.

Element keys, always all present:

| Key | Value |
|---|---|
| `proto` | `tcp`, `udp` |
| `port` | number |
| `address` | `0.0.0.0`, `::`, or the bound address; IPv4-mapped IPv6 is shown as IPv4 |
| `family` | `ipv4`, `ipv6` |
| `interface` | interface that has the address; `any` for all addresses; `""` when no interface has it |
| `process` | name of the lowest pid holding the socket; `""` when it cannot be seen (not root, or a kernel socket) |

Sockets with the same proto, address and port (`SO_REUSEPORT`) are one
element. Sorted by proto, port, address. TCP: state LISTEN. UDP: no remote
address. `tcp` must be readable; a missing IPv6 file means IPv6 is off.
Process names are looked up only when `listening` is requested.

## Left to the builder

File layout, parsing helpers, test fixtures.

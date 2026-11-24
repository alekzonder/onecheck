# Module `ports`

Reports the TCP and UDP ports the host listens on, so you know that nothing
unexpected is exposed and that every port which must serve is open.
Linux only: the data comes from the kernel (`/proc/net`), the same that
`ss -tuln` shows. No root is needed and no external command is run.

A port cannot be reached from outside when nothing listens on an outside
address, whatever the firewall says. Ports published by Docker are
listening sockets too (of `docker-proxy`), so they are in the list even
though Docker opens them past `ufw`.

The module always returns the same fields: the list of listening sockets
that match the params, and their number. Params only narrow the list.

## Params

| Key | Required | Description |
|---|---|---|
| `proto` | no | `tcp` or `udp`; both when omitted |
| `interface` | no | only sockets reachable on this interface, e.g. `eth0` |

Without `interface` the list has every socket except those bound to a
loopback address (`127.0.0.0/8`, `::1`): what other machines could reach.

With `interface` the list has the sockets bound to an address of that
interface and the sockets bound to all addresses (`0.0.0.0`, `::`), which
answer on every interface. `interface: lo` lists what is reachable locally.
An interface that does not exist is an error.

## Fields

| Field | Type | Description |
|---|---|---|
| `listening` | array | listening sockets sorted by proto, port, address |
| `count` | number | number of listening sockets |

Each element of `listening` has all of these keys:

| Key | Type | Value |
|---|---|---|
| `proto` | string | `tcp`, `udp` |
| `port` | number | port number |
| `address` | string | `0.0.0.0` or `::` for all addresses, otherwise the address the socket is bound to |
| `family` | string | `ipv4`, `ipv6` |
| `interface` | string | interface that has the address; `any` for all addresses; `""` when no interface has it |
| `process` | string | name of the process that holds the socket; `""` when onecheck may not see it |

TCP sockets are those in the LISTEN state. UDP has no such state: the list
has the UDP sockets without a remote address, the ones `ss -uln` shows.

A service listening on IPv4 and IPv6 gives two elements (`0.0.0.0` and
`::`). Several sockets on the same proto, address and port (worker
processes) are one element.

`process` is filled for the processes onecheck may inspect: all of them as
root, only its own user's otherwise. A rule that only checks `count` does
not look processes up.

## Recipes

### No unknown ports

Every listening socket must be an allowed one. Anything else is a failure
that names the port, the address and the process.

```yaml
- name: no-unknown-ports
  module: ports
  check:
    listening:
      every:
        any:
          - { path: port, in: [22, 80, 443] }
          - { path: port, gte: 60001, lte: 60010 }        # mosh
```

To allow a port only for one protocol or on one interface, put the
conditions of one alternative into `all`:

```yaml
          - all:
              - { path: port, equals: 51820 }
              - { path: proto, equals: udp }
          - all:
              - { path: port, in: [8200, 8201] }
              - { path: interface, equals: wg0 }          # only on the VPN
```

### A port must be open

```yaml
- name: web-ports
  module: ports
  params: { proto: tcp }
  check:
    listening.#.port: { contains: 443 }
```

`listening.#.port` is the list of port numbers. Write one rule per protocol,
or select the protocol in the path:
`listening.#(proto=="udp")#.port: { contains: 51820 }`.

### Both in one rule

```yaml
- name: tcp-ports
  module: ports
  params: { proto: tcp }
  check:
    listening:
      every: { path: port, in: [22, 80, 443] }
    listening.#.port:
      all:
        - { contains: 80 }
        - { contains: 443 }
```

### One interface

```yaml
- name: public-interface
  module: ports
  params: { interface: eth0, proto: tcp }
  check:
    listening:
      every: { path: port, in: [22, 443] }
```

### A local service is up

```yaml
- name: postgres-listens
  module: ports
  params: { interface: lo, proto: tcp }
  check:
    listening.#.port: { contains: 5432 }
```

## Limits

- **The firewall is not consulted.** A port that listens on `0.0.0.0` and
  is hidden only by the firewall is in the list: allow it in the rule or
  bind the service to `127.0.0.1`. A port that listens but is blocked by the
  firewall still counts as open. A firewall of the cloud provider is not
  visible from the host at all.
- **UDP is noisier than TCP.** Programs open unconnected UDP sockets on
  random ports for a short time (DNS clients, DHCP, mosh sessions). When a
  "no unknown ports" rule fails now and then on UDP, check only TCP
  (`proto: tcp`) or allow the port range.
- A socket bound to a device rather than an address (`SO_BINDTODEVICE`) is
  shown with the address it is bound to, usually `0.0.0.0`.
- Port forwarding done only in the firewall (DNAT without a listening
  socket, Docker with `userland-proxy: false`) is not seen.

## Output

```
FAIL   no-unknown-ports  (ports, 20ms)  [transition: fail]
       listening.3: expected any [{"in":[22,80,443],"path":"port"},{"gte":60001,"lte":60010,"path":"port"}], got {"address":"0.0.0.0","family":"ipv4","interface":"any","port":8080,"process":"docker-proxy","proto":"tcp"}
FAIL   web-ports         (ports, 19ms)  [transition: fail]
       listening.#.port: expected contains 443, got [22,22,80]
```

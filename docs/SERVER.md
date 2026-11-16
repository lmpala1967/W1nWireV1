# W1nWire server reference

The server is a single static Go binary (`w1nwire-server`) that opens a UDP
socket, terminates the W1nWire handshake, and puts the client traffic on a TUN
device which it masquerades onto the internet.

This document is the reference: configuration schema, CLI, systemd behaviour,
metrics, logs, peer handling, NAT and performance. For the installation
walkthrough see [`deploy/README.md`](../deploy/README.md); for the protocol
itself see [`PROTOCOL.md`](PROTOCOL.md) and [`ARCHITECTURE.md`](ARCHITECTURE.md).

---

## 1. Paths

| Path | Purpose |
| ---- | ------- |
| `/usr/local/bin/w1nwire-server` | the binary |
| `/etc/w1nwire/server.json` | configuration, 0600 |
| `/etc/w1nwire/server.key` | server key pair (`private_key`, `public_key`), 0600 |
| `/etc/w1nwire/invites/` | invite codes created by the installer, 0600 |
| `/etc/systemd/system/w1nwire-server.service` | the unit |
| `/etc/sysctl.d/99-w1nwire.conf` | `net.ipv4.ip_forward = 1` |
| `/etc/iptables/rules.v4` | persisted NAT/forward rules |

---

## 2. Command line

```
w1nwire-server -config <file> [-log-level debug|info|warn|error]
w1nwire-server keygen [-out <file>]
w1nwire-server peer add -config <file> -name <name> -host <host[:port]>
w1nwire-server peer remove -id <name> [-config <file>]
w1nwire-server version
w1nwire-server help
```

### Run the server

```bash
w1nwire-server -config /etc/w1nwire/server.json
```

* `-config` defaults to `server.json` in the working directory; the unit always
  passes the absolute path.
* `-log-level` defaults to `info`. `debug` logs every rejected packet and every
  frame; it is the right level when a client cannot connect, and the wrong
  level for a busy server.
* The process runs in the foreground and logs to stderr. There is no daemon
  mode; systemd is the supervisor.
* Exit code is 1 for an unreadable or invalid configuration, an unusable key,
  a UDP port that cannot be bound, or a TUN device that cannot be created.

### `keygen`

```bash
w1nwire-server keygen -out /etc/w1nwire/server.key
```

Writes:

```json
{
  "private_key": "<base64, 32 bytes>",
  "public_key": "<base64, 32 bytes>"
}
```

The file is created 0600. Without `-out` the pair is printed to stdout in
`private_key = ...` / `public_key  = ...` form.

Keys are X25519. Changing the server key invalidates every invite code that was
generated before.

### `peer add`

```bash
w1nwire-server peer add -config /etc/w1nwire/server.json -name laptop \
  -host 203.0.113.10:51820
```

Generates a client key pair, appends a peer to the `peers` array with the next
free tunnel address (starting at `10.66.0.2`), rewrites the configuration and
prints exactly one line to stdout:

```
ww1.eyJ...
```

That line is the invite code. It is the only time it exists — the server keeps
only the client's public key — so save it or hand it to the device
immediately.

`-host` is the endpoint written into the invite code: the address the app will
dial. It is **required** in current builds; `peer add` refuses to generate an
invite that points at `0.0.0.0`, because such a code can never work. Use the
public IP or a DNS name that resolves to it. A bare host is accepted too and the
listening port is appended; with a port, the value is used verbatim.

`peer add` edits the file only. The running server keeps its peer list in
memory, so restart the unit afterwards:

```bash
systemctl restart w1nwire-server
```

### `peer remove`

```bash
w1nwire-server peer remove -id laptop
```

Removes the peer from `peers` (by `id`), so its key no longer authenticates.
`-config` defaults to the standard configuration path; pass it explicitly when
the file is somewhere else. Like `peer add`, it only edits the JSON file — the
change takes effect when the service is restarted. A removed device cannot get
back in with the invite code it had; issue a new one with `peer add`.

### `version`

```bash
w1nwire-server version
# w1nwire-server v0.1.0
```

The version is injected at build time (`-ldflags "-X main.version=..."`);
a binary built by hand reports `dev` (or `source` when built by
`install.sh --from-source`).

---

## 3. Invite code format

```
ww1.<base64url without padding of a compact JSON object>
```

| Key | Type | Meaning |
| --- | ---- | ------- |
| `v` | number | format version, currently `1` |
| `n` | string | peer name, for display |
| `h` | string | server endpoint, `host:port` |
| `k` | string | server public key, base64 |
| `p` | string | **client private key**, base64 |
| `a` | string | client tunnel address, e.g. `10.66.0.2` |
| `g` | string | server tunnel address, `10.66.0.1` |

The code is self-contained: the app needs nothing else to connect. It is also
the client's private key, so it is a credential, not a link. Anyone who has it
can use the tunnel as that peer.

The format is stable; new fields may be added in later versions and old clients
must ignore keys they do not know.

---

## 4. Configuration schema

`/etc/w1nwire/server.json` is decoded with `DisallowUnknownFields`: an unknown
key is a hard error, not a warning. This is deliberate — a typo must not
silently disable NAT.

The file is validated before the server starts. Validation errors mention the
field, for example:

```
config: at least one peer is required
config: invalid tun.address "10.66.0.1": invalid CIDR address
config: peers[1].id is required
config: duplicate peer id "laptop"
```

### Top level

| Field | Type | Default | Notes |
| ----- | ---- | ------- | ----- |
| `listen` | string | required | UDP `host:port`. `0.0.0.0:51820` is what the installer writes. |
| `server_private_key` | string | required | base64 private key from `keygen`. |
| `tun` | object | required | see below |
| `nat` | object | — | see below |
| `http` | object | — | metrics |
| `peers` | array | required, at least 1 | see below |

### `tun`

| Field | Type | Default | Notes |
| ----- | ---- | ------- | ----- |
| `enabled` | bool | `false` | `false` serves only the stream/proxy mode, no IP forwarding. |
| `name` | string | `""` | Interface name. Linux allows up to 15 characters; the installer uses `w1nwire0`. |
| `address` | string | required when enabled | Server side of the tunnel in CIDR form, `10.66.0.1/24`. |
| `mtu` | number | `0` | Applied with `ip link set ... mtu` when greater than zero. 1420 leaves room for the UDP/IP overhead of a 1500-byte path. |

### `nat`

| Field | Type | Default | Notes |
| ----- | ---- | ------- | ----- |
| `enabled` | bool | `false` | When true the server installs the masquerade and forward rules itself at startup. |
| `outbound_interface` | string | `""` | Empty means "ask the default route". Set it when the default route changes or when the instance has several NICs. |

With `nat.enabled: true` the server runs `sysctl -w net.ipv4.ip_forward=1`,
`iptables -t nat ... MASQUERADE` and two `FORWARD` rules at startup. It needs
the privileges for that; the shipped unit grants them. If the rules cannot be
installed (container, missing `iptables`, read-only `/proc/sys`) the server logs
a warning and keeps serving — the tunnel is up, but nothing reaches the
internet. The installer installs the same rules independently, so a failed
`nat` step at startup is not fatal.

### `http.metrics`

| Field | Type | Default | Notes |
| ----- | ---- | ------- | ----- |
| `enabled` | bool | `false` | Exposes `/metrics` and `/healthz`. |
| `host` | string | `""` | **Empty means every interface.** Always set `127.0.0.1` unless a firewall protects the port. |
| `port` | number | `0` | `9187` on the server, `9188` on the client. |

### `peers[]`

| Field | Type | Default | Notes |
| ----- | ---- | ------- | ----- |
| `id` | string | required, unique | Used in logs and metric labels. |
| `public_key` | string | required | The client's static public key. |
| `tunnel_address` | string | required | Plain IPv4 (`10.66.0.2`), must be unique. |
| `allowed_ips` | array of strings | `[]` | See §7. |

### Example

The configuration the installer writes:

```json
{
  "listen": "0.0.0.0:51820",
  "server_private_key": "<base64>",
  "tun": {
    "enabled": true,
    "name": "w1nwire0",
    "address": "10.66.0.1/24",
    "mtu": 1420
  },
  "nat": {
    "enabled": true,
    "outbound_interface": ""
  },
  "http": {
    "metrics": {
      "enabled": true,
      "host": "127.0.0.1",
      "port": 9187
    }
  },
  "peers": [
    {
      "id": "desktop",
      "public_key": "<base64>",
      "tunnel_address": "10.66.0.2",
      "allowed_ips": ["10.66.0.0/24", "10.66.0.2/32"]
    }
  ]
}
```

---

## 5. systemd

The unit is `/etc/systemd/system/w1nwire-server.service`
(source: [`deploy/systemd/w1nwire-server.service`](../deploy/systemd/w1nwire-server.service)).

```bash
systemctl status w1nwire-server
systemctl restart w1nwire-server        # after editing server.json
journalctl -u w1nwire-server -f
```

Behaviour worth knowing:

* `Restart=always`, `RestartSec=5s`: the process is restarted when it exits for
  any reason. `SIGTERM` (what `systemctl stop` sends) is handled: the UDP
  socket and the TUN device are closed and the process exits 0.
* `ConditionPathExists=/etc/w1nwire/server.json`: without a configuration the
  unit is skipped instead of entering a restart loop.
* `StartLimitIntervalSec=300` / `StartLimitBurst=5`: after five failed starts
  in five minutes systemd gives up. Clear that state with
  `systemctl reset-failed w1nwire-server`.
* **It runs as root on purpose.** The daemon opens `/dev/net/tun` and executes
  `ip`, `iptables` and `sysctl`. An unprivileged user would need an udev rule
  for `/dev/net/tun`, access to `/run/xtables.lock` and write access to
  `/proc/sys/net/ipv4/ip_forward`. Instead the unit hardens the process:
  `NoNewPrivileges`, `ProtectSystem=full`, `ProtectHome`, `PrivateTmp`,
  `ProtectKernelModules`, `ProtectControlGroups`, `RestrictNamespaces`,
  `RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX AF_NETLINK` and a
  capability bounding set limited to `CAP_NET_ADMIN CAP_NET_RAW`.
* `ProtectKernelTunables` is deliberately **not** set: the daemon writes
  `net.ipv4.ip_forward` at startup.
* There is no `ExecReload`: the server does not handle `SIGHUP`. Restart it.

Running it without systemd is possible (`w1nwire-server -config ...` as root),
but then nothing restarts it and nothing applies the firewall rules at boot.

---

## 6. Metrics

When `http.metrics.enabled` is true the server serves:

* `http://127.0.0.1:9187/metrics` — Prometheus text format
* `http://127.0.0.1:9187/healthz` — `ok`

| Metric | Labels | Meaning |
| ------ | ------ | ------- |
| `w1nwire_up` | — | 1 while the process is serving |
| `w1nwire_handshakes_total` | `result` | Handshakes by outcome (`success`, `failed`) |
| `w1nwire_handshake_duration_seconds` | — | Histogram of successful handshakes |
| `w1nwire_sessions_active` | — | Established sessions |
| `w1nwire_peers_total` | `state` | Peers by state (`handshaked`, `offline`, `initiating`) |
| `w1nwire_last_handshake_seconds` | `peer` | Unix timestamp of the last successful handshake (use `time() - w1nwire_last_handshake_seconds` for the age) |
| `w1nwire_bytes_total` | `direction` | Bytes through the tunnel (`rx`, `tx`) |
| `w1nwire_packets_total` | `direction`, `kind` | Packets through the tunnel (`data`, `keepalive`) |
| `w1nwire_replay_dropped_total` | — | Packets dropped as replays |
| `w1nwire_decrypt_errors_total` | — | Packets that failed authentication |
| `w1nwire_client_connected` | `peer` | 1 while the client has a session |

The names are a contract with `monitoring/` (dashboard and alert rules). The
server's registry is per process, so `w1nwire_up` reads 1 as soon as the
endpoint answers.

The endpoint has no authentication. Keep it on `127.0.0.1`, or put a reverse
proxy with auth in front of it.

---

## 7. Peers and allowed IPs

A peer is one device: one static key pair, one tunnel address, one entry in
`peers`.

**Handshake.** The server identifies a client by trying the configured static
public keys against the handshake. There is no peer index in the first message,
so the cost of a handshake grows linearly with the number of peers (one X25519
operation per candidate). The stateless cookie is verified *before* that loop,
so an attacker with a spoofed source address cannot make the server pay it.

**Return traffic.** Packets read from the TUN device are matched to a peer by
their source address (`tunnel_address`). Two peers must therefore never share
an address, and the address must be inside the tunnel subnet — otherwise
return traffic is silently dropped.

**`allowed_ips`** is the intended source filter for a peer: the prefixes it may
send. It is part of the configuration schema and of the in-memory peer object,
but **the current server does not enforce it**. Treat it as documentation for
now: set it to `["10.66.0.0/24"]` (or the peer's `/32`) so the intent is
recorded, and do not rely on it to contain a malicious client. The tunnel
address is what actually identifies the peer.

**A client behind NAT** is fine: the server learns the client's address from the
handshake and updates it whenever an authenticated packet arrives from a new
source address. That also makes network switching (Wi-Fi to LTE) survivable
without a new invite code.

---

## 8. Running behind NAT

### Clients behind NAT

Nothing to do. The client sends the first packet; the server replies to the
address it observed, and follows the client if that address changes.

### The server behind NAT

The server can sit behind a router or a cloud NAT gateway, as long as:

1. the router forwards UDP `<port>` to the server's LAN address, and
2. the clients use the **public** address of the router in the invite code,
   with the forwarded port. `W1NWIRE_PUBLIC_HOST` and `--port` exist for this
   when running `install.sh`:
   `sudo W1NWIRE_PUBLIC_HOST=vpn.example.com bash install.sh --port 51820`.

`nat.enabled` and `outbound_interface` still describe the server's own egress
(the LAN interface), and the masquerade rule stays the same. What changes is
outside the machine.

Carrier-grade NAT (no port forwarding possible) cannot host a server. Use a
VPS, or a relay with a public address.

### Two NATs / no public IPv4

Out of scope. The protocol is UDP with a fixed server endpoint; there is no
hole punching and no relay.

---

## 9. Logs

The server logs with Go's `slog` text handler to **stderr**; systemd sends that
to the journal. There is no log file and nothing to rotate.

```bash
journalctl -u w1nwire-server -n 100 --no-pager
journalctl -u w1nwire-server -f
journalctl -u w1nwire-server -o cat | grep -i handshake
```

What is logged at `info`:

* `w1nwire server version=... public_key=...` at startup (the public key is
  safe to log; it is what the invite codes contain),
* `server is listening address=... peers=N`,
* `tunnel device is ready name=... address=...`,
* `client connected peer=... address=...` on every successful handshake,
* `session expired peer=...` after three minutes without traffic,
* `the server stopped`.

At `debug` additionally: rejected cookies and handshakes, unknown message
types, per-packet payload sizes, stream frame errors. `debug` is per packet —
expect a lot of output on a busy tunnel.

The installer's own log is the terminal output; `systemctl status` shows the
last lines if the service fails to start.

---

## 10. Performance notes

The data path is intentionally simple, and the numbers follow from that.

* **One UDP socket, one reader, one writer.** Datagrams are read by a single
  goroutine, copied once, decrypted with ChaCha20-Poly1305 and written to the
  TUN device. TUN packets take the same path in reverse. On a small instance
  one core is the throughput limit; expect hundreds of Mbit/s, not tens of
  Gbit/s. The server does not shard sessions across cores.
* **MTU 1420.** The tunnel adds an IP + UDP + AEAD header to every packet. 1420
  keeps a full-size tunnel packet under the 1500-byte path MTU, so TCP does not
  need PMTU discovery (which the fixed MTU would break anyway). Lower it for
  paths that need it (some mobile networks); keep both sides equal, because the
  client configuration carries the same value.
* **Handshakes are linear in peers** (see §7). Dozens of peers are fine,
  thousands are not.
* **Rekeying is cheap but not free.** The client starts a new handshake every
  two minutes; that is one X25519 exchange per peer per two minutes. Sessions
  expire after three minutes without traffic.
* **Keepalives** are sent by the server every 15 seconds to every peer with a
  session. They keep NAT mappings alive and count as `packets_total{kind="keepalive"}`.
* **Kernel side.** Forwarding and conntrack are the real limits for many
  simultaneous flows: `sysctl net.netfilter.nf_conntrack_max` (default 65536 on
  small instances) and the NAT table. If the tunnel carries a lot of small UDP
  flows, conntrack is what fills up first.
* **Egress is the bill.** On Oracle Always Free the allowance is 10 TB/month.
  The instance's CPU will run out long before that, but a saturated tunnel does
  count against it.
* **`-log-level debug` costs real throughput** — every packet formats a log
  line.

---

## 11. Backup and restore

Everything that matters is three paths:

```bash
sudo tar -C /etc -czf w1nwire-backup.tgz w1nwire
```

Restore on a new machine:

```bash
sudo tar -C /etc -xzf w1nwire-backup.tgz
sudo bash install.sh            # reuses server.json and the existing peers
```

Because the installer keeps an existing `server.json`, the peers and the server
key survive a reinstall — the invite codes keep working as long as the endpoint
address stays the same. If the public IP changes, the codes carry the old
`host:port` and have to be reissued (the invite is generated on the server, so
`peer add` is the way to get a fresh one).

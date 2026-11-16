# Windows tunnel support

The Windows client uses [Wintun](https://www.wintun.net/) as a layer 3 TUN
device. Wintun carries raw IPv4/IPv6 packets through a shared memory ring and
installs a small kernel driver the first time an adapter is created.

## Files

| File | Purpose |
| --- | --- |
| `wintun_windows.go` | Locating, downloading and loading `wintun.dll`, plus the pure Go bindings and `WintunStatus` / `WintunVersion`. |
| `tun_windows.go` | `Open`, the `Device` implementation, the packet ring and the session lifetime. |
| `routes_windows.go` | Address and MTU configuration, split default routes, bypass route, `DefaultRoute`, `Cleanup`, `ResetInterface`. |
| `wintun_windows_test.go` | Layout checks, download/hash verification, status and non-admin `Open` tests. |

`tun_unsupported.go` and `routes_unsupported.go` are built only on platforms
that are neither Linux nor Windows.

## DLL lifecycle

`WintunPath` searches, in order:

1. the directory of the running executable;
2. `%LOCALAPPDATA%\W1nWire\wintun.dll` (created on demand);
3. `%SystemRoot%\System32\wintun.dll`.

If no copy is found, `InstallWintun(ctx)` downloads
`https://www.wintun.net/builds/wintun-0.14.1.zip`, extracts
`bin/amd64/wintun.dll`, verifies its SHA-256 and moves it into
`%LOCALAPPDATA%\W1nWire\` with a temporary file plus rename. The DLL is GPLv2,
so it is not committed to the repository or embedded in the EXE.

The pinned DLL is the official 0.14.1 amd64 build:

```
SHA-256 E5DA8447DC2C320EDC0FC52FA01885C103DE8C118481F683643CACC3220DAFCE
```

The archive itself is:

```
SHA-256 07C256185D6EE3652E09FA55C0B673E2624B565E02C4B9091C79CA7D2F24EF51
```

To install manually, copy `wintun.dll` next to the executable or into
`%LOCALAPPDATA%\W1nWire\`. A DLL in the executable directory wins over the
per-user copy and System32, which allows an operator to override the version.

`WintunStatus() (path, version string, err error)` is the GUI entry point:
it returns an error wrapping `ErrWintunUnavailable` when no DLL exists, and
returns the path with an empty version when the DLL is installed but the
driver is not running. `WintunVersion()` returns `ErrWintunNotRunning` in that
case. `WintunGetRunningDriverVersion` returns the driver file's
`dwFileVersionMS`, which is `major << 16 | minor` (0.14.1 arrives as
`0x0000000e`); the formatter also accepts the packed
`major << 24 | minor << 16 | patch` value (0.14.1 as `0x000e0001`) and the
classic `major << 16 | minor << 8 | patch` value.

## Adapter and session

`Open(name, mtu)` first tries `WintunOpenAdapter` so a stale adapter from a
crashed process is reused rather than duplicated. If it does not exist,
`WintunCreateAdapter` creates it with the pool name `W1nWire` and a fixed GUID.
The fixed GUID gives the adapter a deterministic network location awareness
entry. The session uses a 4 MiB ring.

`Read` waits on the session's read-wait event for at most 200 ms, then checks
the closed flag. `WintunReceivePacket` returns a pointer into the ring; the
bytes are copied out and `WintunReleaseReceivePacket` is called immediately.
`Write` allocates a send packet, copies the bytes and sends it; a full ring
(`ERROR_BUFFER_OVERFLOW`) is retried a few times before returning an error.

`Close` is idempotent. It sets the closed flag, ends the session, removes the
routes and closes the adapter. The session lock lets a blocked `Read` notice
the closed flag within one wait period, so `Close` cannot hang.

Creating an adapter installs the kernel driver and therefore requires
administrator rights. Loading the DLL, `WintunStatus` and `WintunVersion` do
not.

## Route strategy

Full tunnel mode does not replace the physical default route. Instead it adds
two more specific routes through the Wintun adapter:

```
0.0.0.0/1     -> gateway or 0.0.0.0 on W1nWire
128.0.0.0/1   -> gateway or 0.0.0.0 on W1nWire
```

Together they cover all IPv4 destinations, but they are more specific than
`0.0.0.0/0`, so the physical default route stays in the table. Removing the
two routes restores normal connectivity even after a crash.

The VPN server address gets a `/32` bypass route through the physical default
gateway. Without it the encrypted UDP packets would follow the split default
route into the tunnel and loop. `ConfigureInterface` snapshots the physical
default route *before* any tunnel route exists; `ConfigureBypassRoute` uses
that snapshot. If no snapshot exists the bypass call fails with an explicit
error instead of guessing.

IPv6 leakage is blocked with a best effort pair of routes:

```
::/1       -> :: on W1nWire
8000::/1   -> :: on W1nWire
```

A failure to add them is ignored because a machine without IPv6 must not fail
the connection. `ConfigureInterface` also sets the IPv6 interface MTU and
metric to keep that path usable.

`DefaultRoute()` scans the IPv4 forwarding table and excludes the remembered
Wintun interface, so it returns the physical alias and next hop even while the
split default routes are active.

IP Helper (`iphlpapi.dll`) is used through hand-written `MIB_*` structures in
`routes_windows.go`; each structure documents its Windows SDK layout. If an
address, MTU or interface call fails, `ConfigureInterface` falls back to
`netsh` and reports both errors only when both paths fail. `ConfigureNAT` is a
no-op because the Windows build only implements the client.

## Crash recovery

Every route this package installs is tracked as
`{destination prefix, interface index, next hop}` in memory and mirrored to
`%LOCALAPPDATA%\W1nWire\routes.json` (best effort). `Cleanup` removes the
routes, `Close` calls it, and `ResetInterface(name)` is meant to run at
application start before `Open`:

* delete `0.0.0.0/1`, `128.0.0.0/1`, `::/1` and `8000::/1` on the named adapter
  by walking the forwarding table;
* delete the routes recorded by the previous process, including the `/32`
  bypass route on the physical interface;
* delete anything still tracked in the current process;
* treat a missing adapter (`ERROR_INVALID_PARAMETER`, `ERROR_FILE_NOT_FOUND`,
  `ERROR_NOT_FOUND`) and an already missing route as success.

Route deletion is idempotent and never fails the device close.

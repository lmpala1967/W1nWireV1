# W1nWire on Windows

This guide covers the desktop application: installation, the two modes, what
it changes on the machine, and what to do when something does not work.

## Install

1. Download `w1nwire-desktop.exe` from the
   [releases page](https://github.com/w1nchi/w1nwire/releases).
2. Put it anywhere, for example `C:\Program Files\W1nWire\`. There is no
   installer and no registry litter: the application keeps its files in
   `%APPDATA%\W1nWire`.
3. Run it. Windows SmartScreen shows a warning because the binary is not code
   signed — choose **More info → Run anyway**. Verify the download against
   `SHA256SUMS.txt` from the same release if you want to be sure:

   ```powershell
   Get-FileHash .\w1nwire-desktop.exe -Algorithm SHA256
   ```

Requirements: Windows 10 1803+ or Windows 11 with the Microsoft Edge WebView2
runtime. It ships with Windows 11 and with any Windows 10 that has Edge; if it
is missing the application falls back to opening the interface in your default
browser.

## First run

Press **«Добавить сервер» → «Импорт кода»** and paste the invite code printed
by the server installer (`ww1....`). The code already contains:

* the address of the server,
* its public key,
* the private key of this client.

That is why the code is a secret: anybody who has it can connect to your server
as you. To issue a second code (for a laptop, for a friend) run on the server:

```bash
sudo w1nwire-server peer add -config /etc/w1nwire/server.json -host <public-ip> -name laptop
sudo systemctl restart w1nwire-server
```

To revoke a code, remove the peer and restart the service:

```bash
sudo w1nwire-server peer list -config /etc/w1nwire/server.json
sudo w1nwire-server peer remove -config /etc/w1nwire/server.json -id laptop-1a2b3c
sudo systemctl restart w1nwire-server
```

## Tunnel mode

Sends all IPv4 traffic of the machine through the server. Needs administrator
rights, because it creates a network adapter and edits the routing table.

What happens on connect:

1. The application downloads `wintun.dll` once (about 430 KB) from
   `wintun.net`, verifies its SHA-256 and stores it in
   `%LOCALAPPDATA%\W1nWire\wintun.dll`. You can also put the DLL there
   yourself.
2. A `W1nWire` adapter appears in **Network Connections** with the address
   `10.66.0.2/24` and MTU 1420.
3. The default route is not replaced. Instead `0.0.0.0/1` and `128.0.0.0/1`
   point at the tunnel, which covers the whole IPv4 space while keeping the
   original default route intact and easy to restore.
4. A `/32` route pins the VPN server's address to the physical gateway, so the
   encrypted UDP does not go into the tunnel it belongs to.
5. IPv6 is blocked for the duration: applications fall back to IPv4 instead of
   leaking around the tunnel.

On disconnect everything is deleted, including the routes. If the application
is killed, the routes are cleaned up on the next start — before connecting.

**If something goes wrong** and the machine has no internet after a crash,
open an elevated PowerShell and run:

```powershell
route delete 0.0.0.0 mask 128.0.0.0
route delete 128.0.0.0 mask 128.0.0.0
```

or simply start W1nWire again and let it clean up (it does so even if you do
not connect).

## Proxy mode

No administrator rights, no driver, no routing table changes. The application
starts a SOCKS5 listener on `127.0.0.1:1080` and tunnels the TCP connections
through the server, which resolves the host names — so DNS does not leak.

The **«Системный прокси»** switch writes the per-user WinINET settings and
restores the previous values (including a PAC script, if one was configured)
when the tunnel goes down. Applications that read WinINET settings — Edge,
Chrome, Internet Explorer, most .NET and WinHTTP programs — start using the
tunnel immediately.

Not covered automatically:

* **Firefox** — set `SOCKS v5`, host `127.0.0.1`, port `1080`, and enable
  *Proxy DNS when using SOCKS v5* in the network settings.
* **Telegram Desktop** — Settings → Advanced → Connection type → SOCKS5,
  `127.0.0.1:1080`.
* Games and applications with their own network stack — most of them ignore
  the system proxy; use the tunnel mode for those.

## Settings that matter

| Setting | Meaning |
|---|---|
| `DNS` | The resolver handed to the tunnel adapter in tunnel mode. Queries go through the tunnel, so it must be a public resolver (`1.1.1.1`, `9.9.9.9`). |
| `MTU` | 1420 by default: 1500 minus the W1nWire header and the UDP/IP overhead. Lower it to 1280 if some sites hang. |
| `Прокси-порт` | The local SOCKS5 port. |
| `Блокировать IPv6` | Adds the blackhole routes while the tunnel is up. Keep it on unless you have IPv6 at home and know what you are doing. |
| `Автозапуск` | Adds `HKCU\...\Run\W1nWire`. |
| `Подключаться при запуске` | Connects the active profile when the application starts. |

## Where the files live

```
%APPDATA%\W1nWire\config.json          profiles and settings (mode 0600)
%APPDATA%\W1nWire\logs\w1nwire.log     rotating log, 4 MB
%LOCALAPPDATA%\W1nWire\webview\        WebView2 profile of the window (cache, not data)
%LOCALAPPDATA%\W1nWire\wintun.dll      the tunnel driver, downloaded on demand
```

Nothing is written elsewhere. To uninstall: delete the executable, the two
folders above, and (if you enabled it) the autostart entry — the application
removes that entry itself when you turn the setting off.

## Logs

The **Журнал** tab shows the last 500 lines live. The same lines are written to
`%APPDATA%\W1nWire\logs\w1nwire.log`. Useful lines when debugging:

* `the tunnel is established` — the handshake succeeded and the address was
  assigned;
* `handshake timed out: the server did not answer` — wrong address, wrong
  port, or a firewall dropping UDP;
* `the handshake was rejected` — the client's public key is not in the
  server's `peers` list;
* `the wintun driver could not be installed` — no internet, or a proxy that
  blocks `wintun.net`.

## Self test

**Настройки → Проверить установку** runs a real server on loopback and a real
client, and fetches a page through the SOCKS5 listener. It proves that the
cryptography, the framing and the proxy work on this machine. It does not
test the Wintun driver (that needs a Linux server on the other side), but it
does test everything else.

## Command line

The GUI is the product, but the binary keeps the usual switches:

```
w1nwire-desktop.exe                 open the window
w1nwire-desktop.exe -no-window      serve the interface in the default browser
w1nwire-desktop.exe -debug          verbose log, WebView2 devtools
w1nwire-desktop.exe -data-dir D     keep the profiles in D
w1nwire-desktop.exe -version        print the version
```

# W1nWire on the iPhone

## The short version

There is no native W1nWire app for iOS. The free way to use your server from an
iPhone is the **WireGuard** endpoint that `install.sh --ios` creates: the phone
runs the official WireGuard app (free, App Store) and connects to a second,
independent tunnel on UDP 51821. From that point the phone speaks WireGuard,
not W1nWire.

You still get what you want — the phone's traffic leaves through your server's
IP address — you just get it through WireGuard.

---

## 1. Why the native protocol cannot run on iOS for free

On iOS, a VPN client is not a normal app. It is a **Network Extension**
(`NEPacketTunnelProvider`): a separately packaged, separately signed binary that
the system starts and that owns the tunnel interface. Apple gates that with an
entitlement, `com.apple.developer.networking.networkextension`, and grants it
only to members of the **paid** Apple Developer Program. A free Apple ID can
sign apps for seven days, but it cannot carry that entitlement, so a
self-built W1nWire client cannot create a VPN interface at all.

Two workarounds exist, and neither is free in the way this project needs:

* pay the Apple Developer Program (99 USD/year) and ship a W1nWire client with
  the entitlement, or
* put the W1nWire client inside a commercial VPN-container app that already has
  the entitlement, which is a third-party dependency and not something this
  repository can offer.

The SOCKS5 proxy mode of the W1nWire client is not a way around it either: iOS
has no system-wide SOCKS proxy setting, so only apps that implement SOCKS5
themselves would use it, and the phone's other traffic would stay outside the
tunnel.

WireGuard is the exception that makes this practical: the official WireGuard
app is free in the App Store, is signed by a developer who already holds the
entitlement, and is designed exactly for this. So the iPhone uses WireGuard.

---

## 2. What you get

| | W1nWire (desktop) | WireGuard (iPhone) |
| --- | --- | --- |
| Port | UDP 51820 | UDP 51821 |
| Tunnel subnet | `10.66.0.0/24` | `10.67.0.0/24` |
| Server address | `10.66.0.1` | `10.67.0.1` |
| Credential | invite code (`ww1.…`) | WireGuard configuration / QR code |
| Client | W1nWire app | WireGuard app (App Store, free) |
| Protocol | W1nWire | WireGuard |

The two endpoints are independent: removing the iPhone setup does not touch the
W1nWire server, and updating W1nWire does not touch `wg0`.

---

## 3. Set it up

### Fresh server

```bash
sudo bash install.sh --ios
```

At the end, in addition to the W1nWire invite code, the installer prints a QR
code. That QR code is the phone's configuration.

### Server that is already installed

```bash
# from the downloaded install.sh, or from the checkout: deploy/install.sh
sudo bash install.sh --ios
```

`--ios` is idempotent. On a server that already has `/etc/wireguard/wg0.conf`
it does **not** overwrite it: it starts `wg-quick@wg0`, opens UDP 51821 and
prints the QR code of the newest phone configuration it finds. If no phone
configuration exists, it adds one.

> QR note: the QR code is printed as UTF-8 blocks (`qrencode -t ansiutf8`) and
> needs a terminal with a black background and a font that can draw block
> characters. If it looks like a mess, see §7.

### What the installer creates

| Path | Purpose |
| ---- | ------- |
| `/etc/wireguard/wg0.conf` | the server side: `10.67.0.1/24`, UDP 51821, one `[Peer]` per phone |
| `/etc/wireguard/wg0-client-1.conf` | the phone's configuration, the file the QR code encodes |
| `/etc/wireguard/wg0-server.pub` | the server public key, for adding phones later |

All of them are 0600 inside a 0700 directory.

### Manual setup (server not installed with `install.sh`)

The script is the supported path, but the underlying steps are short:

```bash
sudo apt-get update && sudo apt-get install -y wireguard qrencode
sudo install -d -m 0700 /etc/wireguard
cd /etc/wireguard
umask 077

wg genkey > server.key && wg pubkey < server.key > server.pub
wg genkey > phone1.key && wg pubkey < phone1.key > phone1.pub

cat > wg0.conf <<EOF
[Interface]
Address = 10.67.0.1/24
ListenPort = 51821
PrivateKey = $(cat server.key)
PostUp = sysctl -q -w net.ipv4.ip_forward=1
PostUp = iptables -C FORWARD -i %i -j ACCEPT 2>/dev/null || iptables -I FORWARD 1 -i %i -j ACCEPT
PostUp = iptables -t nat -C POSTROUTING -s 10.67.0.0/24 -o eth0 -j MASQUERADE 2>/dev/null || iptables -t nat -A POSTROUTING -s 10.67.0.0/24 -o eth0 -j MASQUERADE
PostDown = iptables -D FORWARD -i %i -j ACCEPT 2>/dev/null || true
PostDown = iptables -t nat -D POSTROUTING -s 10.67.0.0/24 -o eth0 -j MASQUERADE 2>/dev/null || true

[Peer]
# phone-1
PublicKey = $(cat phone1.pub)
AllowedIPs = 10.67.0.2/32
EOF

cat > wg0-client-1.conf <<EOF
[Interface]
PrivateKey = $(cat phone1.key)
Address = 10.67.0.2/32
DNS = 1.1.1.1
MTU = 1420

[Peer]
PublicKey = $(cat server.pub)
Endpoint = YOUR.PUBLIC.IP:51821
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25
EOF

systemctl enable --now wg-quick@wg0
qrencode -t ansiutf8 < wg0-client-1.conf
```

Replace `eth0` with the real outbound interface (`ip -4 route show default`)
and `YOUR.PUBLIC.IP` with the server's public address. Open **UDP 51821** in the
cloud security list — the same step the W1nWire port needs.

---

## 4. Install the app and scan the code

1. Install **WireGuard** from the App Store. It is free and published by the
   WireGuard project; there is no account, no subscription and no analytics.
2. On the server terminal, run the command that prints the QR code:

   ```bash
   sudo qrencode -t ansiutf8 < /etc/wireguard/wg0-client-1.conf
   ```

3. In the app: **+** (top right) → **Create from QR code** → allow camera
   access → scan the code shown in the terminal.
4. Give the tunnel a name (`W1nWire`), save it, and switch the toggle on. iOS
   asks for permission to add a VPN configuration — accept it (Face ID / pass
   code).

Scanning replaces typing: the configuration is a few hundred characters of
keys and addresses, and a single wrong character makes the handshake fail
silently.

### Without a camera

Copy the file instead and open it on the phone (AirDrop, iCloud Drive, Files):

```bash
scp ubuntu@<server>:/etc/wireguard/wg0-client-1.conf .
```

In the app: **+** → **Create from file or archive** → pick the `.conf`.

---

## 5. Verify

1. In the app the tunnel shows **Active**, and after a few seconds a
   **handshake** timestamp and increasing received/sent counters.
2. Open Safari and go to **https://api.ipify.org**. It must show the server's
   public IP, not the mobile carrier's.
3. On the server, `sudo wg show` lists the peer with a recent handshake and
   non-zero transfer:

   ```
   peer: <phone public key>
     endpoint: 203.0.113.45:51422
     allowed ips: 10.67.0.2/32
     latest handshake: 12 seconds ago
     transfer: 1.21 MiB received, 8.44 MiB sent
   ```

If the handshake stays empty, the packets are not arriving: see §7.

---

## 6. More phones, and revocation

### Add a second phone

```bash
sudo bash install.sh --add-phone
```

It appends the next peer (`10.67.0.3`), writes
`/etc/wireguard/wg0-client-2.conf`, restarts `wg-quick@wg0` and prints the new
QR code. Every phone has its own key pair and its own `/32`; never reuse one
configuration on two devices — the addresses would collide.

Manual equivalent: add a `[Peer]` block with a new key and
`AllowedIPs = 10.67.0.3/32`, restart `wg-quick@wg0`, then build a client
configuration with the same server public key and endpoint.

### Revoke a phone

```bash
sudo nano /etc/wireguard/wg0.conf     # delete that phone's [Peer] block
sudo systemctl restart wg-quick@wg0
sudo rm -f /etc/wireguard/wg0-client-2.conf
```

The phone loses access as soon as the interface restarts. Deleting the client
file afterwards removes the private key from the server; the phone still has
its copy, which is now useless.

If a phone is lost, revoke it and treat the QR code as leaked. The server's
`wg0.conf` contains only public keys, so the server itself is not compromised.

---

## 7. Troubleshooting

**The QR code looks like garbage.** The terminal font or colours cannot render
it. Generate a PNG instead and open it on the phone or a screen:

```bash
qrencode -o /tmp/wg0-phone1.png < /etc/wireguard/wg0-client-1.conf
# then copy it over: scp ubuntu@<server>:/tmp/wg0-phone1.png .
```

**The app says "no handshake".** UDP 51821 is not reaching the server:

```bash
sudo ss -lunp | grep 51821                  # interface listening?
sudo tcpdump -ni any udp port 51821         # packets arriving while the phone tries?
sudo iptables -S INPUT | head               # Oracle REJECT rule before an ACCEPT?
```

Add an ingress rule for UDP 51821 to the cloud security list. The installer
opens the host firewall, but it cannot change the provider's.

**Connected, but no internet.** Same checks as for the W1nWire tunnel:

```bash
sysctl net.ipv4.ip_forward
sudo iptables -S FORWARD | grep wg0
sudo iptables -t nat -S POSTROUTING | grep 10.67
ip -4 route show default                    # does the MASQUERADE interface still exist?
```

**DNS fails, everything else works.** The profile sets `DNS = 1.1.1.1`. If that
resolver is blocked on the network you are on, edit the configuration in the
app and set another resolver, or remove the `DNS` line to keep the carrier's.

**Battery.** `PersistentKeepalive = 25` sends a tiny packet every 25 seconds to
keep the NAT mapping open. That is the right trade-off for a phone that moves
between networks; if you only use the tunnel occasionally, deleting the line
saves a little power at the cost of a slower first packet. iOS suspends
background networking aggressively: use the app's **On-Demand** setting
(Always, or Wi-Fi only) if the tunnel should come up by itself instead of you
flipping the switch each time.

---

## 8. Limitations

* **It is WireGuard, not W1nWire.** The phone does not exercise the W1nWire
  handshake, replay window, cookie or streams. The desktop app and the phone
  use different protocols and different subnets.
* **The `wg0` endpoint is full-tunnel** (`AllowedIPs = 0.0.0.0/0`). Split
  tunnelling is possible by narrowing that list in the profile, but then those
  destinations bypass the server.
* **The phone has no W1nWire invite code.** `ww1.…` codes are for the desktop
  app; the QR code is the phone's credential.
* **Metrics differ.** The phone's traffic shows up in `wg show` and in the
  interface counters, not in the `w1nwire_*` Prometheus metrics.
* **iOS chooses the routes.** The WireGuard app, not this repository, decides
  whether the tunnel is on-demand, always on, or only on Wi-Fi.

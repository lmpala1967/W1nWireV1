# Security policy

## Reporting a vulnerability

Open a private security advisory on GitHub
(`Security → Advisories → Report a vulnerability`) or write to the maintainer
of the repository. Please do not open a public issue for a vulnerability that
would let somebody read or redirect the traffic of another user.

Include the version (`w1nwire-desktop -version`, or the release tag), the
platform, and a description of the impact. A proof of concept helps; a
description of the attack is enough.

## What is in scope

* The protocol implementation: handshake, key schedule, nonce construction,
  replay window, framing, cookie.
* The Windows client: routing, the system proxy handling, the local API.
* The server: peer handling, NAT, the install script.
* The invite code format and anything that could leak a private key.

## What is not in scope

* The Wintun driver (report to the WireGuard project) and the WebView2
  runtime (report to Microsoft).
* The fact that the server sees the traffic: it terminates the tunnel by
  design, see [docs/THREAT-MODEL.md](docs/THREAT-MODEL.md).
* Traffic analysis, blocking by DPI, and the absence of obfuscation: known,
  documented limitations, not vulnerabilities.
* The unsigned Windows binary and the SmartScreen warning.

## Supported versions

Only the latest release receives fixes. The protocol version is part of the
wire format: a client and a server of different major versions are not
expected to interoperate.

## Good practice for operators

* Keep the invite codes secret: they contain the private key of a client.
* Rotate a code that leaked: `w1nwire-server peer remove` then `peer add`.
* Do not expose the metrics endpoint (`127.0.0.1:9187`) to the internet.
* Keep the server updated; the installer is idempotent and can be re-run.

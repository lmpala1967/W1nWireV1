# W1nWire — Threat Model

This document states what W1nWire protects, what it deliberately does not
protect, and what an attacker with each capability can still achieve. It is a
self-assessment, not a third-party audit.

The byte-level description of the wire format lives in
[`PROTOCOL.md`](PROTOCOL.md); the component overview lives in
[`ARCHITECTURE.md`](ARCHITECTURE.md).

---

## 1. Scope and assumptions

**In scope.** The protocol between a W1nWire client and a W1nWire server
running over UDP, the local proxy mode on the client, and the configuration
and key material on both ends.

**Out of scope.** The security of the operating system, the virtualisation
layer, the OCI tenancy, the TLS stack of any application that runs over the
tunnel, and the physical security of the machine.

**Assumptions.**

* The static private keys of the server and of the clients are stored with
  restrictive permissions and are not leaked. A leaked static key allows an
  attacker to impersonate that peer.
* The client trusts the server it configured. This is the fundamental trade-off
  of the model: the server terminates the tunnel and therefore sees the
  plaintext.
* The clocks of the client and the server are roughly synchronised. The
  protocol rejects a handshake when the difference exceeds five minutes, it
  does not attempt to correct it.
* `crypto/rand` and the operating system key material are trustworthy.

---

## 2. Assets

### 2.1 Protected assets

| Asset | Where it lives | Why it matters |
| ----- | -------------- | -------------- |
| Payload of the tunnel | in flight between client and server | The content of the user's traffic |
| Static private keys of the peers | server configuration, client configuration | A leak allows impersonation |
| Ephemeral private keys | memory of the two peers during a handshake | Compromise of an ephemeral key breaks one session |
| Session keys (data plane) | memory of both peers | Confidentiality and integrity of the data plane |
| Identity of the peer | the peer list on the server | A peer must not be able to claim the identity of another peer |
| Availability of the handshake | server CPU | An attacker must not be able to make the server do expensive work for nothing |

### 2.2 Assets that are explicitly NOT protected

| Asset | Status |
| ----- | ------ |
| The fact that a VPN tunnel is being used | Not protected. The traffic is not disguised, see section 6.2 |
| Packet sizes and timing | Not protected. The sizes and the inter-arrival times of the UDP datagrams are visible to the network |
| The identity of the destination | Protected in transit, **not** on the server. The server sees every destination |
| The plaintext of the traffic on the server host | Not protected. The server terminates the tunnel |
| The plaintext on a compromised client host | Not protected. The decryption key lives in the client memory |
| The mapping between a client and its source IP address | Minimised by the cookie, not eliminated. The server still observes the addresses of its peers |
| Traffic content on a compromised server host | Not protected |

---

## 3. Adversaries

### A1. Passive observer on the network

Reads packets, measures sizes and timing, records them for later analysis.
Cannot inject packets.

### A2. Active man-in-the-middle on the network

Injects, modifies, drops and replays datagrams. Can spoof the source address
of a UDP datagram. May be able to delay or reorder packets.

### A3. Malicious or compromised server

Operates the endpoint the client dials. Has the server's static private key
and sees all plaintext. May return crafted handshake responses, may serve
modified content, may record everything.

### A4. Local attacker on the client host

Has access to the client process: reads memory, reads the configuration file,
reads the private key, uses the local SOCKS5 listener.

### A5. Denial-of-service attacker

Floods the server with traffic: spoofed handshakes, forged data packets,
cookies, garbage.

---

## 4. Cryptographic construction

| Element | Choice |
| ------- | ------ |
| Key agreement | X25519 (`crypto/ecdh`), static keys for both peers plus an ephemeral pair per handshake |
| Key derivation | HKDF-SHA256, implemented locally in `internal/crypto/kdf.go`, with a distinct `info` label per derived key |
| Authenticated encryption | ChaCha20-Poly1305 (`golang.org/x/crypto/chacha20poly1305`), 12-byte nonce: four zero bytes plus a little-endian counter |
| Nonce spaces | One key **and** one counter per direction, so the two directions never share a nonce |
| Handshake | Noise-IK-style, two messages, transcript bound into the HKDF salt |
| Cookie | Stateless AEAD blob under a per-process server key, binds the observed source address and a 120 s expiry |
| Replay protection | Sliding window of 64 packets, one per session and direction |
| Rekey | Every 2 minutes, new ephemeral pair, new session keys |
| Liveness check | The server timestamp inside the handshake response must be within five minutes of the local clock |

Key schedule:

```
msg1 (client -> server)   k1 = HKDF(DH(e_i, s_r) || DH(s_i, s_r), salt = transcript1, "w1nwire/v1/msg1")
msg2 (server -> client)   k2 = HKDF(DH(e_i, e_r) || DH(e_r, s_i), salt = transcript2, "w1nwire/v1/msg2")
data keys                 HKDF(DH(e_i, e_r), salt = transcript2, "w1nwire/v1/data/c2s" | "w1nwire/v1/data/s2c")
```

The transcript is a hash chain:

```
salt0 = SHA256("w1nwire/v1/transcript" || version)
salt1 = SHA256(salt0 || senderIndex || e_i || cookie)
salt2 = SHA256(salt1 || e_r)
```

`salt1` is the HKDF salt of `k1`, `salt2` is the salt of `k2` and of the two
data keys. Both sides derive the same salts from the same bytes, and any
modification of the transcript changes every derived key.

---

## 5. Threat table

| # | Threat | Protection | Residual risk |
| - | ------ | ---------- | ------------- |
| 1 | Eavesdropping of the payload | ChaCha20-Poly1305 with keys derived from X25519 DH; the payload is encrypted and authenticated | Metadata (size, timing, direction) remains visible. See 6.2 |
| 2 | Modification of packets in flight | AEAD authentication tag; a modified ciphertext fails to open and is dropped and counted in `w1nwire_decrypt_errors_total` | None for the payload. Denial of service through dropping is out of scope |
| 3 | Impersonation of the server | The client authenticates the server by decrypting `msg2` with `k2`, which contains `DH(e_r, s_i)`; only the holder of the server's static private key can produce it | A compromised server is indistinguishable from an honest one. See 6.1 |
| 4 | Impersonation of a client | The server identifies the client by trial decryption of `msg1` with `k1`; only the holder of the matching static private key produces a valid `k1`. The decrypted payload must also contain the same static public key | The peer lookup is a linear scan of the peer list, so its cost grows with the number of peers. The cookie bounds that cost per source address |
| 5 | Impersonation of another peer | The static key inside `msg1` is compared against the peer that matched the decryption, and the session table is keyed by the sender index | None, provided the static keys are unique per peer |
| 6 | Replay of data packets | Sliding replay window of 64 counters per session and direction; a counter below the window or already seen is rejected and counted in `w1nwire_replay_dropped_total` | Packets that fall out of the 64-packet window are rejected, so a large loss burst also causes legitimate packets to be dropped. The window is not persisted across a restart |
| 7 | Replay of a handshake | The cookie carries an expiry (120 s) and is bound to the source address; the handshake transcript contains the ephemeral public key, so a recorded `msg1` cannot be re-used with a different ephemeral key | A cookie that is replayed inside its validity window and from the same address is accepted; the resulting session is nevertheless useless to the attacker, because it lacks the ephemeral private key. See 6.4 |
| 8 | Offline guessing of a static key | The handshake requires an interactive DH with an ephemeral key and a live response; an observer cannot compute `DH(s_i, s_r)` from recorded traffic, and `msg1` is encrypted with a key that depends on both ephemeral and static DH results | A passive observer who obtains the client's static public key learns nothing about the private key. The static key can still be stolen from the host |
| 9 | Denial of service through handshakes | The cookie is verified **before** the peer lookup: the server does not spend the expensive trial decryption until the requester proves it can receive replies at its address. Cookie verification is a single AEAD open | An attacker that can receive replies at a spoofed address still forces a full peer lookup per handshake. The cookie does not raise a cost for a real source address |
| 10 | Flood of garbage datagrams | Cheap rejection paths: the type byte selects the handler, malformed messages are rejected by the decoders, unauthenticated data packets fail the AEAD open | The UDP receive path is single-threaded; extreme floods can saturate it. There is no rate limiter in front of the handshake |
| 11 | Denial of service by an authenticated peer | A peer that completes the handshake can consume a session slot and stream slots | Sessions expire after three minutes of silence. There is no per-peer quota |
| 12 | Traffic analysis and DPI | None. The protocol has a fixed 1-byte type and version header, and the datagram sizes map closely onto the sizes of the encapsulated traffic | The tunnel is trivially fingerprintable. See 6.2 |
| 13 | Decryption of a long-lived session after the fact | Forward secrecy. Every handshake uses a fresh ephemeral X25519 pair, and the client rekeys every two minutes, so a recording is only decryptable until the next rekey | A session is still decryptable for the remainder of its two-minute window if the ephemeral private key is captured |
| 14 | Reading the traffic of a client by the server | None. The server terminates the tunnel by design | Accepted by the model. The client must trust the server it configured. See 6.1 |
| 15 | Reading the traffic from a compromised client host | None | The session keys and the private key are in the process memory of the client |
| 16 | Reordering or loss of datagrams | None at the transport level. UDP is a datagram protocol and the frames are not sequenced | TCP running inside the tunnel repairs the ordering. The proxy mode only carries TCP. See 6.3 |
| 17 | Impersonation of a client to the cookie endpoint | The `CookieRequest` carries `mac1 = HMAC-SHA256(mac1Key, senderIndex)` where `mac1Key` is derived from the **server's public key**. It is not a secret: it only proves that the datagram is a well-formed W1nWire message and not random garbage | `mac1` is not an authentication factor. The real client authentication is the trial decryption of `msg1` |

---

## 6. What is deliberately not protected

### 6.1 The server sees everything

This is the central trade-off of the project. The server terminates the
protocol: it holds the data keys, it decrypts every packet and it observes
every destination. A malicious or compromised server can:

* read and modify the entire traffic of the client;
* return a modified web page or inject content;
* log every destination the client contacts;
* lie about its identity? No. The client authenticates the server, so a
  *different* machine cannot impersonate the server without the static private
  key. But a server that *is* the configured server cannot be distinguished
  from an honest one.

The mitigation is operational: run the server on infrastructure you control,
keep the static private key out of reach, and pin the server public key in the
client configuration so that a swapped endpoint is detected immediately.

### 6.2 Metadata and traffic analysis

The protocol protects the content, not the shape of the traffic. An observer
on the network sees:

* that a UDP conversation exists between two addresses;
* the size of every datagram, which maps closely onto the size of the
  encapsulated packet;
* the timing and the direction of the traffic;
* a recognisable 1-byte type and version header on every datagram.

There is no padding, no cover traffic, no fixed-size outer packet and no
attempt to disguise the protocol as something else. A DPI system that knows
the W1nWire header can identify and block the tunnel reliably, and it can
estimate what is inside from the packet sizes.

### 6.3 No transport-level ordering or loss protection

The data plane is plain UDP. Datagrams may be lost, duplicated or reordered.
The protocol detects replays, but it does not reorder: the frames carry no
sequence number of their own. A packet that arrives out of order and was not
seen before is accepted as is, and a packet that was already seen is rejected
as a duplicate by the replay window — which is not the same thing as putting
the stream back in order.

The impact is limited by the payload design:

* in the tunnel mode the encapsulated traffic is IP, and TCP inside the tunnel
  repairs the loss;
* in the proxy mode only TCP is carried, and TCP repairs the loss.

A UDP-based application that is run directly over the tunnel mode would see
the loss and the reordering of the underlying network, which is the expected
behaviour of a datagram network.

### 6.4 No replay protection for the cookie layer

The cookie itself is a stateless AEAD blob: the server keeps no table of
issued cookies, it just decrypts the blob and checks the expiry and the
address. The 12-byte nonce is random per cookie, so a cookie cannot be
forged, but a captured cookie can be presented again:

* from the **same** source address, inside the 120 s validity window — the
  server accepts it, but the attacker still cannot complete the handshake,
  because `msg1` is encrypted with a key derived from an ephemeral DH that the
  attacker does not have. The practical effect is limited to making the server
  spend one peer lookup;
* from a **different** source address — the address binding rejects it.

### 6.5 No protection of the client against itself

The local SOCKS5 listener binds to `127.0.0.1:1080` by default and does not
implement any authentication. Any local process can use the tunnel. On a shared
or multi-user machine this is a local privilege and confidentiality issue.

### 6.6 Side channels

The cryptographic primitives come from the Go standard library and
`golang.org/x/crypto`. `x/crypto/chacha20poly1305` provides a constant-time
implementation, and the comparisons of tags and public keys in W1nWire use a
constant-time helper (`crypto.ConstantTimeEqual`). The X25519 implementation
is the one of `crypto/ecdh`.

That said:

* W1nWire does not implement or audit the primitives itself, and it does not
  make any formal constant-time guarantee about the surrounding code;
* the code does not attempt to be constant-time with respect to the *length* of
  the compared values, which is standard practice and does not leak key
  material in this construction;
* no hardening against cache, timing or power analysis of the local host is
  attempted; the threat model of a local attacker on the client host (A4) is
  essentially "all bets are off".

---

## 7. Forward secrecy

The data keys are derived from `DH(e_i, e_r)`, the Diffie-Hellman between the
two ephemeral keys. The static keys contribute only to the authentication of
the handshake messages, never to the data keys.

Consequences:

* compromising the static private key of the server or of the client at any
  later point does **not** allow the decryption of the recorded data traffic;
* compromising one ephemeral private key exposes only the session it belongs
  to;
* the client rekeys every two minutes, which bounds the lifetime of a single
  ephemeral key even if the process memory is dumped later.

The limit: within the two-minute rekey interval, a dump of the client memory
yields the current ephemeral key and therefore the current session.

---

## 8. Mutual authentication

Both directions are authenticated, with different mechanisms.

**Server to client.** The server's `msg2` is encrypted with
`k2 = HKDF(DH(e_i, e_r) || DH(e_r, s_i))`. The second term is an
ephemeral-static DH between the server's ephemeral key and the client's static
key. Only the holder of the server's static private key can compute it, so a
successful decryption of `msg2` proves that the peer owns the server's static
key. The client additionally rejects a response whose embedded timestamp is
more than five minutes away from the local clock, which detects a stale or
replayed response.

**Client to server.** The server has no hint about which client is talking to
it, so it tries every configured peer: for each peer it computes
`k1 = HKDF(DH(e_i, s_r) || DH(s_i, s_r))` and tries to decrypt `msg1`. Only
the peer that owns the matching static private key produces a valid `k1` and
therefore a valid Poly1305 tag. The decrypted payload contains the client's
static public key, which is compared against the matched peer. A client that
is not in the peer list gets `ErrUnknownPeer`.

The cost of this lookup is the reason the cookie exists: the server must not
perform a linear scan of the peer list for every datagram that claims to be a
handshake.

---

## 9. Denial of service and the stateless cookie

The handshake has an asymmetric cost. The client can produce a `CookieRequest`
for free. The server can produce a `CookieResponse` for free as well, because
the cookie is a stateless AEAD blob: encrypting the expiry and the observed
address costs one AEAD seal.

The expensive part is the peer lookup, which performs an X25519 DH and an
AEAD open per configured peer. The cookie makes that cost conditional: the
server only runs the lookup after it has verified the cookie, and the cookie
can only be obtained by a party that receives replies at the address the
server observed. A spoofed source address cannot obtain a cookie, therefore it
cannot force a peer lookup.

Properties of this design:

* **No server state.** The server does not track pending handshakes, so an
  attacker cannot exhaust memory by starting handshakes. This is the reason
  the cookie is stateless rather than a table of half-open sessions.
* **Address binding.** The cookie contains the address the server observed, so
  it cannot be moved to another source address.
* **Expiry.** The cookie is valid for 120 s, which bounds the replay window
  of the cookie itself.
* **Cost.** One AEAD seal on the request path, one AEAD open on the handshake
  path. Both are constant time with respect to the number of peers.

Limitations:

* an attacker that controls a real source address (a botnet, a compromised
  host) can obtain valid cookies and still force peer lookups. The cookie
  protects against *spoofing*, not against a determined volumetric attacker;
* there is no rate limiting and no connection tracking on the server, so a
  volumetric flood is handled by the operating system and by the network
  rather than by the protocol;
* the peer list is scanned linearly. A large peer list makes every legitimate
  handshake proportionally more expensive.

---

## 10. Summary of the guarantees

Provided:

* confidentiality and integrity of the tunnel payload;
* mutual authentication of the peers through the static key pairs;
* forward secrecy over time, with a two-minute rekey interval;
* replay protection of the data plane within a 64-packet window;
* binding of a handshake to the source address, through the cookie;
* isolation of the peers from each other: a peer can only open streams for
  itself, and its traffic is mapped back to it by the tunnel address.

Not provided:

* protection of the metadata, the sizes or the timing of the traffic;
* disguise of the protocol against DPI;
* resistance to a malicious server;
* protection of a compromised client or a compromised server host;
* ordering and loss recovery at the transport level;
* replay protection of the cookie beyond its 120 s expiry and its address
  binding;
* a formal side-channel analysis of the code around the primitives.

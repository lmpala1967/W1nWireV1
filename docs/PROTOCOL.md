# The W1nWire protocol, version 1

This document describes the wire format of W1nWire. It is the normative
reference for the implementation in `internal/protocol`.

The protocol runs over UDP. Every datagram starts with a type byte followed by
a version byte. All integers are little endian, the address and the port of
the cookie use big endian.

```
+--------+--------+---------------------------+
| type   | ver    | payload                   |
+--------+--------+---------------------------+
```

## 1. Notation

| Symbol       | Meaning                                                       |
|--------------|---------------------------------------------------------------|
| `s_r`        | static private key of the server, X25519                        |
| `s_i`        | static private key of the client, X25519                        |
| `e_i`        | ephemeral private key of the client, X25519                     |
| `e_r`        | ephemeral private key of the server, X25519                     |
| `DH(a, b)`   | X25519 shared secret between the private key `a` and the public key `b` |
| `HKDF(k, s, i)` | HKDF-SHA256 with the input keying material `k`, the salt `s` and the info label `i`, expanded to 32 bytes |
| `AEAD(k, n, m)` | ChaCha20-Poly1305 encryption of `m` with the key `k` and the nonce `n` |
| `H(x)`       | SHA-256                                                         |

The transcript of the handshake is a hash chain. Every handshake message is
folded into it, so a message can never be replayed into a different
conversation.

## 2. Key derivation

```
mac1Key   = HKDF(s_r_public, nil, "w1nwire/v1/mac1")
salt0     = H("w1nwire/v1/transcript" || ver)
salt1     = H(salt0 || senderIndex || e_i || cookie)
salt2     = H(salt1 || e_r)
```

The keys of the two handshake messages and of the data plane:

```
k1        = HKDF(DH(e_i, s_r) || DH(s_i, s_r), salt1, "w1nwire/v1/msg1")
k2        = HKDF(DH(e_i, e_r) || DH(e_r, s_i), salt2, "w1nwire/v1/msg2")
k_c2s     = HKDF(DH(e_i, e_r), salt2, "w1nwire/v1/data/c2s")
k_s2c     = HKDF(DH(e_i, e_r), salt2, "w1nwire/v1/data/s2c")
```

Every key is derived from its own HKDF instance, so a key never depends on
the order in which the caller asks for the other keys.

`DH(e_i, s_r)` and `DH(s_i, s_r)` are only known to the two participants of
the handshake, and only the holder of `s_i` can compute `DH(s_i, s_r)`. The
same holds for `DH(e_r, s_i)` and the server.

## 3. Handshake

```
client                                                  server
  |                                                       |
  |  CookieRequest (0x01)                                |
  |------------------------------------------------------>|
  |                                                       |
  |  CookieResponse (0x02)  + cookie                     |
  |<------------------------------------------------------|
  |                                                       |
  |  HandshakeInit (0x03)  e_i, cookie, ciphertext        |
  |------------------------------------------------------>|
  |                                                       |
  |  HandshakeResp (0x04)  e_r, ciphertext                |
  |<------------------------------------------------------|
  |                                                       |
  |  Data (0x05)                                          |
  |------------------------------------------------------>|
```

### 3.1 CookieRequest, type 0x01

```
+--------+--------+----------------+---------------------------+
| 0x01   | ver    | senderIndex    | mac1 (32 bytes)           |
+--------+--------+----------------+---------------------------+
```

`mac1` is `HMAC-SHA256(mac1Key, senderIndex)`. The key is derived from the
public key of the server, therefore every client can compute it, but random
garbage does not produce a valid tag. The server answers a request only when
the tag is correct, which keeps the amplification of garbage traffic low.

### 3.2 CookieResponse, type 0x02

```
+--------+--------+----------------+----------------+-----------------+
| 0x02   | ver    | senderIndex    | mac1 (32 bytes)| cookie (54 bytes)|
+--------+--------+----------------+----------------+-----------------+
```

The cookie is a stateless blob:

```
cookie = nonceSeed (12 bytes) || AEAD(cookieKey, nonceSeed, expiry || address)
expiry = unix timestamp, 8 bytes
address = IPv6 mapped address (16 bytes) || port (2 bytes, big endian)
```

`cookieKey` is a random secret of the server. The cookie binds the handshake
to the address the server observed and to a short lifetime of 120 seconds.
The server keeps no state for the cookies, so a flood of forged handshakes
does not consume memory. A handshake that presents a cookie from a different
address or an expired cookie is dropped before the expensive peer lookup.

### 3.3 HandshakeInit, type 0x03

```
+--------+--------+----------------+--------------+----------+---------+---------------+
| 0x03   | ver    | senderIndex    | e_i (32)     | ckLen(2) | cookie  | ciphertext    |
+--------+--------+----------------+--------------+----------+---------+---------------+
```

```
ciphertext = AEAD(k1, counter=0, clientStaticPublic (32) || unixTimestamp (8))
```

The server finds the peer by trying every configured peer key: only the peer
that owns the matching private key is able to produce a valid `k1`. A
successful decryption therefore authenticates the client, and the embedded
static public key is checked against the configured one.

### 3.4 HandshakeResponse, type 0x04

```
+--------+--------+----------------+--------------+---------------+
| 0x04   | ver    | senderIndex    | e_r (32)      | ciphertext    |
+--------+--------+----------------+--------------+---------------+
```

```
ciphertext = AEAD(k2, counter=0, unixTimestamp (8) || serverIndex (4) || assignedIPv4 (4) || flags (1))
```

The client verifies `k2`. Only the server that owns `s_r` can compute
`DH(e_r, s_i)`, so a valid decryption authenticates the server. The client
also rejects an answer whose timestamp differs from its own clock by more
than five minutes.

### 3.5 Session keys

After the handshake both sides derive the two data keys `k_c2s` and `k_s2c`
from the same ephemeral secret `DH(e_i, e_r)`. Every direction has its own
key and therefore its own counter space.

The client starts a new handshake every two minutes. The new handshake creates
a new ephemeral key pair, which keeps the forward secrecy of long running
sessions.

## 4. Data plane

### 4.1 Data, type 0x05

```
+--------+---------------+---------------+-----------------+-------------------+
| 0x05   | receiverIndex | senderIndex   | counter (8)     | ciphertext        |
+--------+---------------+---------------+-----------------+-------------------+
```

The counter is the nonce counter of the AEAD and is covered by the replay
window of the receiving session. The two session indexes let the receiver
demultiplex without keeping state per peer.

### 4.2 Keepalive, type 0x06

```
+--------+---------------+---------------+-----------------+---------------+
| 0x06   | receiverIndex | senderIndex   | counter (8)     | tag (16 bytes) |
+--------+---------------+---------------+-----------------+---------------+
```

A keepalive carries an encrypted empty payload. It keeps the mapping of the
NAT alive and lets both sides measure the round trip time.

### 4.3 Nonce

The nonce of the AEAD is 12 bytes: four zero bytes followed by the 64 bit
counter in little endian. The counters of the two directions are independent
because the two directions use different keys.

### 4.4 Replay protection

Every session owns a sliding window of 64 counters. The window remembers the
highest counter it has seen and a bitmap of the 64 counters below it. A
packet is accepted when it is newer than the window or when its bit is not
set. Packets that already fell out of the window are rejected as well.

## 5. Payload frames

Inside the encrypted payload the two working modes share the same transport.
The first byte of the payload selects the frame.

| Type | Name         | Meaning                                        |
|------|--------------|------------------------------------------------|
| 0x01 | FrameIP      | a raw IP packet follows, used by the tunnel mode |
| 0x02 | FrameOpen    | a stream is opened                             |
| 0x03 | FrameOpenAck | the answer to a stream open                    |
| 0x04 | FrameData    | payload of a stream                            |
| 0x05 | FrameClose   | a stream is closed                             |

```
FrameOpen    : [0x02][streamId (4)][protocol (1)][addressLen (2)][address]
FrameOpenAck : [0x03][streamId (4)][status (1)]
FrameData    : [0x04][streamId (4)][data]
FrameClose   : [0x05][streamId (4)][reason (1)]
```

The protocol byte is `0x06` for TCP and `0x07` for UDP. The address is the
destination in `host:port` notation. The host is sent as a name, the server
resolves it, so the DNS queries of the client never leave the tunnel.

## 6. Security properties

- **Mutual authentication.** The client proves its identity with `k1`, the
  server proves its identity with `k2`. Neither of them can be forged without
  the matching private key.
- **Forward secrecy.** The data keys depend on the ephemeral key pair of the
  handshake. A new handshake every two minutes limits the exposure of a
  compromised long term key to the traffic of a single session.
- **No offline attack on the static keys.** The handshake needs the
  participation of both sides and the ephemeral secrets, so a recorded
  handshake cannot be used to test a password or a static key offline.
- **Address binding.** A cookie is only valid for the address it was issued
  for and only for a short time. Spoofed source addresses are dropped before
  the server spends any CPU on them.
- **Replay protection.** The sliding window rejects duplicated and reordered
  packets.
- **Confidentiality and integrity.** Every packet is encrypted and
  authenticated with ChaCha20-Poly1305.

Not covered: the metadata. The size and the timing of the packets stay
visible to an observer of the network, the server sees the whole traffic and
all destinations of its clients, and the traffic is not disguised as another
protocol. See `THREAT-MODEL.md` for the complete analysis.

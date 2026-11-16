# Oracle Cloud Always Free — W1nWire walkthrough

Oracle's Always Free tier is the cheapest realistic home for a self-hosted VPN
exit node: an Ampere A1 instance with up to 4 OCPU and 24 GB of RAM, free for
as long as the account exists, with a 10 TB/month egress allowance. This
document is the console walkthrough, including the parts that surprise people:
the idle reclamation rule and the capacity errors.

Everything here is done in the Oracle Cloud console and over SSH. The
installation itself is one command from [`deploy/README.md`](../deploy/README.md).

---

## 1. What Always Free includes

The allowance that matters for this project (check the Always Free page in the
console for the authoritative, current list):

| Resource | Allowance |
| -------- | --------- |
| Ampere A1 compute | up to **4 OCPU and 24 GB RAM in total**, across all A1 instances |
| AMD micro compute | 2 × `VM.Standard.E2.1.Micro` (1/8 OCPU, 1 GB each) |
| Block storage | **200 GB in total** (boot volumes and extra volumes) |
| Outbound data transfer | **10 TB per month** |
| Reserved public IPv4 | included in the allowance; one is enough here |
| Region | the **home region** chosen at sign-up only |

The A1 instance is the interesting one: 1 OCPU and 6 GB is already far more
than a VPN needs, and the egress allowance is what makes it usable as an exit
node.

---

## 2. Sign up

1. Go to <https://www.oracle.com/cloud/free/> and create an account.
2. Choose the **home region** carefully. It cannot be changed later, Always
   Free resources can only be created there, and it determines the latency your
   tunnel will have. Pick the region closest to you — and, if you can, one where
   Ampere capacity is usually available (Frankfurt, Amsterdam, Ashburn,
   Phoenix and Singapore are common choices; availability changes weekly).
3. A payment card is required **for identity verification**, not for payment.
   Oracle places a temporary authorization hold of about 1 USD/EUR, which is
   released. The Always Free resources themselves are not charged.
4. Verify the phone number and the email, and wait for the account to be
   activated (usually minutes, sometimes a few hours).

If the region you want reports "out of host capacity" later, an account
upgraded to **Pay As You Go** gets a better chance at A1 capacity while the
Always Free entitlements stay free (§9).

---

## 3. Create the instance

1. Menu → **Compute** → **Instances** → **Create instance**.
2. **Name**: `w1nwire`.
3. **Placement**: leave the default availability domain (or pick another one if
   the first has no capacity).
4. **Image**: *Canonical Ubuntu* → **24.04** → the **aarch64** build, and the
   *Minimal* variant if it is offered. The installer supports 22.04 as well;
   24.04 is the current LTS.
5. **Shape**: *Ampere* → **VM.Standard.A1.Flex**. Set:
   * OCPUs: **1** (up to 4 is free; 1 is plenty and keeps room for a second
     instance),
   * Memory: **6 GB**.
6. **Primary VNIC**:
   * create a new VCN (or use an existing one),
   * subnet: public,
   * **Assign a public IPv4 address**: yes,
   * select **Reserved public IP address** (create a new one) — a reserved
     address survives a stop/start, an ephemeral one does not, and the invite
     code contains the address.
7. **Add SSH keys**: paste your public key (`~/.ssh/id_ed25519.pub`). Password
   authentication is not offered; keys are the only way in.
8. **Boot volume**: leave the default (about 47 GB) and make sure *Use
   encryption* stays off unless you have a reason. The 200 GB block storage
   allowance is shared by every volume in the tenancy.
9. **Create**. Wait for the instance state to become **RUNNING** and note the
   **Public IP address**.

At this point the machine exists but nothing can reach it except SSH: Oracle
blocks every port that is not in the VCN's security list, and the Ubuntu image
also ships an iptables ruleset that rejects everything but port 22.

---

## 4. Open the VPN port

Networking → **Virtual Cloud Networks** → your VCN → **Security Lists** →
**Default Security List for …** → **Add Ingress Rules**:

| Field | Value |
| ----- | ----- |
| Stateless | no (leave unchecked) |
| Source Type | CIDR |
| Source CIDR | `0.0.0.0/0` |
| IP Protocol | **UDP** |
| Source Port Range | (empty) |
| Destination Port Range | `51820` |

Add a second rule with destination port `51821` if you want the iPhone /
WireGuard endpoint. Keep the existing `TCP 22` rule — it is the only way in over
SSH; if you want to be strict, change its source from `0.0.0.0/0` to your own
address instead of removing it.

If the VCN has a **Network Security Group** and the instance is in it, add the
same rule there too — the security list is not enough when an NSG is attached.

The installer opens the *host* firewall (ufw or the Oracle iptables chain) but
it cannot change the VCN. Both layers have to allow the port.

---

## 5. Install W1nWire

```bash
ssh -i ~/.ssh/id_ed25519 ubuntu@<public-ip>
```

The user is `ubuntu` on the Ubuntu images.

```bash
curl -fsSL https://raw.githubusercontent.com/<user>/W1nWire/main/deploy/install.sh -o install.sh
sudo bash install.sh
```

Replace `<user>` with the GitHub account that owns the repository, or pass
`--repo <user>/W1nWire`. The script takes a couple of minutes: it downloads the
`arm64` binary, generates the server key, writes `/etc/w1nwire/server.json`,
creates the first peer, starts the service, installs the NAT rules and the
keepalive, and prints the public address and the invite code.

Paste the invite code into the desktop app and check that the tunnel works
(`https://api.ipify.org` in a browser should show the instance's public IP).

---

## 6. The idle reclamation gotcha

Oracle may **stop** an Always Free compute instance that is idle. The published
rule is roughly: if, over a period of seven consecutive days, CPU utilization,
network utilization *and* memory utilization all stay below their thresholds,
the instance is considered unused and can be reclaimed (and, eventually, its
boot volume too).

An idle VPN server is the worst case: near-zero CPU, no traffic when nobody is
connected, and a small memory footprint. A fresh instance that nobody uses can
therefore disappear even though nothing is broken.

The installer handles this with `w1nwire-keepalive.timer`, a systemd timer that
runs every five minutes and does three small things:

* burns about 20 seconds of CPU (`dd` from `/dev/zero` to `/dev/null`),
* allocates about 16 MiB of RAM in `/dev/shm` and releases it,
* downloads a few hundred KiB from a public endpoint.

That is roughly 6–7 % of one core on average, at `Nice=19`, so the VPN always
wins the CPU.

**Be honest about the limit of this trick.** The rule is about utilization over
a week, so short periodic spikes cannot make an unused server look busy. The
keepalive keeps the instance out of the "nothing happened at all" bucket; it is
not a guarantee. What reliably counts is real traffic — using the VPN. If the
instance has to be there, also keep a copy of `/etc/w1nwire` (configuration,
keys, invite codes) somewhere else, so that a rebuilt instance is a
five-minute job.

### Check and tune it

```bash
systemctl status w1nwire-keepalive.timer
systemctl list-timers w1nwire-keepalive.timer
journalctl -u w1nwire-keepalive.service -n 20
```

Tune it by writing `/etc/default/w1nwire-keepalive`:

```bash
W1NWIRE_KEEPALIVE_CPU_SECONDS=30
W1NWIRE_KEEPALIVE_MEM_MIB=32
W1NWIRE_KEEPALIVE_NET_BYTES=1048576
```

then `sudo systemctl restart w1nwire-keepalive.timer`. Keep the CPU time well
below 100 % of a core: the point is a floor, not a heater.

### If the instance was stopped anyway

The instance shows **Stopped** in the console. Start it from the console
(Instance details → **Start**). The reserved public IP and the boot volume
survive, so the VPN comes back with the same endpoint and the same invite
codes. If Oracle already reclaimed the boot volume, rebuild from your backup of
`/etc/w1nwire`.

---

## 7. Did it stay free?

Three places to look, in the console:

1. **Billing & Cost Management → Cost Analysis.** Filter by service and by
   compartment for the current month. The Always Free resources show up as
   zero-cost line items. Any non-zero line is a resource outside the allowance.
2. **Governance & Administration → Tenancy Management → Limits, Quotas and
   Usage.** Look at the compute and block-storage usage for your compartment:
   A1 OCPUs must stay at or below 4 in total, block storage at or below 200 GB
   in total.
3. **The Always Free page** in the console lists the entitlements and what you
   have consumed.

Optional but recommended: set a **budget** (Billing → Budgets) with an alert at
1 USD. It is not a hard cap, but it tells you immediately if a paid resource
sneaked in.

The usual ways to accidentally leave the free tier:

* a second A1 instance that pushes the total above 4 OCPU / 24 GB,
* an extra block volume or volume backup beyond 200 GB total,
* a paid shape (`VM.Standard.E4`, `VM.Standard3`, …) instead of the A1 or the
  E2.1.Micro,
* a load balancer, a NAT gateway, or a second reserved public IP beyond the
  allowance,
* more than 10 TB of egress in a month.

Note that the AMD micro shapes are *also* Always Free, but with 1 GB of RAM
they are a poor fit for a VPN that also runs Prometheus.

---

## 8. Out of host capacity

`Out of host capacity` on A1 is common and has nothing to do with your account's
limits. Options, in order of effect:

1. Try another **availability domain** in the same region.
2. Retry later; capacity is released continuously. A small script that retries
   the create call every few minutes is what many people do.
3. Ask for **fewer resources**: 1 OCPU / 6 GB is far easier to place than
   4 OCPU / 24 GB.
4. Upgrade the tenancy to **Pay As You Go** (§9). A1 capacity is prioritized for
   PAYG accounts; the Always Free entitlements do not change.
5. Use a different home region — not possible without a new account.

While you wait, the AMD micro shape (1/8 OCPU, 1 GB) can run the server. Lower
the keepalive there — `W1NWIRE_KEEPALIVE_CPU_SECONDS=5` in
`/etc/default/w1nwire-keepalive`, or `--no-keepalive` — because 20 seconds every
five minutes is a large share of an eighth of a core, and expect a much slower
tunnel.

---

## 9. Optional: upgrade to Pay As You Go

The upgrade keeps every Always Free entitlement and is the standard fix for A1
capacity. What changes: you now *can* create paid resources, so a mistake costs
money. Set the budget alert from §7 first, and keep the shape and the block
storage inside the allowance.

If you upgrade, Oracle charges for anything outside the Always Free limits —
that is the whole difference. The instance you already have stays free.

---

## 10. Cleanup

Everything is reversible except termination. Before deleting anything, copy
the configuration if you might come back:

```bash
sudo tar -C /etc -czf w1nwire-backup.tgz w1nwire
scp ubuntu@<public-ip>:~/w1nwire-backup.tgz .
```

Then, in the console:

1. **Compute → Instances → your instance → Terminate.** Tick *Permanently
   delete the attached block volumes* if you do not want the boot volume to
   keep counting against the 200 GB allowance.
2. **Networking → Reserved public IPs → release** the address (an unreleased
   reserved IP is not billable on Always Free, but it does count against the
   allowance).
3. **Block Volume → Volumes / Volume Backups**: delete anything left over.
4. **Networking → Virtual Cloud Networks**: delete the VCN. That also removes
   the subnets, the internet gateway and the security list — but only after the
   instance is gone.
5. If you used object storage for the backup, empty the bucket.
6. Check **Cost Analysis** the next day; nothing should remain.

Terminating the instance destroys the server key and every peer. The invite
codes in the desktop app then point at nothing, which is the correct outcome
for a decommissioned server.

---

## 11. Checklist

```
[ ] account created in the right home region, card verified
[ ] VCN + public subnet + internet gateway (the console default does this)
[ ] A1.Flex, 1 OCPU / 6 GB, Ubuntu 24.04 aarch64, reserved public IPv4
[ ] ingress UDP 51820 (and 51821 for the iPhone); TCP 22 already open for SSH
[ ] ssh ubuntu@<ip>; install.sh; invite code pasted into the app
[ ] https://api.ipify.org inside the tunnel shows the instance's IP
[ ] w1nwire-keepalive.timer active
[ ] budget alert at 1 USD
[ ] /etc/w1nwire backed up somewhere else
```

Related: [`deploy/README.md`](../deploy/README.md) for the installer,
[`docs/SERVER.md`](SERVER.md) for the configuration reference,
[`docs/IOS.md`](IOS.md) for the iPhone.

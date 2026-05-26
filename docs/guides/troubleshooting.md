---
title: "Troubleshooting"
description: "Common Windsor failures and their fixes."
---
# Troubleshooting

Operator-facing recipes for the failures a new user is most likely to hit. Each entry names the symptom, explains why it happens, and gives a verbatim recovery command.

If your problem isn't here and you can reproduce it cleanly, open an issue at [github.com/windsorcli/cli/issues](https://github.com/windsorcli/cli/issues) with the full `windsor --verbose` output of the failing command and the contents of `windsor version --verbose`.

## DNS

### DNS on non-systemd Linux

**Symptom:** `windsor configure network` errors with `DNS configuration on this distro requires systemd-resolved, which is not running.`

Windsor's supported Linux DNS setup writes a systemd-resolved drop-in at `/etc/systemd/resolved.conf.d/`. The mechanism requires `systemd-resolved` to be active on the host.

**On distros that ship systemd-resolved** (Ubuntu, Debian, Fedora, openSUSE), enable it and re-run:

```bash
sudo systemctl enable --now systemd-resolved
windsor configure network
```

**On distros that don't ship systemd-resolved** (Alpine, Void, Devuan, NixOS, Slackware), wire DNS manually via your distro's resolver. The Windsor cluster's DNS service IP is in `workstation.dns.address` (run `windsor get config workstation.dns.address` after `windsor up`). Add a per-domain rule for `*.<dns.domain>` via your distro's mechanism:

- **resolvconf / openresolv:** `echo "nameserver <address>" | sudo resolvconf -a <iface>.windsor`
- **dnsmasq:** add `server=/<dns.domain>/<address>` to `/etc/dnsmasq.d/windsor.conf` and reload.
- **unbound:** add a `forward-zone` block targeting `<address>` and restart.

We don't ship a wrapper for these — operators on non-systemd distros are expected to be comfortable with their resolver of choice.

### Windows NRPT GPO conflict

**Symptom:** `windsor configure network` succeeds, but `*.<dns.domain>` still resolves to a corporate DNS server. Stderr shows `⚠ NRPT rule for *.<domain> was added locally, but the effective rule resolves to a different name server (<gpo-served-ip>).`

A Group Policy NRPT rule is overriding the per-machine rule Windsor installs. GPO rules win.

**Recovery options** (in order of preference):

1. **Contact your IT administrator** and ask for either an NRPT exception covering `*.<dns.domain>`, or a domain on the corporate allow-list. Provide the GPO-served IP from the warning so they can locate the policy.
2. **Use IP-based access** in the meantime — point your tools directly at the cluster's API endpoint or service IPs, skipping name resolution. Run `windsor get config workstation.address` for the workstation IP and consult `windsor get services` for the per-service IPs.
3. **Use WSL2 with its own resolver** for development — WSL2 distros do not inherit Windows NRPT and can use systemd-resolved drop-ins directly (see *WSL2 doesn't see Windows NRPT* below).

You can confirm the effective rule yourself:

```powershell
Get-DnsClientNrptPolicy -Effective | Where-Object { $_.Namespace -eq '.<dns.domain>' }
```

If the `NameServers` field shows an IP other than the one in `workstation.dns.address`, a GPO is winning.

### Browser DNS-over-HTTPS shadows the system resolver

**Symptom:** `curl https://dns.local.test` works; the same URL in Firefox/Chrome/Edge fails with `NXDOMAIN` or `DNS_PROBE_FINISHED_NXDOMAIN`.

The browser bypasses the system resolver to query Cloudflare/Google/NextDNS over HTTPS. Your per-domain Windsor rule never gets consulted.

Disable DoH for development:

- **Firefox** → Settings → Privacy & Security → DNS over HTTPS → **Off**.
- **Chrome** → `chrome://settings/security` → Use secure DNS → **Off**.
- **Edge** → `edge://settings/privacy` → Use secure DNS → **Off**.
- **Safari** doesn't enable DoH by default — no action needed.

If you can't disable DoH globally, add `*.<dns.domain>` to the browser's NRPT exclusions where supported, or use `curl` / a non-browser client for the affected service.

### Containers don't share the host's resolver

**Symptom:** `*.<dns.domain>` resolves from the host shell but not from inside a container running on the host.

`/etc/resolv.conf` is baked into the container at run time and is not your host's resolv.conf. Windsor's per-domain rule never propagates.

**Docker:**

```bash
docker run --dns <workstation.dns.address> ...
```

Or in `docker-compose.yml`:

```yaml
services:
  myservice:
    dns:
      - <workstation.dns.address>
```

**Kubernetes pods:** CoreDNS forwarding is usually wired by `../core`. If a pod can't resolve `*.<dns.domain>`, the issue is the cluster's CoreDNS config, not the container's `/etc/resolv.conf`.

### VPN clients overwrite DNS

**Symptom:** DNS works on a fresh boot. After connecting to corporate VPN, `*.<dns.domain>` stops resolving.

Cisco AnyConnect, GlobalProtect, ZScaler, NetExtender, and others rewrite the system resolver, inject NRPT rules, or take over the DNS interface entirely. They typically don't respect per-domain rules installed before the connection.

**Options:**

- **Disconnect the VPN while developing.** Simplest and reliable.
- **Configure split-DNS exclusion** in the VPN client for `*.<dns.domain>` — supported by AnyConnect's "Allowed by Trusted Network" mode, GlobalProtect's split-tunnel DNS list, and ZScaler's bypass list. Specifics vary by vendor and policy; ask IT.
- **Re-run `windsor configure network` after connecting** — sometimes the rule survives the VPN's takeover. Not reliable.

### WSL2 doesn't see Windows NRPT

**Symptom:** You configured DNS on the Windows host. `*.<dns.domain>` resolves from PowerShell but not from inside an Ubuntu WSL2 shell.

WSL2 distros are separate Linux machines with their own kernel and `/etc/resolv.conf`. They cannot inherit NRPT rules from the Windows host.

Two approaches:

1. **Run `windsor configure network` inside the WSL2 distro** — use the Linux systemd-resolved drop-in path (see *DNS on non-systemd Linux* above if your WSL distro doesn't ship resolved).

2. **Pin a static resolv.conf** — disable WSL's auto-generated resolv.conf and manage it yourself:

   ```bash
   # In /etc/wsl.conf:
   [network]
   generateResolvConf = false
   ```

   Then create `/etc/resolv.conf` with `nameserver <workstation.dns.address>`. Restart WSL with `wsl --shutdown` from PowerShell.

### Verify DNS the right way on macOS

**Symptom:** `dig dns.<domain>` or `nslookup dns.<domain>` reports a failure even when browsers and `curl` work fine.

The macOS resolver framework (`scutil --dns`) routes per-domain queries through `/etc/resolver/<domain>`. `dig` and `nslookup` go straight to UDP/53 against the system resolver and skip the per-domain rules entirely.

Use macOS-native tools to verify:

```bash
# Confirm Windsor's resolver entry is registered.
scutil --dns | grep -A4 "<dns.domain>"

# Confirm a name resolves through the per-domain rule.
dscacheutil -q host -a name dns.<dns.domain>
```

### DNS resolves but the service is unreachable

**Symptom:** `host dns.<dns.domain>` returns the right IP, but `curl` against the same name times out.

DNS resolution and reachability are separate concerns. Resolution succeeded — the failure is upstream: the route, the firewall, or the container is the problem, not DNS.

Diagnose by separating the two:

```bash
# Resolution check (DNS only).
host dns.<dns.domain>

# Reachability check (network only — uses the resolved IP).
ping -c 3 <ip-from-host-output>
```

If `ping` fails, the issue is one of:

- **On VM-backed runtimes (colima):** missing/stale host route, or missing in-VM iptables FORWARD. Re-run `windsor configure network` from an elevated shell.
- **On any runtime:** the cluster service isn't running, the port mapping is wrong, or a host firewall is blocking. Check `windsor get services` and `windsor get pods`.

## Networking

### Persistent routes without systemd-networkd

**Symptom:** Your host route to the Windsor VM works immediately after `windsor configure network` but disappears after reboot. Stderr showed `⚠ host route to <address> installed but will not survive reboot on this system`.

Windsor relies on systemd-networkd or NetworkManager to make host routes survive reboots. Without either, the route is ephemeral.

Pick one of these:

- **Install systemd-networkd** (Ubuntu/Debian/Fedora):

  ```bash
  sudo systemctl enable --now systemd-networkd
  windsor configure network
  ```

- **Install NetworkManager** (`networkmanager` package on most distros; default on Fedora Workstation, Ubuntu Desktop).

- **Re-run `windsor configure network` after every reboot.** Acceptable for short-lived dev cycles; awkward for laptops you reboot often. Wrap in a user systemd unit or `.bashrc` shim if you need it automated.

- **Add the route manually to your distro's persistent-route mechanism.** Examples: `/etc/network/interfaces` (ifupdown), `nmcli connection modify` (NetworkManager CLI), `/etc/sysconfig/network-scripts/route-<iface>` (older RHEL). Specifics depend on your distro.

### Stale routes after `workstation.address` changed

**Symptom:** The cluster appears to start, but nothing reaches it. The host's routing table contains a route pointing at an IP that nothing answers on.

A previous Windsor session installed a host route for the old `workstation.address`. When the VM came back up on a different IP, the stale route was never removed.

The clean recovery is `windsor down --clean`, which removes the route via the revert path. If state was deleted manually outside Windsor (or revert never ran), remove the stale route by hand:

- **macOS:** `sudo route delete -net <old-cidr>`
- **Linux:** `sudo ip route del <old-cidr>`
- **Windows (Administrator PowerShell):** `Remove-NetRoute -DestinationPrefix <old-cidr> -Confirm:$false`

Then re-run `windsor configure network` to install the route for the current address.

## Workstation lifecycle

### `windsor up` hangs at "waiting for kustomization"

**Symptom:** `windsor up --wait` (or any wait-equivalent step) sits indefinitely at a "waiting for kustomization …" line.

Almost always a Talos or Flux readiness issue underneath, not a Windsor bug. The kustomization is reconciling a resource that's stuck.

Diagnose:

```bash
# What's the cluster doing right now?
kubectl get pods -A
kubectl get kustomizations -A

# Which resource is the kustomization waiting on?
kubectl describe kustomization <name> -n flux-system
```

Common root causes: a node hasn't joined yet (Talos config issue), an image pull is failing (registry auth), a CRD hasn't applied (ordering), or a HelmRelease is in a bad reconciliation loop.

If you're stuck, `Ctrl-C` is safe — `windsor up` is idempotent. Fix the underlying resource and re-run.

### "another windsor operation is in progress"

**Symptom:** `windsor up`, `down`, or `apply` immediately exits with `another windsor operation is in progress`.

Windsor holds a per-context stack lock to prevent concurrent runs from corrupting state. The lock file lives under `.windsor/locks/` in the project root.

**If you know no other Windsor process is running** (previous run crashed, hit `Ctrl-C` at the wrong moment, etc.):

```bash
windsor unlock
```

This removes the lock file. **Do not run this while another Windsor process is actually running** — it will let two operations collide and you'll end up with state divergence.

To check whether another process really holds the lock, inspect the lock file:

```bash
cat .windsor/locks/<context>.lock
```

The file contains the PID and start time of the holder.

### "error acquiring the state lock"

**Symptom:** `windsor apply`, `windsor up`, or `windsor destroy` errors with `error acquiring the state lock` and a long Terraform error message.

This is Terraform's lock, not Windsor's stack lock. Terraform holds a lock on its state file (or the remote backend) for the duration of an apply.

**Common causes:**

- A previous Terraform process exited without releasing the lock (crashed, killed via `kill -9`).
- A teammate is currently running `terraform apply` or `windsor apply` against the same remote state.
- Network flakiness with the backend (S3, azurerm) made the lock release fail.

**Recovery:**

- If you're certain no other process holds it, re-run with a lock timeout:

  ```bash
  windsor apply --lock-timeout 30s
  ```

  Terraform retries until the timeout, then fails clearly.

- If the lock is genuinely orphaned, follow the Terraform unlock instructions Terraform itself prints (the error message names the `lock ID` to use).

### "OCI artifact authentication required"

**Symptom:** `windsor init` or `windsor up` fails fetching a blueprint or facet with `OCI artifact authentication required` or `401 Unauthorized`.

The blueprint or facet lives behind a private OCI registry. Windsor uses the system credential store (Docker keychain on macOS, gnome-keyring/kwallet on Linux, Windows Credential Manager on Windows) to authenticate.

Make sure you've logged in to the registry the artifact comes from:

```bash
# Most common: ghcr.io for windsorcli/core forks.
echo $GITHUB_TOKEN | docker login ghcr.io -u <your-username> --password-stdin

# Or other registries.
docker login <registry-host>
```

Then re-run the failing Windsor command. The credential lookup is read on each invocation, so no Windsor restart is needed.

If you don't want to use the keychain integration, `windsor up --no-cache` (or `windsor init --no-cache`) forces a fresh artifact pull and surfaces the auth error earlier.

### "Trusted directory" errors

**Symptom:** `windsor up`, `apply`, or `env` exits with `not in a trusted directory. If you are in a Windsor project, run 'windsor init' to approve`.

Windsor only operates inside directories you have explicitly trusted by running `windsor init`. This protects against environment-injection attacks from Windsor configs in untrusted repositories.

If this is a Windsor project you actually want to use, run:

```bash
windsor init
```

This adds the current directory to `$HOME/.config/windsor/.trusted`. Subsequent commands in this directory tree will work.

If this is *not* a Windsor project — for example, you're in a directory that happens to contain a `windsor.yaml` from a different tool — back out and ignore the file. Do not run `windsor init` blindly in directories whose contents you haven't reviewed.

See [Trusted Folders](../security/trusted-folders.md) for the full background on this mechanism.

<!-- Footer Start -->

<div>
  {{ footer('Blueprint Testing', '../testing/index.html', 'Hello, World!', '../../tutorial/hello-world/index.html') }}
</div>

<script>
  document.getElementById('previousButton').addEventListener('click', function() {
    window.location.href = '../testing/index.html';
  });
  document.getElementById('nextButton').addEventListener('click', function() {
    window.location.href = '../../tutorial/hello-world/index.html';
  });
</script>

<!-- Footer End -->

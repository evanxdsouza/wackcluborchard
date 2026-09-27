# Install

There are two paths, and picking the wrong one causes most install problems.

- **A bare Linux box with nothing on it** → the one-command installer. It brings its own k3s.
- **A cluster you already operate** → the Helm chart. Do *not* run the installer
  on an existing cluster; it would install k3s underneath you (it refuses when it
  can see one).

## A bare Linux box

```bash
sudo bash -c "$(curl -fsSL https://raw.githubusercontent.com/evanxdsouza/wackcluborchard/main/deploy/install.sh)"
```

It installs k3s, Traefik with the Gateway API, CloudNativePG, cert-manager, a
`zot` registry for the images it builds, and Wack Club Orchard itself. Everything it
generates lands in `/etc/wackcluborchard/values.yaml`; edit that file and run
`wackcluborchard-update` to apply changes.

Run it that way round, not `curl … | sudo bash`: the installer asks questions,
and piping it into `bash` leaves them nowhere to read your answer from. To read
it first:

```bash
curl -fsSL https://raw.githubusercontent.com/evanxdsouza/wackcluborchard/main/deploy/install.sh -o wackcluborchard-install.sh
less wackcluborchard-install.sh
sudo bash wackcluborchard-install.sh
```

**amd64 only**, and **on a real machine or VM**, not inside a Docker container
(k3s cannot nest its overlayfs snapshotter there).

### Modes

| Mode | For | TLS | Ports |
| --- | --- | --- | --- |
| `public` | A routable IP, DNS pointed at it | Let's Encrypt over HTTP-01 | 80 (challenges) and 443 |
| `tunnel` | Behind NAT or CGNAT, via Cloudflare Tunnel | Cloudflare terminates it | none bound on the host |
| `lan` | LAN or Tailscale only | self-signed | 80/443, or `--http-port`/`--https-port` |

Port 80 in `public` mode cannot be negotiated: ACME fixes the HTTP-01 challenge
at port 80, and every new app hostname and every renewal needs it again. If
something else owns 80/443, use `tunnel`, or `lan` with custom ports.

### What it asks

Your **domain** and an **email for Let's Encrypt** (public mode), the traffic
**mode**, and whether updates are **manual** (`wackcluborchard-update`) or **automatic**
(a nightly systemd timer). Architecture, memory, disk, ports and NAT are detected.
It derives `apps.<domain>` for apps and `mcp.<domain>` for the MCP endpoint, and
checks the MCP name resolves before relying on it; if not, it offers to check
again, use another name, or skip (turn it on later with
`sudo wackcluborchardctl mcp enable --apply`).

### Unattended

```bash
WACKCLUBORCHARD_ASSUME_YES=true \
WACKCLUBORCHARD_DOMAIN=wackcluborchard.example.com \
WACKCLUBORCHARD_ACME_EMAIL=me@example.com \
WACKCLUBORCHARD_INGRESS_MODE=public \
WACKCLUBORCHARD_UPDATE_POLICY=manual \
  sudo -E bash install.sh
```

`--yes` skips the questions, not the requirements: a missing MCP record leaves
the MCP off rather than prompting.

### Sizing

| RAM | Concurrent builds |
| --- | --- |
| under 8Gi | 1 |
| 8 to 16Gi | 2 |
| over 16Gi | 3 |

Build cache and registry each take a fifth of free disk, capped at 60G and 50G.
The floor is 4Gi RAM and 20G disk; 8Gi and 60G is comfortable.

### Over Tailscale

`lan` mode is the right one for a box you reach over Tailscale. The installer
answers on every address the machine had, its hostname and `.local` name, and
`wackcluborchard.<ip>.sslip.io`. Add MagicDNS names it could not guess:

```bash
sudo wackcluborchardctl hostname add mango --apply
sudo wackcluborchardctl hostname add mango.tailnet-name.ts.net --apply
```

## An existing cluster

```bash
helm install wackcluborchard ./deploy/helm/wackcluborchard -n wackcluborchard --create-namespace -f my-values.yaml
```

| You bring | Needed for |
| --- | --- |
| CloudNativePG operator | Every database users create |
| Traefik with the Gateway API | App routing (`HTTPRoute` via a shared `Gateway`) and the auth wall middleware |
| cert-manager | TLS, plus a DNS-01 `ClusterIssuer` for the wildcard app certificate |
| A default StorageClass | Control-plane state, database volumes, app volumes, the registry |
| An ingress controller | Custom-domain Ingresses for user apps |
| metrics-server | CPU and memory charts |
| gVisor / Kata RuntimeClasses | Optional, sandboxed tenant pods, builds and Greenhouse sandboxes |

A minimal values file:

```yaml
ingress:
  host: wackcluborchard.example.com
gateway:
  http:
    appDomain: apps.example.com
tenant:
  ingressClass: traefik
  certClusterIssuer: letsencrypt-prod
signupMode: invite
server:
  env:
    - name: FRONTEND_URL
      value: https://wackcluborchard.example.com
```

Every variable the server reads is in [Configuration](../reference/configuration.md).

## Claiming the instance

A fresh install has no superadmin. On first boot the server mints a
**single-use setup token**, valid for 24 hours, and logs the URL that redeems it.
Sign up with the account that should own the instance, then open the link.

```bash
sudo wackcluborchardctl claim                         # installer boxes
kubectl -n wackcluborchard exec deploy/wackcluborchard-server -- wackcluborchard-server admin claim --data /data
```

The link is the credential: single use, and it makes you superadmin. **Add a
passkey or password straight after**; if you lock yourself out, `wackcluborchardctl claim`
mints a recovery link that signs in as the superadmin.

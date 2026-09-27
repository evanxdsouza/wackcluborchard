#!/usr/bin/env bash
# Wack Club Orchard installer: a bare Linux box to a running platform.
#
#   sudo bash -c "$(curl -fsSL https://raw.githubusercontent.com/evanxdsouza/wackcluborchard/main/deploy/install.sh)"
#
# It installs k3s (with Traefik and the Gateway API), cert-manager,
# CloudNativePG, a zot registry and Wack Club Orchard. Do NOT run it on a cluster you
# already operate: it installs k3s underneath you. Use the Helm chart there.
#
# Every prompt has an environment variable (see --help), so it can run
# unattended or be handed to an agent.

set -euo pipefail

WACKCLUBORCHARD_REPO="${WACKCLUBORCHARD_REPO:-https://github.com/evanxdsouza/wackcluborchard}"
WACKCLUBORCHARD_VERSION="${WACKCLUBORCHARD_VERSION:-main}"
WACKCLUBORCHARD_CHART="${WACKCLUBORCHARD_CHART:-}"
WACKCLUBORCHARD_IMAGE_REGISTRY="${WACKCLUBORCHARD_IMAGE_REGISTRY:-ghcr.io}"
WACKCLUBORCHARD_IMAGE_REPOSITORY="${WACKCLUBORCHARD_IMAGE_REPOSITORY:-evanxdsouza/wackcluborchard}"
WACKCLUBORCHARD_STATE_DIR="${WACKCLUBORCHARD_STATE_DIR:-/etc/wackcluborchard}"
WACKCLUBORCHARD_ASSUME_YES="${WACKCLUBORCHARD_ASSUME_YES:-false}"
WACKCLUBORCHARD_DOMAIN="${WACKCLUBORCHARD_DOMAIN:-}"
WACKCLUBORCHARD_APP_DOMAIN="${WACKCLUBORCHARD_APP_DOMAIN:-}"
WACKCLUBORCHARD_ACME_EMAIL="${WACKCLUBORCHARD_ACME_EMAIL:-}"
WACKCLUBORCHARD_INGRESS_MODE="${WACKCLUBORCHARD_INGRESS_MODE:-}"
WACKCLUBORCHARD_UPDATE_POLICY="${WACKCLUBORCHARD_UPDATE_POLICY:-}"
WACKCLUBORCHARD_TUNNEL_TOKEN="${WACKCLUBORCHARD_TUNNEL_TOKEN:-}"
WACKCLUBORCHARD_MCP="${WACKCLUBORCHARD_MCP:-true}"
WACKCLUBORCHARD_MCP_DOMAIN="${WACKCLUBORCHARD_MCP_DOMAIN:-}"
WACKCLUBORCHARD_NODE_PORT_RANGE="${WACKCLUBORCHARD_NODE_PORT_RANGE:-30000-32767}"
HTTP_PORT="${WACKCLUBORCHARD_HTTP_PORT:-80}"
HTTPS_PORT="${WACKCLUBORCHARD_HTTPS_PORT:-443}"
K3S_VERSION="${K3S_VERSION:-v1.31.4+k3s1}"
GATEWAY_API_VERSION="${GATEWAY_API_VERSION:-v1.2.1}"
CERT_MANAGER_VERSION="${CERT_MANAGER_VERSION:-v1.16.2}"
CNPG_VERSION="${CNPG_VERSION:-1.25.0}"

red=$'\e[31m'; green=$'\e[32m'; yellow=$'\e[33m'; bold=$'\e[1m'; dim=$'\e[2m'; reset=$'\e[0m'
step() { printf '\n%s==>%s %s%s%s\n' "$red" "$reset" "$bold" "$*" "$reset"; }
info() { printf '    %s\n' "$*"; }
warn() { printf '%s  ! %s%s\n' "$yellow" "$*" "$reset"; }
die()  { printf '%s  ✗ %s%s\n' "$red" "$*" "$reset" >&2; exit 1; }
yes()  { [ "$WACKCLUBORCHARD_ASSUME_YES" = "true" ]; }

usage() {
  cat <<EOF
Wack Club Orchard installer

Flags:
  --domain D         dashboard hostname              (WACKCLUBORCHARD_DOMAIN)
  --app-domain D     wildcard parent for apps        (WACKCLUBORCHARD_APP_DOMAIN, default apps.<domain>)
  --email E          Let's Encrypt contact           (WACKCLUBORCHARD_ACME_EMAIL)
  --mode M           public | tunnel | lan           (WACKCLUBORCHARD_INGRESS_MODE)
  --updates U        manual | automatic              (WACKCLUBORCHARD_UPDATE_POLICY)
  --mcp BOOL         publish the MCP endpoint        (WACKCLUBORCHARD_MCP)
  --mcp-domain D     MCP hostname                    (WACKCLUBORCHARD_MCP_DOMAIN, default mcp.<domain>)
  --http-port P      lan mode only                   (WACKCLUBORCHARD_HTTP_PORT)
  --https-port P     lan mode only                   (WACKCLUBORCHARD_HTTPS_PORT)
  --tunnel-token T   Cloudflare Tunnel token         (WACKCLUBORCHARD_TUNNEL_TOKEN)
  --yes              never prompt                    (WACKCLUBORCHARD_ASSUME_YES=true)

Also: WACKCLUBORCHARD_VERSION WACKCLUBORCHARD_CHART WACKCLUBORCHARD_IMAGE_REGISTRY WACKCLUBORCHARD_IMAGE_REPOSITORY
WACKCLUBORCHARD_STATE_DIR WACKCLUBORCHARD_NODE_PORT_RANGE K3S_VERSION GATEWAY_API_VERSION
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --domain) WACKCLUBORCHARD_DOMAIN="$2"; shift ;;
    --app-domain) WACKCLUBORCHARD_APP_DOMAIN="$2"; shift ;;
    --email) WACKCLUBORCHARD_ACME_EMAIL="$2"; shift ;;
    --mode) WACKCLUBORCHARD_INGRESS_MODE="$2"; shift ;;
    --updates) WACKCLUBORCHARD_UPDATE_POLICY="$2"; shift ;;
    --mcp) WACKCLUBORCHARD_MCP="$2"; shift ;;
    --mcp-domain) WACKCLUBORCHARD_MCP_DOMAIN="$2"; shift ;;
    --http-port) HTTP_PORT="$2"; CUSTOM_PORTS=1; shift ;;
    --https-port) HTTPS_PORT="$2"; CUSTOM_PORTS=1; shift ;;
    --tunnel-token) WACKCLUBORCHARD_TUNNEL_TOKEN="$2"; shift ;;
    --yes|-y) WACKCLUBORCHARD_ASSUME_YES=true ;;
    -h|--help) usage; exit 0 ;;
    --) ;;
    *) die "unknown flag $1 (see --help)" ;;
  esac
  shift
done

# Prompts read from the terminal, not stdin: stdin may be the script.
ask() {
  local prompt="$1" default="${2:-}" answer
  if yes; then echo "$default"; return; fi
  if [ ! -r /dev/tty ]; then die "no terminal to ask \"$prompt\"; set the environment variable or pass --yes"; fi
  if [ -n "$default" ]; then printf '  %s %s[%s]%s ' "$prompt" "$dim" "$default" "$reset" >/dev/tty
  else printf '  %s ' "$prompt" >/dev/tty; fi
  read -r answer </dev/tty || true
  echo "${answer:-$default}"
}

# ---------------------------------------------------------------- checks

step "Checking this machine"
[ "$(id -u)" = "0" ] || die "run as root: sudo bash -c \"\$(curl -fsSL …/install.sh)\""
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) info "architecture: amd64" ;;
  *) die "amd64 only: image builds target linux/amd64, so apps built here could not start on $arch" ;;
esac
if [ -f /.dockerenv ] || grep -qa 'docker\|containerd' /proc/1/cgroup 2>/dev/null; then
  die "this looks like a container; k3s cannot nest its overlayfs snapshotter there. Use a real machine or VM."
fi
if command -v kubectl >/dev/null 2>&1 && kubectl get nodes >/dev/null 2>&1 && ! command -v k3s >/dev/null 2>&1; then
  die "a Kubernetes cluster is already reachable here. Use the Helm chart on existing clusters: $WACKCLUBORCHARD_REPO/tree/main/deploy/helm/wackcluborchard"
fi
mem_mi=$(( $(awk '/MemTotal/ {print $2}' /proc/meminfo) / 1024 ))
disk_g=$(df -BG --output=avail / | tail -1 | tr -dc 0-9)
cpus=$(nproc)
info "memory: ${mem_mi}Mi · free disk: ${disk_g}G · cpus: ${cpus}"
[ "$mem_mi" -ge 3800 ] || die "need at least 4Gi of RAM (8Gi is comfortable)"
[ "$disk_g" -ge 20 ] || die "need at least 20G of free disk (60G is comfortable)"

if [ "$mem_mi" -lt 8192 ]; then build_slots=1; elif [ "$mem_mi" -le 16384 ]; then build_slots=2; else build_slots=3; fi
cache_g=$(( disk_g / 5 )); [ "$cache_g" -gt 60 ] && cache_g=60
registry_g=$(( disk_g / 5 )); [ "$registry_g" -gt 50 ] && registry_g=50
info "sizing: ${build_slots} concurrent build(s), ${cache_g}G build cache, ${registry_g}G registry"

port_owner() { ss -Hltnp "sport = :$1" 2>/dev/null | sed -n 's/.*users:(("\([^"]*\)".*/\1/p' | head -1; }
owner80="$(port_owner 80 || true)"; owner443="$(port_owner 443 || true)"
[ -n "$owner80" ] && info "port 80 is held by $owner80"
[ -n "$owner443" ] && info "port 443 is held by $owner443"

public_ip="$(curl -fsS --max-time 5 https://api.ipify.org 2>/dev/null || true)"
local_ips="$(ip -4 -o addr show scope global 2>/dev/null | awk '{print $4}' | cut -d/ -f1 | tr '\n' ' ')"
behind_nat=true
for ip in $local_ips; do [ "$ip" = "$public_ip" ] && behind_nat=false; done
info "addresses: ${local_ips:-none}${public_ip:+· public $public_ip}"
$behind_nat && info "this box is behind NAT (its public address is not on an interface)"

# ---------------------------------------------------------------- questions

step "A few questions"
if [ -z "$WACKCLUBORCHARD_INGRESS_MODE" ]; then
  suggested=public; $behind_nat && suggested=tunnel
  { [ -n "$owner80" ] || [ -n "$owner443" ]; } && [ "$suggested" = public ] && suggested=lan
  info "public: routable IP, Let's Encrypt over HTTP-01 · tunnel: behind NAT via Cloudflare Tunnel · lan: LAN or Tailscale only, self-signed"
  WACKCLUBORCHARD_INGRESS_MODE="$(ask "How does traffic reach this box? (public/tunnel/lan)" "$suggested")"
fi
case "$WACKCLUBORCHARD_INGRESS_MODE" in public|tunnel|lan) ;; *) die "mode must be public, tunnel or lan" ;; esac
if [ "${CUSTOM_PORTS:-}" = 1 ] && [ "$WACKCLUBORCHARD_INGRESS_MODE" != lan ]; then
  die "--http-port/--https-port only work in lan mode: public needs port 80 for HTTP-01, tunnel binds no host ports"
fi
if [ "$WACKCLUBORCHARD_INGRESS_MODE" != tunnel ]; then
  if [ "$WACKCLUBORCHARD_INGRESS_MODE" = public ] && [ -n "$owner80" ]; then die "public mode needs port 80 (held by $owner80): ACME fixes HTTP-01 at port 80. Free it or use tunnel mode."; fi
  if [ "$HTTP_PORT" = 80 ] && [ -n "$owner80" ]; then die "port 80 is held by $owner80; free it, or use --mode lan --http-port 8080"; fi
  if [ "$HTTPS_PORT" = 443 ] && [ -n "$owner443" ]; then die "port 443 is held by $owner443; free it, or use --mode lan --https-port 8443"; fi
fi

if [ "$WACKCLUBORCHARD_INGRESS_MODE" = lan ]; then
  first_ip="$(echo "$local_ips" | awk '{print $1}')"
  WACKCLUBORCHARD_DOMAIN="${WACKCLUBORCHARD_DOMAIN:-$first_ip}"
  WACKCLUBORCHARD_APP_DOMAIN="${WACKCLUBORCHARD_APP_DOMAIN:-apps.${first_ip}.sslip.io}"
else
  [ -n "$WACKCLUBORCHARD_DOMAIN" ] || WACKCLUBORCHARD_DOMAIN="$(ask "Domain for the dashboard (e.g. wackcluborchard.example.com):" "")"
  [ -n "$WACKCLUBORCHARD_DOMAIN" ] || die "a domain is required outside lan mode"
  WACKCLUBORCHARD_APP_DOMAIN="${WACKCLUBORCHARD_APP_DOMAIN:-apps.$WACKCLUBORCHARD_DOMAIN}"
fi
if [ "$WACKCLUBORCHARD_INGRESS_MODE" = public ] && [ -z "$WACKCLUBORCHARD_ACME_EMAIL" ]; then
  WACKCLUBORCHARD_ACME_EMAIL="$(ask "Email for Let's Encrypt:" "")"
  [ -n "$WACKCLUBORCHARD_ACME_EMAIL" ] || die "Let's Encrypt needs a contact email"
fi
if [ "$WACKCLUBORCHARD_INGRESS_MODE" = tunnel ] && [ -z "$WACKCLUBORCHARD_TUNNEL_TOKEN" ]; then
  WACKCLUBORCHARD_TUNNEL_TOKEN="$(ask "Cloudflare Tunnel token:" "")"
  [ -n "$WACKCLUBORCHARD_TUNNEL_TOKEN" ] || die "tunnel mode needs a Cloudflare Tunnel token"
fi
[ -n "$WACKCLUBORCHARD_UPDATE_POLICY" ] || WACKCLUBORCHARD_UPDATE_POLICY="$(ask "Updates: manual (wackcluborchard-update) or automatic (nightly)?" "manual")"

# The MCP hostname must resolve before its certificate can be issued; a
# certificate that cannot be issued fails quietly, so check now.
if [ "$WACKCLUBORCHARD_MCP" = true ] && [ "$WACKCLUBORCHARD_INGRESS_MODE" = public ]; then
  WACKCLUBORCHARD_MCP_DOMAIN="${WACKCLUBORCHARD_MCP_DOMAIN:-mcp.$WACKCLUBORCHARD_DOMAIN}"
  while ! getent hosts "$WACKCLUBORCHARD_MCP_DOMAIN" >/dev/null 2>&1; do
    if yes; then warn "$WACKCLUBORCHARD_MCP_DOMAIN does not resolve; skipping the MCP. Enable later: sudo wackcluborchardctl mcp enable --apply"; WACKCLUBORCHARD_MCP=false; break; fi
    printf '\n  %s! %s does not resolve yet.%s\n' "$yellow" "$WACKCLUBORCHARD_MCP_DOMAIN" "$reset" >/dev/tty
    printf '  That is where AI agents connect over MCP. Its certificate is issued\n  over HTTP-01, so it cannot be obtained until the name points here.\n\n' >/dev/tty
    printf '  1. I have added the record now, check again\n  2. use a different hostname\n  3. skip the MCP for now, I will turn it on later\n' >/dev/tty
    choice="$(ask "What would you like to do?" "1")"
    case "$choice" in
      2) WACKCLUBORCHARD_MCP_DOMAIN="$(ask "MCP hostname:" "")" ;;
      3) WACKCLUBORCHARD_MCP=false; break ;;
    esac
  done
elif [ "$WACKCLUBORCHARD_MCP" = true ] && [ "$WACKCLUBORCHARD_INGRESS_MODE" = tunnel ]; then
  WACKCLUBORCHARD_MCP_DOMAIN="${WACKCLUBORCHARD_MCP_DOMAIN:-mcp.$WACKCLUBORCHARD_DOMAIN}"
fi

echo
info "mode:        $WACKCLUBORCHARD_INGRESS_MODE"
info "dashboard:   $WACKCLUBORCHARD_DOMAIN"
info "apps:        *.$WACKCLUBORCHARD_APP_DOMAIN"
[ "$WACKCLUBORCHARD_MCP" = true ] && info "mcp:         ${WACKCLUBORCHARD_MCP_DOMAIN:-$WACKCLUBORCHARD_DOMAIN/mcp}"
info "updates:     $WACKCLUBORCHARD_UPDATE_POLICY"
if [ "$WACKCLUBORCHARD_INGRESS_MODE" = public ]; then
  echo
  info "${bold}DNS records to create (A records to ${public_ip:-the public IP of this box}):${reset}"
  info "  $WACKCLUBORCHARD_DOMAIN"
  info "  *.$WACKCLUBORCHARD_APP_DOMAIN"
  [ "$WACKCLUBORCHARD_MCP" = true ] && info "  $WACKCLUBORCHARD_MCP_DOMAIN"
fi
if ! yes; then
  binds=""
  [ "$WACKCLUBORCHARD_INGRESS_MODE" != tunnel ] && binds=" and binds ports $HTTP_PORT/$HTTPS_PORT"
  go="$(ask "This installs k3s system-wide${binds}. Continue? (y/n)" "y")"
  case "$go" in y|Y|yes) ;; *) die "stopped; nothing was installed" ;; esac
fi

# ---------------------------------------------------------------- k3s

step "Installing k3s $K3S_VERSION"
if ! command -v k3s >/dev/null 2>&1; then
  curl -sfL https://get.k3s.io | INSTALL_K3S_VERSION="$K3S_VERSION" sh -s - server \
    --write-kubeconfig-mode 0600 \
    --service-node-port-range "$WACKCLUBORCHARD_NODE_PORT_RANGE"
else
  info "k3s is already installed"
fi
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
for _ in $(seq 60); do k3s kubectl get nodes >/dev/null 2>&1 && break; sleep 2; done
kubectl() { k3s kubectl "$@"; }

if ! command -v helm >/dev/null 2>&1; then
  step "Installing helm"
  curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
fi
if ! command -v yq >/dev/null 2>&1; then
  curl -fsSL -o /usr/local/bin/yq "https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64" && chmod +x /usr/local/bin/yq
fi

step "Gateway API $GATEWAY_API_VERSION and Traefik"
kubectl apply -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/$GATEWAY_API_VERSION/standard-install.yaml" >/dev/null
traefik_service="LoadBalancer"; [ "$WACKCLUBORCHARD_INGRESS_MODE" = tunnel ] && traefik_service="ClusterIP"
cat <<EOF | kubectl apply -f - >/dev/null
apiVersion: helm.cattle.io/v1
kind: HelmChartConfig
metadata:
  name: traefik
  namespace: kube-system
spec:
  valuesContent: |-
    providers:
      kubernetesGateway:
        enabled: true
      kubernetesCRD:
        allowCrossNamespace: true
    gateway:
      enabled: false
    service:
      type: $traefik_service
    ports:
      web:
        exposedPort: $HTTP_PORT
      websecure:
        exposedPort: $HTTPS_PORT
EOF

if [ "$WACKCLUBORCHARD_INGRESS_MODE" != tunnel ]; then
  step "cert-manager $CERT_MANAGER_VERSION"
  helm repo add jetstack https://charts.jetstack.io >/dev/null 2>&1 || true
  helm repo update >/dev/null
  helm upgrade --install cert-manager jetstack/cert-manager -n cert-manager --create-namespace \
    --version "$CERT_MANAGER_VERSION" --set crds.enabled=true --wait >/dev/null
  if [ "$WACKCLUBORCHARD_INGRESS_MODE" = public ]; then
    cat <<EOF | kubectl apply -f - >/dev/null
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: $WACKCLUBORCHARD_ACME_EMAIL
    privateKeySecretRef: { name: letsencrypt-prod }
    solvers:
      - http01:
          ingress: { ingressClassName: traefik }
EOF
  else
    cat <<EOF | kubectl apply -f - >/dev/null
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned
spec:
  selfSigned: {}
EOF
  fi
fi

step "CloudNativePG $CNPG_VERSION"
helm repo add cnpg https://cloudnative-pg.github.io/charts >/dev/null 2>&1 || true
helm repo update >/dev/null
helm upgrade --install cnpg cnpg/cloudnative-pg -n cnpg-system --create-namespace --version "0.23.0" --wait >/dev/null

if [ "$WACKCLUBORCHARD_INGRESS_MODE" = tunnel ]; then
  step "Cloudflare Tunnel"
  kubectl create namespace cloudflared --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  kubectl -n cloudflared create secret generic tunnel --from-literal=token="$WACKCLUBORCHARD_TUNNEL_TOKEN" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  cat <<'EOF' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: { name: cloudflared, namespace: cloudflared }
spec:
  replicas: 2
  selector: { matchLabels: { app: cloudflared } }
  template:
    metadata: { labels: { app: cloudflared } }
    spec:
      containers:
        - name: cloudflared
          image: cloudflare/cloudflared:2024.12.2
          args: ["tunnel", "--no-autoupdate", "run", "--token", "$(TOKEN)"]
          env: [{ name: TOKEN, valueFrom: { secretKeyRef: { name: tunnel, key: token } } }]
EOF
  info "point the tunnel's public hostnames at http://traefik.kube-system.svc.cluster.local:$HTTP_PORT"
fi

# ---------------------------------------------------------------- wackcluborchard

step "Wack Club Orchard"
mkdir -p "$WACKCLUBORCHARD_STATE_DIR" /usr/local/share/wackcluborchard
if [ -z "$WACKCLUBORCHARD_CHART" ]; then
  tmp="$(mktemp -d)"
  curl -fsSL "$WACKCLUBORCHARD_REPO/archive/$WACKCLUBORCHARD_VERSION.tar.gz" | tar -xz -C "$tmp"
  rm -rf /usr/local/share/wackcluborchard/chart
  cp -r "$tmp"/*/deploy/helm/wackcluborchard /usr/local/share/wackcluborchard/chart
  WACKCLUBORCHARD_CHART=/usr/local/share/wackcluborchard/chart
fi

scheme=https; issuer=letsencrypt-prod; tls=true
case "$WACKCLUBORCHARD_INGRESS_MODE" in
  lan) scheme=http; issuer=selfsigned; tls=false ;;
  tunnel) tls=false ;;
esac
port_suffix=""
[ "$WACKCLUBORCHARD_INGRESS_MODE" = lan ] && [ "$HTTP_PORT" != 80 ] && port_suffix=":$HTTP_PORT"
extra_hosts=""
if [ "$WACKCLUBORCHARD_INGRESS_MODE" = lan ]; then
  for ip in $local_ips; do extra_hosts="$extra_hosts\"$ip\", \"wackcluborchard.$ip.sslip.io\", "; done
  extra_hosts="$extra_hosts\"$(hostname)\", \"$(hostname).local\""
fi

if [ ! -f "$WACKCLUBORCHARD_STATE_DIR/values.yaml" ]; then
cat > "$WACKCLUBORCHARD_STATE_DIR/values.yaml" <<EOF
# Generated by the Wack Club Orchard installer. Edit, then run wackcluborchard-update.
image:
  repository: $WACKCLUBORCHARD_IMAGE_REGISTRY/$WACKCLUBORCHARD_IMAGE_REPOSITORY
  tag: "$WACKCLUBORCHARD_VERSION"
ingress:
  host: $WACKCLUBORCHARD_DOMAIN
  extraHosts: [$extra_hosts]
  tls: $tls
  certClusterIssuer: $issuer
gateway:
  enabled: $( [ "$WACKCLUBORCHARD_INGRESS_MODE" = public ] && echo true || echo false )
  http:
    appDomain: $WACKCLUBORCHARD_APP_DOMAIN
tenant:
  ingressClass: traefik
  certClusterIssuer: $issuer
registry:
  enabled: true
  storage: ${registry_g}Gi
buildSlots: $build_slots
signupMode: open
mcp:
  enabled: $WACKCLUBORCHARD_MCP
  host: "${WACKCLUBORCHARD_MCP_DOMAIN:-}"
tenantSandbox:
  enabled: false
  runtimeClassName: gvisor
builderSandbox:
  enabled: false
  runtimeClassName: kata
server:
  env:
    - name: FRONTEND_URL
      value: "$scheme://$WACKCLUBORCHARD_DOMAIN$port_suffix"
    - name: INGRESS_MODE
      value: "$WACKCLUBORCHARD_INGRESS_MODE"
    - name: PUBLIC_HTTP_PORT
      value: "$HTTP_PORT"
    - name: PUBLIC_HTTPS_PORT
      value: "$HTTPS_PORT"
    - name: PUBLIC_IP
      value: ""
EOF
  info "wrote $WACKCLUBORCHARD_STATE_DIR/values.yaml"
else
  info "keeping existing $WACKCLUBORCHARD_STATE_DIR/values.yaml"
fi

helm upgrade --install wackcluborchard "$WACKCLUBORCHARD_CHART" -n wackcluborchard --create-namespace \
  -f "$WACKCLUBORCHARD_STATE_DIR/values.yaml" --wait --timeout 10m

# wackcluborchardctl and wackcluborchard-update live on the host
image="$WACKCLUBORCHARD_IMAGE_REGISTRY/$WACKCLUBORCHARD_IMAGE_REPOSITORY:$WACKCLUBORCHARD_VERSION"
kubectl -n wackcluborchard exec deploy/wackcluborchard-server -- cat /usr/local/bin/wackcluborchardctl > /usr/local/bin/wackcluborchardctl && chmod +x /usr/local/bin/wackcluborchardctl
kubectl -n wackcluborchard exec deploy/wackcluborchard-server -- cat /usr/local/bin/wackcluborchard > /usr/local/bin/wackcluborchard && chmod +x /usr/local/bin/wackcluborchard
cat > /usr/local/bin/wackcluborchard-update <<EOF
#!/usr/bin/env bash
set -euo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
exec /usr/local/bin/wackcluborchardctl update
EOF
chmod +x /usr/local/bin/wackcluborchard-update
cat > /etc/profile.d/wackcluborchard.sh <<'EOF'
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
EOF

if [ "$WACKCLUBORCHARD_UPDATE_POLICY" = automatic ]; then
  cat > /etc/systemd/system/wackcluborchard-update.service <<'EOF'
[Unit]
Description=Update Wack Club Orchard
[Service]
Type=oneshot
ExecStart=/usr/local/bin/wackcluborchard-update
EOF
  cat > /etc/systemd/system/wackcluborchard-update.timer <<'EOF'
[Unit]
Description=Nightly Wack Club Orchard update
[Timer]
OnCalendar=*-*-* 04:17:00
RandomizedDelaySec=30m
Persistent=true
[Install]
WantedBy=timers.target
EOF
  systemctl daemon-reload && systemctl enable --now wackcluborchard-update.timer >/dev/null
  info "automatic updates: nightly (systemctl list-timers wackcluborchard-update.timer)"
fi

step "Done"
claim="$(KUBECONFIG=/etc/rancher/k3s/k3s.yaml /usr/local/bin/wackcluborchardctl claim --url "$scheme://$WACKCLUBORCHARD_DOMAIN$port_suffix" 2>/dev/null | tail -1 | tr -d ' ')"
echo
echo "  ${green}Wack Club Orchard is running.${reset}"
echo
echo "  Dashboard:  $scheme://$WACKCLUBORCHARD_DOMAIN$port_suffix"
[ "$WACKCLUBORCHARD_MCP" = true ] && [ -n "${WACKCLUBORCHARD_MCP_DOMAIN:-}" ] && echo "  MCP:        https://$WACKCLUBORCHARD_MCP_DOMAIN/mcp"
echo
echo "  ${bold}Claim link${reset} (single use, valid 24h, signs in as the superadmin; open it yourself):"
echo
echo "    $claim"
echo
echo "  Sign up first, then open the link. Add a passkey or password straight after:"
echo "  the link is spent. Lost it? sudo wackcluborchardctl claim"
[ "$WACKCLUBORCHARD_INGRESS_MODE" = lan ] && echo "  Reached by another name (Tailscale)? sudo wackcluborchardctl hostname add <name> --apply"
echo "  Set a default public IP for TCP/UDP and databases: sudo wackcluborchardctl public-ip set <ip> --apply"
echo

#!/usr/bin/env bash
# Wack Club Orchard installer: a bare Linux box to a running platform.
#
#   sudo bash -c "$(curl -fsSL https://raw.githubusercontent.com/evanxdsouza/wackcluborchard/main/deploy/install.sh)"
#
# It installs k3s (with Traefik and the Gateway API), cert-manager,
# CloudNativePG, a zot registry and Orchard. Do NOT run it on a cluster you
# already operate: it installs k3s underneath you. Use the Helm chart there.
#
# Every prompt has an environment variable (see --help), so it can run
# unattended or be handed to an agent.

set -euo pipefail

ORCHARD_REPO="${ORCHARD_REPO:-https://github.com/evanxdsouza/wackcluborchard}"
ORCHARD_VERSION="${ORCHARD_VERSION:-main}"
ORCHARD_CHART="${ORCHARD_CHART:-}"
ORCHARD_IMAGE_REGISTRY="${ORCHARD_IMAGE_REGISTRY:-ghcr.io}"
ORCHARD_IMAGE_REPOSITORY="${ORCHARD_IMAGE_REPOSITORY:-evanxdsouza/wackcluborchard}"
ORCHARD_STATE_DIR="${ORCHARD_STATE_DIR:-/etc/orchard}"
ORCHARD_ASSUME_YES="${ORCHARD_ASSUME_YES:-false}"
ORCHARD_DOMAIN="${ORCHARD_DOMAIN:-}"
ORCHARD_APP_DOMAIN="${ORCHARD_APP_DOMAIN:-}"
ORCHARD_ACME_EMAIL="${ORCHARD_ACME_EMAIL:-}"
ORCHARD_INGRESS_MODE="${ORCHARD_INGRESS_MODE:-}"
ORCHARD_UPDATE_POLICY="${ORCHARD_UPDATE_POLICY:-}"
ORCHARD_TUNNEL_TOKEN="${ORCHARD_TUNNEL_TOKEN:-}"
ORCHARD_MCP="${ORCHARD_MCP:-true}"
ORCHARD_MCP_DOMAIN="${ORCHARD_MCP_DOMAIN:-}"
ORCHARD_NODE_PORT_RANGE="${ORCHARD_NODE_PORT_RANGE:-30000-32767}"
HTTP_PORT="${ORCHARD_HTTP_PORT:-80}"
HTTPS_PORT="${ORCHARD_HTTPS_PORT:-443}"
K3S_VERSION="${K3S_VERSION:-v1.31.4+k3s1}"
GATEWAY_API_VERSION="${GATEWAY_API_VERSION:-v1.2.1}"
CERT_MANAGER_VERSION="${CERT_MANAGER_VERSION:-v1.16.2}"
CNPG_VERSION="${CNPG_VERSION:-1.25.0}"

red=$'\e[31m'; green=$'\e[32m'; yellow=$'\e[33m'; bold=$'\e[1m'; dim=$'\e[2m'; reset=$'\e[0m'
step() { printf '\n%s==>%s %s%s%s\n' "$red" "$reset" "$bold" "$*" "$reset"; }
info() { printf '    %s\n' "$*"; }
warn() { printf '%s  ! %s%s\n' "$yellow" "$*" "$reset"; }
die()  { printf '%s  ✗ %s%s\n' "$red" "$*" "$reset" >&2; exit 1; }
yes()  { [ "$ORCHARD_ASSUME_YES" = "true" ]; }

usage() {
  cat <<EOF
Wack Club Orchard installer

Flags:
  --domain D         dashboard hostname              (ORCHARD_DOMAIN)
  --app-domain D     wildcard parent for apps        (ORCHARD_APP_DOMAIN, default apps.<domain>)
  --email E          Let's Encrypt contact           (ORCHARD_ACME_EMAIL)
  --mode M           public | tunnel | lan           (ORCHARD_INGRESS_MODE)
  --updates U        manual | automatic              (ORCHARD_UPDATE_POLICY)
  --mcp BOOL         publish the MCP endpoint        (ORCHARD_MCP)
  --mcp-domain D     MCP hostname                    (ORCHARD_MCP_DOMAIN, default mcp.<domain>)
  --http-port P      lan mode only                   (ORCHARD_HTTP_PORT)
  --https-port P     lan mode only                   (ORCHARD_HTTPS_PORT)
  --tunnel-token T   Cloudflare Tunnel token         (ORCHARD_TUNNEL_TOKEN)
  --yes              never prompt                    (ORCHARD_ASSUME_YES=true)

Also: ORCHARD_VERSION ORCHARD_CHART ORCHARD_IMAGE_REGISTRY ORCHARD_IMAGE_REPOSITORY
ORCHARD_STATE_DIR ORCHARD_NODE_PORT_RANGE K3S_VERSION GATEWAY_API_VERSION
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --domain) ORCHARD_DOMAIN="$2"; shift ;;
    --app-domain) ORCHARD_APP_DOMAIN="$2"; shift ;;
    --email) ORCHARD_ACME_EMAIL="$2"; shift ;;
    --mode) ORCHARD_INGRESS_MODE="$2"; shift ;;
    --updates) ORCHARD_UPDATE_POLICY="$2"; shift ;;
    --mcp) ORCHARD_MCP="$2"; shift ;;
    --mcp-domain) ORCHARD_MCP_DOMAIN="$2"; shift ;;
    --http-port) HTTP_PORT="$2"; CUSTOM_PORTS=1; shift ;;
    --https-port) HTTPS_PORT="$2"; CUSTOM_PORTS=1; shift ;;
    --tunnel-token) ORCHARD_TUNNEL_TOKEN="$2"; shift ;;
    --yes|-y) ORCHARD_ASSUME_YES=true ;;
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
  die "a Kubernetes cluster is already reachable here. Use the Helm chart on existing clusters: $ORCHARD_REPO/tree/main/deploy/helm/orchard"
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
if [ -z "$ORCHARD_INGRESS_MODE" ]; then
  suggested=public; $behind_nat && suggested=tunnel
  { [ -n "$owner80" ] || [ -n "$owner443" ]; } && [ "$suggested" = public ] && suggested=lan
  info "public: routable IP, Let's Encrypt over HTTP-01 · tunnel: behind NAT via Cloudflare Tunnel · lan: LAN or Tailscale only, self-signed"
  ORCHARD_INGRESS_MODE="$(ask "How does traffic reach this box? (public/tunnel/lan)" "$suggested")"
fi
case "$ORCHARD_INGRESS_MODE" in public|tunnel|lan) ;; *) die "mode must be public, tunnel or lan" ;; esac
if [ "${CUSTOM_PORTS:-}" = 1 ] && [ "$ORCHARD_INGRESS_MODE" != lan ]; then
  die "--http-port/--https-port only work in lan mode: public needs port 80 for HTTP-01, tunnel binds no host ports"
fi
if [ "$ORCHARD_INGRESS_MODE" != tunnel ]; then
  if [ "$ORCHARD_INGRESS_MODE" = public ] && [ -n "$owner80" ]; then die "public mode needs port 80 (held by $owner80): ACME fixes HTTP-01 at port 80. Free it or use tunnel mode."; fi
  if [ "$HTTP_PORT" = 80 ] && [ -n "$owner80" ]; then die "port 80 is held by $owner80; free it, or use --mode lan --http-port 8080"; fi
  if [ "$HTTPS_PORT" = 443 ] && [ -n "$owner443" ]; then die "port 443 is held by $owner443; free it, or use --mode lan --https-port 8443"; fi
fi

if [ "$ORCHARD_INGRESS_MODE" = lan ]; then
  first_ip="$(echo "$local_ips" | awk '{print $1}')"
  ORCHARD_DOMAIN="${ORCHARD_DOMAIN:-$first_ip}"
  ORCHARD_APP_DOMAIN="${ORCHARD_APP_DOMAIN:-apps.${first_ip}.sslip.io}"
else
  [ -n "$ORCHARD_DOMAIN" ] || ORCHARD_DOMAIN="$(ask "Domain for the dashboard (e.g. orchard.example.com):" "")"
  [ -n "$ORCHARD_DOMAIN" ] || die "a domain is required outside lan mode"
  ORCHARD_APP_DOMAIN="${ORCHARD_APP_DOMAIN:-apps.$ORCHARD_DOMAIN}"
fi
if [ "$ORCHARD_INGRESS_MODE" = public ] && [ -z "$ORCHARD_ACME_EMAIL" ]; then
  ORCHARD_ACME_EMAIL="$(ask "Email for Let's Encrypt:" "")"
  [ -n "$ORCHARD_ACME_EMAIL" ] || die "Let's Encrypt needs a contact email"
fi
if [ "$ORCHARD_INGRESS_MODE" = tunnel ] && [ -z "$ORCHARD_TUNNEL_TOKEN" ]; then
  ORCHARD_TUNNEL_TOKEN="$(ask "Cloudflare Tunnel token:" "")"
  [ -n "$ORCHARD_TUNNEL_TOKEN" ] || die "tunnel mode needs a Cloudflare Tunnel token"
fi
[ -n "$ORCHARD_UPDATE_POLICY" ] || ORCHARD_UPDATE_POLICY="$(ask "Updates: manual (orchard-update) or automatic (nightly)?" "manual")"

# The MCP hostname must resolve before its certificate can be issued; a
# certificate that cannot be issued fails quietly, so check now.
if [ "$ORCHARD_MCP" = true ] && [ "$ORCHARD_INGRESS_MODE" = public ]; then
  ORCHARD_MCP_DOMAIN="${ORCHARD_MCP_DOMAIN:-mcp.$ORCHARD_DOMAIN}"
  while ! getent hosts "$ORCHARD_MCP_DOMAIN" >/dev/null 2>&1; do
    if yes; then warn "$ORCHARD_MCP_DOMAIN does not resolve; skipping the MCP. Enable later: sudo orchardctl mcp enable --apply"; ORCHARD_MCP=false; break; fi
    printf '\n  %s! %s does not resolve yet.%s\n' "$yellow" "$ORCHARD_MCP_DOMAIN" "$reset" >/dev/tty
    printf '  That is where AI agents connect over MCP. Its certificate is issued\n  over HTTP-01, so it cannot be obtained until the name points here.\n\n' >/dev/tty
    printf '  1. I have added the record now, check again\n  2. use a different hostname\n  3. skip the MCP for now, I will turn it on later\n' >/dev/tty
    choice="$(ask "What would you like to do?" "1")"
    case "$choice" in
      2) ORCHARD_MCP_DOMAIN="$(ask "MCP hostname:" "")" ;;
      3) ORCHARD_MCP=false; break ;;
    esac
  done
elif [ "$ORCHARD_MCP" = true ] && [ "$ORCHARD_INGRESS_MODE" = tunnel ]; then
  ORCHARD_MCP_DOMAIN="${ORCHARD_MCP_DOMAIN:-mcp.$ORCHARD_DOMAIN}"
fi

echo
info "mode:        $ORCHARD_INGRESS_MODE"
info "dashboard:   $ORCHARD_DOMAIN"
info "apps:        *.$ORCHARD_APP_DOMAIN"
[ "$ORCHARD_MCP" = true ] && info "mcp:         ${ORCHARD_MCP_DOMAIN:-$ORCHARD_DOMAIN/mcp}"
info "updates:     $ORCHARD_UPDATE_POLICY"
if [ "$ORCHARD_INGRESS_MODE" = public ]; then
  echo
  info "${bold}DNS records to create (A records to ${public_ip:-the public IP of this box}):${reset}"
  info "  $ORCHARD_DOMAIN"
  info "  *.$ORCHARD_APP_DOMAIN"
  [ "$ORCHARD_MCP" = true ] && info "  $ORCHARD_MCP_DOMAIN"
fi
if ! yes; then
  binds=""
  [ "$ORCHARD_INGRESS_MODE" != tunnel ] && binds=" and binds ports $HTTP_PORT/$HTTPS_PORT"
  go="$(ask "This installs k3s system-wide${binds}. Continue? (y/n)" "y")"
  case "$go" in y|Y|yes) ;; *) die "stopped; nothing was installed" ;; esac
fi

# ---------------------------------------------------------------- k3s

step "Installing k3s $K3S_VERSION"
if ! command -v k3s >/dev/null 2>&1; then
  curl -sfL https://get.k3s.io | INSTALL_K3S_VERSION="$K3S_VERSION" sh -s - server \
    --write-kubeconfig-mode 0600 \
    --service-node-port-range "$ORCHARD_NODE_PORT_RANGE"
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
traefik_service="LoadBalancer"; [ "$ORCHARD_INGRESS_MODE" = tunnel ] && traefik_service="ClusterIP"
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

if [ "$ORCHARD_INGRESS_MODE" != tunnel ]; then
  step "cert-manager $CERT_MANAGER_VERSION"
  helm repo add jetstack https://charts.jetstack.io >/dev/null 2>&1 || true
  helm repo update >/dev/null
  helm upgrade --install cert-manager jetstack/cert-manager -n cert-manager --create-namespace \
    --version "$CERT_MANAGER_VERSION" --set crds.enabled=true --wait >/dev/null
  if [ "$ORCHARD_INGRESS_MODE" = public ]; then
    cat <<EOF | kubectl apply -f - >/dev/null
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: $ORCHARD_ACME_EMAIL
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

if [ "$ORCHARD_INGRESS_MODE" = tunnel ]; then
  step "Cloudflare Tunnel"
  kubectl create namespace cloudflared --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  kubectl -n cloudflared create secret generic tunnel --from-literal=token="$ORCHARD_TUNNEL_TOKEN" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
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

# ---------------------------------------------------------------- orchard

step "Wack Club Orchard"
mkdir -p "$ORCHARD_STATE_DIR" /usr/local/share/orchard
if [ -z "$ORCHARD_CHART" ]; then
  tmp="$(mktemp -d)"
  curl -fsSL "$ORCHARD_REPO/archive/$ORCHARD_VERSION.tar.gz" | tar -xz -C "$tmp"
  rm -rf /usr/local/share/orchard/chart
  cp -r "$tmp"/*/deploy/helm/orchard /usr/local/share/orchard/chart
  ORCHARD_CHART=/usr/local/share/orchard/chart
fi

scheme=https; issuer=letsencrypt-prod; tls=true
case "$ORCHARD_INGRESS_MODE" in
  lan) scheme=http; issuer=selfsigned; tls=false ;;
  tunnel) tls=false ;;
esac
port_suffix=""
[ "$ORCHARD_INGRESS_MODE" = lan ] && [ "$HTTP_PORT" != 80 ] && port_suffix=":$HTTP_PORT"
extra_hosts=""
if [ "$ORCHARD_INGRESS_MODE" = lan ]; then
  for ip in $local_ips; do extra_hosts="$extra_hosts\"$ip\", \"orchard.$ip.sslip.io\", "; done
  extra_hosts="$extra_hosts\"$(hostname)\", \"$(hostname).local\""
fi

if [ ! -f "$ORCHARD_STATE_DIR/values.yaml" ]; then
cat > "$ORCHARD_STATE_DIR/values.yaml" <<EOF
# Generated by the Wack Club Orchard installer. Edit, then run orchard-update.
image:
  repository: $ORCHARD_IMAGE_REGISTRY/$ORCHARD_IMAGE_REPOSITORY
  tag: "$ORCHARD_VERSION"
ingress:
  host: $ORCHARD_DOMAIN
  extraHosts: [$extra_hosts]
  tls: $tls
  certClusterIssuer: $issuer
gateway:
  enabled: $( [ "$ORCHARD_INGRESS_MODE" = public ] && echo true || echo false )
  http:
    appDomain: $ORCHARD_APP_DOMAIN
tenant:
  ingressClass: traefik
  certClusterIssuer: $issuer
registry:
  enabled: true
  storage: ${registry_g}Gi
buildSlots: $build_slots
signupMode: open
mcp:
  enabled: $ORCHARD_MCP
  host: "${ORCHARD_MCP_DOMAIN:-}"
tenantSandbox:
  enabled: false
  runtimeClassName: gvisor
builderSandbox:
  enabled: false
  runtimeClassName: kata
server:
  env:
    - name: FRONTEND_URL
      value: "$scheme://$ORCHARD_DOMAIN$port_suffix"
    - name: INGRESS_MODE
      value: "$ORCHARD_INGRESS_MODE"
    - name: PUBLIC_HTTP_PORT
      value: "$HTTP_PORT"
    - name: PUBLIC_HTTPS_PORT
      value: "$HTTPS_PORT"
    - name: PUBLIC_IP
      value: ""
EOF
  info "wrote $ORCHARD_STATE_DIR/values.yaml"
else
  info "keeping existing $ORCHARD_STATE_DIR/values.yaml"
fi

helm upgrade --install orchard "$ORCHARD_CHART" -n orchard --create-namespace \
  -f "$ORCHARD_STATE_DIR/values.yaml" --wait --timeout 10m

# orchardctl and orchard-update live on the host
image="$ORCHARD_IMAGE_REGISTRY/$ORCHARD_IMAGE_REPOSITORY:$ORCHARD_VERSION"
kubectl -n orchard exec deploy/orchard-server -- cat /usr/local/bin/orchardctl > /usr/local/bin/orchardctl && chmod +x /usr/local/bin/orchardctl
kubectl -n orchard exec deploy/orchard-server -- cat /usr/local/bin/orchard > /usr/local/bin/orchard && chmod +x /usr/local/bin/orchard
cat > /usr/local/bin/orchard-update <<EOF
#!/usr/bin/env bash
set -euo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
exec /usr/local/bin/orchardctl update
EOF
chmod +x /usr/local/bin/orchard-update
cat > /etc/profile.d/orchard.sh <<'EOF'
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
EOF

if [ "$ORCHARD_UPDATE_POLICY" = automatic ]; then
  cat > /etc/systemd/system/orchard-update.service <<'EOF'
[Unit]
Description=Update Wack Club Orchard
[Service]
Type=oneshot
ExecStart=/usr/local/bin/orchard-update
EOF
  cat > /etc/systemd/system/orchard-update.timer <<'EOF'
[Unit]
Description=Nightly Wack Club Orchard update
[Timer]
OnCalendar=*-*-* 04:17:00
RandomizedDelaySec=30m
Persistent=true
[Install]
WantedBy=timers.target
EOF
  systemctl daemon-reload && systemctl enable --now orchard-update.timer >/dev/null
  info "automatic updates: nightly (systemctl list-timers orchard-update.timer)"
fi

step "Done"
claim="$(KUBECONFIG=/etc/rancher/k3s/k3s.yaml /usr/local/bin/orchardctl claim --url "$scheme://$ORCHARD_DOMAIN$port_suffix" 2>/dev/null | tail -1 | tr -d ' ')"
echo
echo "  ${green}Wack Club Orchard is running.${reset}"
echo
echo "  Dashboard:  $scheme://$ORCHARD_DOMAIN$port_suffix"
[ "$ORCHARD_MCP" = true ] && [ -n "${ORCHARD_MCP_DOMAIN:-}" ] && echo "  MCP:        https://$ORCHARD_MCP_DOMAIN/mcp"
echo
echo "  ${bold}Claim link${reset} (single use, valid 24h, signs in as the superadmin; open it yourself):"
echo
echo "    $claim"
echo
echo "  Sign up first, then open the link. Add a passkey or password straight after:"
echo "  the link is spent. Lost it? sudo orchardctl claim"
[ "$ORCHARD_INGRESS_MODE" = lan ] && echo "  Reached by another name (Tailscale)? sudo orchardctl hostname add <name> --apply"
echo "  Set a default public IP for TCP/UDP and databases: sudo orchardctl public-ip set <ip> --apply"
echo

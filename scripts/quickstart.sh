#!/usr/bin/env bash
# Wack Club Orchard on a throwaway local cluster.
#
#   ./scripts/quickstart.sh          create the cluster and deploy
#   ./scripts/quickstart.sh down     delete the cluster and its registry
#   ./scripts/quickstart.sh claim    print a fresh claim link
#   ./scripts/quickstart.sh reload   rebuild the image from this checkout and roll it out
#
# It creates a k3d cluster with a local registry, installs the platform
# layer (Gateway API, Traefik's gateway provider, cert-manager,
# CloudNativePG), builds the server image from this checkout, and deploys
# the chart with deploy/helm/wackcluborchard/values-dev.yaml.
#
# Needs: docker (running), k3d, kubectl, helm.

set -euo pipefail

CLUSTER="${WACKCLUBORCHARD_CLUSTER:-wackcluborchard}"
REGISTRY="$CLUSTER-registry"
REGISTRY_PORT="${WACKCLUBORCHARD_REGISTRY_PORT:-5050}"
HTTP_PORT="${WACKCLUBORCHARD_HTTP_PORT:-8080}"
HTTPS_PORT="${WACKCLUBORCHARD_HTTPS_PORT:-8443}"
NAMESPACE=wackcluborchard
GATEWAY_API_VERSION="${GATEWAY_API_VERSION:-v1.2.1}"
CERT_MANAGER_VERSION="${CERT_MANAGER_VERSION:-v1.16.2}"
CNPG_CHART_VERSION="${CNPG_CHART_VERSION:-0.23.0}"

root="$(cd "$(dirname "$0")/.." && pwd)"
chart="$root/deploy/helm/wackcluborchard"
ctx="k3d-$CLUSTER"

red=$'\e[31m'; green=$'\e[32m'; bold=$'\e[1m'; reset=$'\e[0m'
step() { printf '\n%s==>%s %s%s%s\n' "$red" "$reset" "$bold" "$*" "$reset"; }
die() { printf '%s✗ %s%s\n' "$red" "$*" "$reset" >&2; exit 1; }
k() { kubectl --context "$ctx" "$@"; }

need() {
  for bin in "$@"; do
    command -v "$bin" >/dev/null 2>&1 || die "$bin is not installed. See https://k3d.io, https://kubernetes.io/docs/tasks/tools/ and https://helm.sh/docs/intro/install/"
  done
  docker info >/dev/null 2>&1 || die "docker is not running"
}

claim() {
  k -n "$NAMESPACE" exec deploy/wackcluborchard-server -- \
    wackcluborchard-server admin claim --data /data --url "http://wackcluborchard.localhost:$HTTP_PORT"
}

build_image() {
  step "Building the server image from this checkout"
  docker build -t wackcluborchard:dev "$root"
  k3d image import wackcluborchard:dev -c "$CLUSTER"
}

case "${1:-up}" in
  down)
    need k3d
    k3d cluster delete "$CLUSTER" || true
    k3d registry delete "k3d-$REGISTRY" 2>/dev/null || true
    echo "deleted $CLUSTER"
    exit 0
    ;;
  claim)
    need kubectl
    claim
    exit 0
    ;;
  reload)
    need k3d kubectl
    build_image
    k -n "$NAMESPACE" rollout restart deploy/wackcluborchard-server
    k -n "$NAMESPACE" rollout status deploy/wackcluborchard-server --timeout 180s
    exit 0
    ;;
  up) ;;
  *) die "usage: $0 [up|down|claim|reload]" ;;
esac

need k3d kubectl helm

step "Cluster $CLUSTER"
if ! k3d registry list 2>/dev/null | grep -q "k3d-$REGISTRY"; then
  k3d registry create "$REGISTRY" --port "$REGISTRY_PORT"
fi
if k3d cluster list 2>/dev/null | grep -q "^$CLUSTER "; then
  echo "already exists; reusing it"
else
  # The registry joins the cluster network (and CoreDNS), so builders push
  # to k3d-<registry>:5000 over plain HTTP. containerd pulls the same name,
  # so tell it that endpoint is HTTP too.
  regconf="$(mktemp)"
  printf 'mirrors:\n  "k3d-%s:5000":\n    endpoint:\n      - http://k3d-%s:5000\n' "$REGISTRY" "$REGISTRY" > "$regconf"
  k3d cluster create "$CLUSTER" \
    --agents 1 \
    --registry-use "k3d-$REGISTRY:$REGISTRY_PORT" \
    --registry-config "$regconf" \
    --port "$HTTP_PORT:80@loadbalancer" \
    --port "$HTTPS_PORT:443@loadbalancer" \
    --wait
fi
kubectl config use-context "$ctx" >/dev/null

step "Gateway API $GATEWAY_API_VERSION"
k apply -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/$GATEWAY_API_VERSION/standard-install.yaml" >/dev/null

step "Traefik: enable the Gateway API provider"
k apply -f - >/dev/null <<'EOF'
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
EOF
# k3s re-runs the Traefik chart when its config changes
for _ in $(seq 60); do
  k -n kube-system get deploy traefik >/dev/null 2>&1 && break
  sleep 2
done
k -n kube-system rollout status deploy/traefik --timeout 180s >/dev/null || true
for _ in $(seq 60); do
  k get gatewayclass traefik >/dev/null 2>&1 && break
  sleep 2
done

step "cert-manager $CERT_MANAGER_VERSION"
helm repo add jetstack https://charts.jetstack.io >/dev/null 2>&1 || true
helm repo add cnpg https://cloudnative-pg.github.io/charts >/dev/null 2>&1 || true
helm repo update >/dev/null
helm --kube-context "$ctx" upgrade --install cert-manager jetstack/cert-manager \
  -n cert-manager --create-namespace --version "$CERT_MANAGER_VERSION" \
  --set crds.enabled=true --wait >/dev/null
k apply -f - >/dev/null <<'EOF'
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned
spec:
  selfSigned: {}
EOF

step "CloudNativePG"
helm --kube-context "$ctx" upgrade --install cnpg cnpg/cloudnative-pg \
  -n cnpg-system --create-namespace --version "$CNPG_CHART_VERSION" --wait >/dev/null

build_image

step "Wack Club Orchard"
helm --kube-context "$ctx" upgrade --install wackcluborchard "$chart" \
  -n "$NAMESPACE" --create-namespace \
  -f "$chart/values-dev.yaml" \
  --wait --timeout 5m

step "Done"
link="$(claim)"
cat <<EOF

  ${green}Wack Club Orchard is running on k3d.${reset}

  Dashboard:  http://wackcluborchard.localhost:$HTTP_PORT
  Apps:       https://<app>.apps.localhost:$HTTPS_PORT   (self-signed certificate)
  Registry:   localhost:$REGISTRY_PORT

  Sign up, then open this single-use link to become the instance superadmin:

    $link

  ./scripts/quickstart.sh reload   rebuild and roll out after changing code
  ./scripts/quickstart.sh claim    mint another claim link
  ./scripts/quickstart.sh down     delete everything

EOF

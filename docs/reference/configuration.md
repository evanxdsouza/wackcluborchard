# Configuration

The server reads flags and environment variables. Environment values win over
what is stored, so the installer's values file stays the source of truth.

| Variable | Default | Meaning |
| --- | --- | --- |
| `WACKCLUBORCHARD_ADDR` / `PORT` | `:8080` | Listen address |
| `WACKCLUBORCHARD_DATA` | `./data` | State directory (`wackcluborchard.json`, `admin.sock`) |
| `WACKCLUBORCHARD_RUNTIME` | `auto` | `kubernetes`, `sim`, or `auto` (in-cluster or kubeconfig if reachable, else sim) |
| `WACKCLUBORCHARD_DEMO` | `true` on sim | Seed the demo project for the first account |
| `WACKCLUBORCHARD_WEB_DIR` | embedded | Serve the dashboard from a directory |
| `FRONTEND_URL` | | Public dashboard URL: links, OAuth redirects, Secure cookies, passkey origin |
| `INSTANCE_NAME` | Wack Club Orchard | |
| `WACKCLUBORCHARD_DOMAIN` | localhost | Dashboard hostname |
| `APP_DOMAIN` | apps.localhost | Apps get `<name>.<APP_DOMAIN>` |
| `INGRESS_MODE` | lan | `public`, `tunnel`, `lan` |
| `INGRESS_CNAME` | | What users point custom domains at |
| `PUBLIC_IP` | | Default IP for TCP/UDP ports and public databases |
| `PUBLIC_DB_DOMAIN` | | Wildcard for public database hostnames |
| `PUBLIC_HTTP_PORT` / `PUBLIC_HTTPS_PORT` | | Non-standard ports (lan mode) |
| `SIGNUP_MODE` | open | `open`, `invite`, `closed` |
| `BUILD_SLOTS` | 2 | Concurrent builds |
| `MCP_ENABLED` / `MCP_DOMAIN` | true / | The MCP endpoint |
| `TENANT_SANDBOX` / `BUILDER_SANDBOX` | false | gVisor for apps / Kata for builds |
| `WACKCLUBORCHARD_NAMESPACE` | wackcluborchard | Control-plane namespace |
| `GATEWAY_NAME` / `GATEWAY_NAMESPACE` | wackcluborchard | Shared Gateway for app hostnames |
| `TENANT_INGRESS_CLASS` | traefik | Ingress class for custom domains |
| `TENANT_CERT_ISSUER` | letsencrypt-prod | cert-manager ClusterIssuer for custom domains |
| `TENANT_STORAGE_CLASS` | cluster default | For app and database volumes |
| `REGISTRY_HOST` / `REGISTRY_INSECURE` | registry.wackcluborchard…:5000 / true | Where builds push |
| `BUILDKIT_IMAGE`, `GIT_IMAGE`, `POSTGRES_IMAGE`, `SANDBOX_IMAGE` | | Override images |
| `BUILDER_RUNTIME_CLASS`, `SANDBOX_RUNTIME_CLASS` | / kata | RuntimeClasses |
| `FORWARD_AUTH_URL` | in-cluster server | Auth wall endpoint Traefik calls |
| `ANTHROPIC_API_KEY`, `WACKCLUBORCHARD_AGENT_MODEL` | / claude-sonnet-5 | Greenhouse agent |
| `WACKCLUBORCHARD_SIM_EXEC` | | `1` runs job scripts locally in the simulator (dev only) |
| `WACKCLUBORCHARD_QUIET` | | Silence the request log |

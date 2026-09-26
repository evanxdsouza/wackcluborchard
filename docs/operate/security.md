# Security and isolation

**Network.** Each project namespace gets a NetworkPolicy admitting traffic only
from pods in the same namespace and from the ingress and control-plane
namespaces. Sandboxes get internet egress only.

**Kernel.** Tenant pods share the host kernel unless you install gVisor and turn
on **gVisor for new tenant apps** (per app: Settings → gVisor sandbox). Builds
share it unless Kata is installed and **Kata VMs for image builds** is on.
Sandboxes always request the `kata` RuntimeClass. Running code you do not trust
is not safe without these.

**Pods.** Tenant namespaces enforce the `baseline` Pod Security Standard; app
pods do not mount a service account token or get service links.

**Accounts.** Passwords are PBKDF2-SHA256 (210k iterations); API tokens are
stored hashed; sessions are HttpOnly SameSite=Lax cookies (Secure over HTTPS);
cookie-authenticated writes must come from the dashboard origin; sign-in, signup
and claim are rate-limited. Passkeys verify origin, RP ID hash, user presence and
signature counters.

**Secrets.** Database passwords and secret variables live in the control-plane
state and in Kubernetes Secrets. Protect the state volume and etcd accordingly.

**Audit.** Every change, shell session and write query is logged per organization.

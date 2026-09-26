# What is Wack Club Orchard

Wack Club Orchard installs into a Kubernetes cluster you control and gives the
people using that cluster a platform instead of a pile of YAML. Connect a
GitHub repo, and Orchard builds it, runs it, routes traffic to it, gives it a
database, and shows you the logs when it breaks.

It is in the same family as Coolify and Dokploy: something you run yourself, on
your own hardware, that makes deploying feel like a hosted platform. The
difference is what it targets. Orchard schedules onto Kubernetes, so it inherits
replicas, rolling updates, health checks, resource quotas and node scheduling
rather than reimplementing them.

## What it is not

**Not a hosted service.** You run it.

**Not a Kubernetes installer.** The one-command installer stands up k3s on a
bare box, but on an existing cluster Orchard expects you to bring the platform
layer underneath it: CloudNativePG, an ingress controller with the Gateway API,
cert-manager, a storage class and a registry. See [Install](install.md).

**Not hardened for untrusted code out of the box.** Deployments are
network-isolated by default, but kernel isolation (gVisor, Kata) is opt-in and
needs node-level runtimes. See [Security](../operate/security.md).

**Not an abstraction that hides Kubernetes.** Pods, replicas and namespaces are
still there, and the UI names them. Orchard is a better front door, not a
disguise.

## How it fits together

Your cluster runs the Orchard control plane (one server process with its state
on a volume, and a zot registry) alongside tenant namespaces holding what people
deploy: one namespace per project. Builds run as Jobs on the cluster and push
to the registry. Informers mirror cluster state into the control plane so reads
are fast and survive a restart. More in [Architecture](../operate/architecture.md).

Without a cluster the server runs a **simulated runtime** so the whole product
can be tried on a laptop: `go run ./cmd/orchard-server`.

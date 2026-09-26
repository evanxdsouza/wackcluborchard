# Architecture

| Component | What it is |
| --- | --- |
| **Server** | One Go binary: API, dashboard, SSE, WebSocket terminals, MCP, build queue, job scheduler, informers. Standard library only |
| **State** | An in-memory document store snapshotted atomically to a volume (`/data/orchard.json`) |
| **Builders** | Kubernetes Jobs: an `alpine/git` clone init container and rootless BuildKit |
| **Registry** | zot, for images Orchard builds (or your own) |
| **Tenant namespaces** | `orchard-<org>-<project>`: apps, databases, jobs; `orchard-<org>-greenhouse`: sandboxes |

```
Browser / CLI / MCP ──▶ API ──reads──▶ state (mirror)
                          │
                          └─writes─▶ apply queue ──server-side apply, retry──▶ Kubernetes API
                                                                                   │
      SSE ◀── event bus ◀── informers (pods, events, CNPG clusters) ◀──watch────────┘
```

## The mirror

The API does not ask Kubernetes on every page load. Informers list and watch
managed pods, warning events and CloudNativePG clusters and mirror their state
into the store; reads come from there. When the mirror and the cluster disagree,
the cluster wins: the mirror is a cache of reality.

## Writes

Cluster writes go through a **retrying apply queue** keyed per object. A deploy
records intent and returns; the queue reconciles, retrying with backoff, and a
newer desired state supersedes a retry in progress. Every write is a
**server-side apply** with field manager `orchard`, so changes made with
`kubectl` to fields Orchard does not own are left alone.

## Events

State changes go on an in-process event bus and out to the dashboard as
server-sent events. The bus keeps a ring of recent events, so a reconnecting
client replays from the last id it saw rather than starting blind.

## Restarts

On start the server re-applies desired state for every app, database and
sandbox, and marks builds and job runs that were in flight as failed with a
message saying why. Build logs are kept in memory.

## Scale

The state store is single-writer, so the server runs one replica with a
`Recreate` strategy. It is fast enough for hundreds of apps; the work that
scales with tenants (builds, jobs, databases, apps) runs in the cluster.

## The simulated runtime

`internal/runtime/sim.go` implements the same driver interface in-process:
pods start and become ready, images produce plausible logs, crash loops produce
crash reports, databases provision and understand a small SQL subset, sandboxes
boot in stages. It is how the dashboard is developed and demoed without a cluster.

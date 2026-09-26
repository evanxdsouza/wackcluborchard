# Concepts

```
Organization
└── Project                  (one Kubernetes namespace)
    ├── Environment          (Production by default)
    ├── App                  Deployment + Service + HTTPRoute/Ingress + PVCs
    ├── Database             CloudNativePG Cluster
    ├── Job                  steps run as init containers of one pod
    └── Template instance / Compose stack
```

**Organization**: members, roles, quotas, SSO, SCIM and audit logs. Each
project in it gets its own namespace, so tenants are separated at the cluster
level, not only in the UI.

**Project**: apps, databases and jobs that belong together. Project membership
gates access to what is inside. Projects carry shared variables, an icon and a
sky of their own.

**App**: one deployable unit: an image, replicas, ports, variables, resources,
volumes, a health check. From a GitHub repo (Orchard builds it) or an image.

**Environment**: a named slice of a project. Variables can be scoped to one.

**Database**: a managed PostgreSQL cluster with backups, extensions, read
replicas, sizing and optional public exposure.

## Status

Statuses reflect what the cluster reports, not a timer: a pod that runs but fails
its readiness check is not "running", and a build in flight dominates. When the
mirror and the cluster disagree, the cluster wins.

## Roles

| Role | Can |
| --- | --- |
| Owner | Everything, including deleting the organization |
| Admin | Members, projects, quotas and settings; sees every project |
| Member | Create and manage resources in projects they belong to |
| Viewer | Read-only; no credentials, shells or queries |

Superadmin is separate: an instance-level role for whoever operates the platform.

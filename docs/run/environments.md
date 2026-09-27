# Environments and variables

A project starts with **Production**; add Staging or anything else under
**Project → Settings → Environments**. Apps, databases and jobs belong to one
environment, and the environment picker on the project page filters by it.

## Layers

An app sees variables in layers, later ones winning:

1. shared by the project, all environments
2. shared by the project, scoped to the app's environment
3. the app's own

Secret variables are hidden from viewers and in listings, and written to a
Kubernetes Secret like the rest. Wack Club Orchard also sets `PORT`, `WACKCLUBORCHARD_APP` and
`WACKCLUBORCHARD_URL`.

## References

Values can point at things in the same project, resolved at rollout:

| Reference | Becomes |
| --- | --- |
| `${{ db.DATABASE_URL }}` | `postgresql://user:pass@pg-db-rw.<ns>.svc.cluster.local:5432/name` |
| `${{ db.HOST }}` `PORT` `USER` `PASSWORD` `DATABASE` | the parts |
| `${{ app.HOST }}` `PORT` `URL` | the app's in-cluster name, port, `http://app:port` |
| `${{ app.PUBLIC_URL }}` | its HTTPS URL |

Saving variables rolls every affected app. Import a whole `.env` from the
Variables tab or with `wackcluborchard env <project> KEY=value …`.

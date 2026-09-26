# Databases

**New → PostgreSQL database** creates a CloudNativePG `Cluster` named
`pg-<name>` in the project's namespace, PostgreSQL 13 to 18, with its own
database and owner. The read-write service is `pg-<name>-rw`, read-only `-ro`.

## Connection

URI, `psql`, `.env` and JDBC forms with copy buttons, and the host, port,
database, user and password. Viewers cannot see credentials. From apps, prefer
`${{ <name>.DATABASE_URL }}` over pasting.

## Backups

Logical `pg_dump` backups in custom format on the database volume, on a cron
schedule (03:00 UTC daily by default) or on demand. The last 30 are listed.

## Extensions

pgvector, pg_stat_statements, pgcrypto, uuid-ossp, citext, hstore, pg_trgm,
PostGIS and more, enabled with `CREATE EXTENSION` in the application database.

## Queries and terminal

**Queries** runs SQL (⌘↵) and renders results as a table, with per-database
history. **Terminal** is a `psql` session in the primary. Write queries are
recorded in the audit log.

## Resources and replicas

CPU and memory per instance (requests equal limits), storage that can grow but
not shrink, and 0 to 4 streaming read replicas.

## Stop and public access

**Stop** hibernates the cluster (`cnpg.io/hibernation=on`) and frees its compute.
**Public access** assigns a random free port on the organization's shared IP
(a NodePort to the primary). Anyone with the password can connect; prefer the
internal host from apps.

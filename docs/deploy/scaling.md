# Scaling and resources

## Replicas

The replica slider on an app's overview changes how many copies run. Rolling
updates (`maxUnavailable: 0`) mean a deploy does not drop traffic as long as
health checks pass. Apps with volumes roll out by recreating their pod, since
volumes are single-writer.

## CPU and memory

**Requests equal limits.** What you pick is what the app gets and what it is
capped at, which avoids the trap where a container looks fine until the node
gets busy and it is throttled to a much smaller request. Size honestly: quotas
are enforced against requests.

When changing resources through the API, send every field you want to keep.

## Health checks

An HTTP path becomes both a readiness probe (no traffic until it passes) and a
more patient liveness probe. They are part of the Deployment Orchard applies on
every rollout, so a rebuild does not drop them.

## Quotas

Organizations have caps, and each member and viewer has an allowance inside
them. The effective limit for something new is the smaller of the two. See
[Organizations](../teams/organizations.md).

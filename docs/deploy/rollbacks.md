# Rollbacks

The **Deploys** tab lists every image the app has run, newest first, with the
commit, trigger and duration. Roll back to any successful one in one click
(or `wackcluborchard rollback <app>`).

A rollback is an ordinary deploy that points at an older image: same rolling
update, same health checks. It does not revert your repository and does not undo
database migrations.

## Recovering from a bad deploy

1. Roll back to the last known good image.
2. Fix the repository.
3. Push, which builds and deploys forward again.

## Restarting

**Restart** recreates the pods on the same image (it bumps a pod-template
annotation). Reach for it when the process is wedged, not when the release is
wrong.
